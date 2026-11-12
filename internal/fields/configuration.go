package fields

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // Configuration must work on hosts without a zoneinfo database.
	"unicode/utf8"
)

// FieldCapabilities advertises accepted configuration keys, not persistence or
// authorization guarantees. Settings use the exact ValidationRules JSON names.
type FieldCapabilities struct {
	Settings []string `json:"settings"`
}

var capabilities = buildCapabilities()

func buildCapabilities() map[FieldType]FieldCapabilities {
	m := make(map[FieldType]FieldCapabilities)
	common := []string{"required", "description", "default", "layout_width", "group", "hidden", "read_only", "private", "read_roles", "edit_roles", "error_message"}
	add := func(t FieldType, settings ...string) {
		m[t] = FieldCapabilities{Settings: append(slices.Clone(common), settings...)}
	}
	for _, t := range []FieldType{TypeShortText, TypeLongText, TypeRichText, TypeEmail, TypeSecret, TypeUID, TypeURL} {
		add(t, "placeholder", "min_length", "max_length", "regex")
	}
	extend := func(t FieldType, settings ...string) {
		c := m[t]
		c.Settings = append(c.Settings, settings...)
		m[t] = c
	}
	for _, t := range []FieldType{TypeShortText, TypeLongText, TypeEmail, TypeUID, TypeURL} {
		extend(t, "unique")
	}
	extend(TypeUID, "target_field")
	extend(TypeURL, "allowed_protocols")
	add(TypeNumber, "placeholder", "unique", "min", "max")
	add(TypeDecimal, "placeholder", "unique", "min", "max")
	add(TypeBoolean)
	for _, t := range []FieldType{TypeDate, TypeDateTime, TypeTime} {
		add(t, "date_min", "date_max", "timezone", "unique")
	}
	add(TypeEnum, "enum_values", "placeholder", "unique")
	add(TypeJSON, "json_schema")
	add(TypeMedia, "multiple", "allowed_types", "min_items", "max_items", "preserve_order")
	add(TypeSingleRelation, "target_type", "relation_cardinality")
	add(TypeMultipleRelation, "target_type", "relation_cardinality", "min_items", "max_items", "preserve_order")
	add(TypeComponent, "component", "repeatable", "min_items", "max_items", "preserve_order")
	add(TypeRepeatableComponent, "component", "repeatable", "min_items", "max_items", "preserve_order")
	add(TypeDynamicZone, "components", "min_items", "max_items", "preserve_order")
	add(TypeBlocks, "allowed_blocks")
	return m
}

// Capabilities returns a fresh map and settings slices safe for caller mutation.
func Capabilities() map[FieldType]FieldCapabilities {
	m := make(map[FieldType]FieldCapabilities, len(capabilities))
	for t, c := range capabilities {
		m[t] = FieldCapabilities{Settings: slices.Clone(c.Settings)}
	}
	return m
}

// ValidateConfiguration validates new configuration, including defaults. It does
// not resolve target schemas, apply defaults, or rewrite legacy configurations.
// Only unidirectional many_to_one (single) and many_to_many (multiple) relations
// are supported; empty cardinality preserves existing behavior.
func ValidateConfiguration(def FieldDefinition) error { return validateConfiguration(def, true) }

func validateConfiguration(def FieldDefinition, complete bool) error {
	fail := func(format string, args ...any) error {
		return FieldError{def.Name, "invalid configuration: " + fmt.Sprintf(format, args...)}
	}
	c, ok := capabilities[def.Type]
	if !ok {
		return fail("unsupported field type %q", def.Type)
	}
	r := def.Rules
	v, typ := reflect.ValueOf(r), reflect.TypeOf(r)
	for i := 0; i < v.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if !v.Field(i).IsZero() && !slices.Contains(c.Settings, key) {
			return fail("%s is not supported for %s", key, def.Type)
		}
	}
	for _, pair := range [][2]*int{{r.MinLength, r.MaxLength}, {r.MinItems, r.MaxItems}} {
		for _, n := range pair {
			if n != nil && (*n < 0 || *n > MaxValueBytes) {
				return fail("length/item bounds must be between 0 and %d", MaxValueBytes)
			}
		}
		if pair[0] != nil && pair[1] != nil && *pair[0] > *pair[1] {
			return fail("minimum exceeds maximum")
		}
	}
	for _, n := range []*float64{r.Min, r.Max} {
		if n != nil && (math.IsNaN(*n) || math.IsInf(*n, 0) || (def.Type == TypeNumber && math.Trunc(*n) != *n)) {
			return fail("numeric bounds must be finite and integer fields require integer bounds")
		}
	}
	if r.Min != nil && r.Max != nil && *r.Min > *r.Max {
		return fail("minimum exceeds maximum")
	}
	if err := validPattern(r.Regex); err != nil {
		return fail("regex: %v", err)
	}
	if !slices.Contains([]string{"", "full", "half", "third", "two-thirds"}, r.LayoutWidth) {
		return fail("invalid layout_width")
	}
	for _, s := range []string{r.Description, r.Placeholder, r.Group, r.ErrorMessage, def.Description, def.Label, def.Name, r.TargetField, r.TargetType, def.TargetType, r.Component} {
		if !utf8.ValidString(s) || len(s) > 4096 {
			return fail("metadata must be UTF-8 and at most 4096 bytes")
		}
	}
	for _, list := range [][]string{r.ReadRoles, r.EditRoles, r.EnumVals, r.AllowedProtocols, r.AllowedTypes, r.Components, r.AllowedBlocks} {
		if err := validNames(list); err != nil {
			return fail("%v", err)
		}
	}
	if def.Type == TypeEnum && len(r.EnumVals) == 0 {
		return fail("enum_values must not be empty")
	}
	for _, protocol := range r.AllowedProtocols {
		if !slices.Contains(safeProtocols, protocol) {
			return fail("allowed_protocols supports only http, https, ftp, ftps, mailto, tel")
		}
	}
	for _, kind := range r.AllowedTypes {
		if !slices.Contains([]string{"image", "video", "audio", "file"}, kind) {
			return fail("unsupported media allowed_types category %q", kind)
		}
	}
	for _, block := range r.AllowedBlocks {
		if !isBlockType(block) {
			return fail("unsupported allowed_blocks node %q", block)
		}
	}
	if r.Timezone != "" {
		if r.Timezone == "Local" {
			return fail("timezone must be an explicit IANA timezone")
		}
		if _, err := time.LoadLocation(r.Timezone); err != nil {
			return fail("invalid timezone")
		}
	}
	var lower, upper time.Time
	var err error
	if r.DateMin != "" {
		lower, err = parseTemporal(def.Type, r.DateMin)
		if err != nil {
			return fail("invalid date_min: %v", err)
		}
	}
	if r.DateMax != "" {
		upper, err = parseTemporal(def.Type, r.DateMax)
		if err != nil {
			return fail("invalid date_max: %v", err)
		}
	}
	if r.DateMin != "" && r.DateMax != "" && lower.After(upper) {
		return fail("date_min exceeds date_max")
	}
	if r.RelationCardinality != "" && !((def.Type == TypeSingleRelation && r.RelationCardinality == "many_to_one") || (def.Type == TypeMultipleRelation && r.RelationCardinality == "many_to_many")) {
		return fail("only many_to_one for relation_single and many_to_many for relation_multiple are supported")
	}
	if def.Type == TypeMedia && !r.Multiple || def.Type == TypeComponent && !r.Repeatable {
		if r.MinItems != nil || r.MaxItems != nil || r.PreserveOrder {
			return fail("item bounds and preserve_order require an array field")
		}
	}
	if r.Required && r.MaxItems != nil && *r.MaxItems == 0 {
		return fail("required arrays cannot have max_items zero")
	}
	if r.Required && r.MaxLength != nil && *r.MaxLength == 0 {
		return fail("required strings cannot have max_length zero")
	}
	if def.Type == TypeUID && r.TargetField != "" && strings.EqualFold(r.TargetField, def.Name) {
		return fail("UID cannot use itself as target_field")
	}
	if def.TargetType != "" && def.Type != TypeSingleRelation && def.Type != TypeMultipleRelation && def.Type != TypeComponent && def.Type != TypeRepeatableComponent {
		return fail("target_type is not supported for this type")
	}
	if def.TargetType != "" && r.TargetType != "" && def.TargetType != r.TargetType {
		return fail("conflicting target_type settings")
	}
	if r.JSONSchema != nil {
		if err := validateSchema(r.JSONSchema); err != nil {
			return fail("json_schema: %v", err)
		}
	}
	if complete {
		if (def.Type == TypeSingleRelation || def.Type == TypeMultipleRelation) && strings.TrimSpace(r.TargetType) == "" && strings.TrimSpace(def.TargetType) == "" {
			return fail("target_type is required")
		}
		if (def.Type == TypeComponent || def.Type == TypeRepeatableComponent) && strings.TrimSpace(r.Component) == "" && strings.TrimSpace(def.TargetType) == "" {
			return fail("component is required")
		}
		if def.Type == TypeDynamicZone && len(r.Components) == 0 {
			return fail("components must not be empty")
		}
		for _, value := range []any{def.Default, r.Default} {
			if value != nil {
				if err := validateValue(def, value); err != nil {
					return fail("invalid default: %v", err)
				}
			}
		}
		if def.Default != nil && r.Default != nil && !jsonEqual(def.Default, r.Default) {
			return fail("conflicting default settings")
		}
	}
	return nil
}

func validNames(names []string) error {
	if len(names) > 256 {
		return fmt.Errorf("setting lists cannot exceed 256 entries")
	}
	seen := make(map[string]bool, len(names))
	for _, s := range names {
		if strings.TrimSpace(s) != s || s == "" || !utf8.ValidString(s) || len(s) > 4096 || seen[s] {
			return fmt.Errorf("setting lists require distinct, nonempty UTF-8 strings without surrounding whitespace")
		}
		seen[s] = true
	}
	return nil
}

func validPattern(pattern string) error {
	// Go's RE2 engine guarantees linear-time matching. Cap compilation input;
	// unsupported lookaround/backreferences and malformed syntax fail closed.
	if !utf8.ValidString(pattern) || len(pattern) > 4096 {
		return fmt.Errorf("pattern exceeds 4096 bytes or is not UTF-8")
	}
	_, err := regexp.Compile(pattern)
	return err
}

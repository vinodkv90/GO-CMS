package fields

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// MaxValueBytes, MaxDepth and MaxNodes bound work on untrusted values. Container
// depth is counted from one; blocks count document/node depth separately.
const (
	MaxValueBytes = 1 << 20
	MaxDepth      = 8
	MaxNodes      = 10000
)

var uidRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var safeProtocols = []string{"http", "https", "ftp", "ftps", "mailto", "tel"}
var timeRegex = regexp.MustCompile(`^\d{2}:\d{2}(?::\d{2}(?:\.\d{1,9})?)?$`)
var datetimeRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$`)

// ValidateValue is pure: it never sanitizes, hashes, coerces, or applies defaults.
// Configuration constraints are checked even for absent optional values. Target
// schema existence is deliberately left to ValidateConfiguration/the service.
func ValidateValue(def FieldDefinition, val any) error {
	if err := validateConfiguration(def, false); err != nil {
		return err
	}
	if err := validateValue(def, val); err != nil {
		if def.Rules.ErrorMessage != "" {
			return FieldError{def.Name, def.Rules.ErrorMessage}
		}
		return FieldError{def.Name, err.Error()}
	}
	return nil
}

func validateValue(def FieldDefinition, val any) error {
	r := def.Rules
	if val == nil {
		if r.Required {
			return fmt.Errorf("value is required")
		}
		return nil
	}
	// Empty strings are missing only for string-backed types. A numeric/boolean/
	// object field cannot bypass strict typing with an empty string.
	if s, ok := val.(string); ok {
		if !utf8.ValidString(s) || len(s) > MaxValueBytes {
			return fmt.Errorf("string must be UTF-8 and at most %d bytes", MaxValueBytes)
		}
		if isStringType(def.Type) {
			if s == "" {
				if r.Required {
					return fmt.Errorf("value is required")
				}
				return nil
			}
			if err := validateString(s, r.MinLength, r.MaxLength, r.Regex); err != nil {
				return err
			}
		}
	}
	switch def.Type {
	case TypeShortText, TypeLongText, TypeRichText, TypeSecret:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("expected string value")
		}
	case TypeUID:
		s, ok := val.(string)
		if !ok || !uidRegex.MatchString(s) {
			return fmt.Errorf("UID must use lowercase ASCII letters, digits, and single hyphens")
		}
	case TypeEmail:
		s, ok := val.(string)
		if !ok || !validEmail(s) {
			return fmt.Errorf("invalid email address")
		}
	case TypeURL:
		s, ok := val.(string)
		if !ok || !validURL(s, r.AllowedProtocols) {
			return fmt.Errorf("invalid URL or disallowed protocol")
		}
	case TypeNumber, TypeDecimal:
		n, ok := numeric(val)
		if !ok {
			return fmt.Errorf("expected finite numeric value")
		}
		if def.Type == TypeNumber && !n.IsInt() {
			return fmt.Errorf("expected integer value")
		}
		if r.Min != nil && n.Cmp(new(big.Rat).SetFloat64(*r.Min)) < 0 {
			return fmt.Errorf("value is below minimum")
		}
		if r.Max != nil && n.Cmp(new(big.Rat).SetFloat64(*r.Max)) > 0 {
			return fmt.Errorf("value exceeds maximum")
		}
	case TypeBoolean:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("expected boolean value")
		}
	case TypeDate, TypeDateTime, TypeTime:
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("expected temporal string")
		}
		value, err := parseTemporal(def.Type, s)
		if err != nil {
			return err
		}
		if r.DateMin != "" {
			lower, _ := parseTemporal(def.Type, r.DateMin)
			if value.Before(lower) {
				return fmt.Errorf("value is before date_min")
			}
		}
		if r.DateMax != "" {
			upper, _ := parseTemporal(def.Type, r.DateMax)
			if value.After(upper) {
				return fmt.Errorf("value is after date_max")
			}
		}
	case TypeEnum:
		s, ok := val.(string)
		if !ok || !slices.Contains(r.EnumVals, s) {
			return fmt.Errorf("value must be one of enum_values")
		}
	case TypeJSON:
		value, err := normalizeJSON(val, true, MaxDepth)
		if err != nil {
			return err
		}
		if r.Required && value == nil {
			return fmt.Errorf("value is required")
		}
		if r.JSONSchema != nil {
			return checkSchema(r.JSONSchema, value, "$")
		}
	case TypeMedia:
		if r.Multiple {
			return validateIDs(val, r)
		}
		return validateID(val)
	case TypeSingleRelation:
		return validateID(val)
	case TypeMultipleRelation:
		return validateIDs(val, r)
	case TypeComponent:
		if r.Repeatable {
			return validateComponents(val, def)
		}
		if _, ok := val.(map[string]any); !ok {
			return fmt.Errorf("expected component object")
		}
		_, err := normalizeJSON(val, false, MaxDepth)
		return err
	case TypeRepeatableComponent, TypeDynamicZone:
		return validateComponents(val, def)
	case TypeBlocks:
		return validateBlocks(val, r)
	default:
		return fmt.Errorf("unsupported field type %q", def.Type)
	}
	return nil
}

func isStringType(t FieldType) bool {
	switch t {
	case TypeShortText, TypeLongText, TypeRichText, TypeSecret, TypeUID, TypeEmail, TypeURL, TypeEnum, TypeDate, TypeTime, TypeDateTime, TypeSingleRelation:
		return true
	case TypeMedia:
		return false // Empty IDs always fail; absence is represented by null.
	}
	return false
}

func validateString(s string, min, max *int, pattern string) error {
	n := utf8.RuneCountInString(s)
	if min != nil && n < *min {
		return fmt.Errorf("length must be at least %d characters", *min)
	}
	if max != nil && n > *max {
		return fmt.Errorf("length cannot exceed %d characters", *max)
	}
	if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil || !re.MatchString(s) {
			return fmt.Errorf("value does not match required pattern")
		}
	}
	return nil
}

func validEmail(s string) bool {
	if len(s) > 254 || strings.TrimSpace(s) != s || strings.ContainsAny(s, "\r\n") {
		return false
	}
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s && strings.Contains(s, "@")
}

func validURL(s string, protocols []string) bool {
	if s == "" || strings.ContainsRune(s, '\\') || strings.IndexFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.User != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if len(protocols) == 0 {
		protocols = []string{"http", "https"}
	}
	if !slices.Contains(safeProtocols, scheme) || !slices.Contains(protocols, scheme) {
		return false
	}
	switch scheme {
	case "mailto":
		return u.Host == "" && u.RawQuery == "" && u.Fragment == "" && validEmail(u.Opaque)
	case "tel":
		return u.Host == "" && u.RawQuery == "" && u.Fragment == "" && phoneRegex.MatchString(u.Opaque)
	default:
		if u.Opaque != "" || u.Hostname() == "" || strings.ContainsAny(u.Hostname(), "<>\"'{}|") {
			return false
		}
		if port := u.Port(); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return false
			}
		}
		return true
	}
}

var phoneRegex = regexp.MustCompile(`^\+?[0-9][0-9().-]*$`)

// DateTime requires an explicit RFC3339 offset. Timezone is UI conversion
// metadata; ambiguous local datetimes are never guessed at persistence time.
func parseTemporal(t FieldType, s string) (time.Time, error) {
	layout := ""
	switch t {
	case TypeDate:
		layout = "2006-01-02"
	case TypeTime:
		if !timeRegex.MatchString(s) {
			return time.Time{}, fmt.Errorf("expected HH:mm or HH:mm:ss[.fraction]")
		}
		layout = "15:04"
		if len(s) > 5 {
			layout = "15:04:05.999999999"
		}
	case TypeDateTime:
		if !datetimeRegex.MatchString(s) {
			return time.Time{}, fmt.Errorf("expected RFC3339 datetime with explicit offset")
		}
		if s[len(s)-1] != 'Z' {
			offset := s[len(s)-5:]
			h, _ := strconv.Atoi(offset[:2])
			m, _ := strconv.Atoi(offset[3:])
			if h > 23 || m > 59 {
				return time.Time{}, fmt.Errorf("invalid datetime offset")
			}
		}
		layout = time.RFC3339Nano
	default:
		return time.Time{}, fmt.Errorf("temporal bound on non-temporal field")
	}
	v, err := time.Parse(layout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s format", t)
	}
	return v, nil
}

// numeric preserves integer precision (including uint64 and json.Number).
// JSON numbers use a bounded representation; finite float64 range is required.
func numeric(value any) (*big.Rat, bool) {
	var s string
	switch v := value.(type) {
	case int:
		s = strconv.FormatInt(int64(v), 10)
	case int8:
		s = strconv.FormatInt(int64(v), 10)
	case int16:
		s = strconv.FormatInt(int64(v), 10)
	case int32:
		s = strconv.FormatInt(int64(v), 10)
	case int64:
		s = strconv.FormatInt(v, 10)
	case uint:
		s = strconv.FormatUint(uint64(v), 10)
	case uint8:
		s = strconv.FormatUint(uint64(v), 10)
	case uint16:
		s = strconv.FormatUint(uint64(v), 10)
	case uint32:
		s = strconv.FormatUint(uint64(v), 10)
	case uint64:
		s = strconv.FormatUint(v, 10)
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false
		}
		return new(big.Rat).SetFloat64(f), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, false
		}
		return new(big.Rat).SetFloat64(v), true
	case json.Number:
		s = string(v)
		if len(s) == 0 || len(s) > 128 || !json.Valid([]byte(s)) || strings.TrimSpace(s) != s {
			return nil, false
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false
		}
		if at := strings.IndexAny(s, "eE"); at >= 0 {
			exponent, err := strconv.Atoi(s[at+1:])
			if err != nil || exponent < -1000 || exponent > 1000 {
				return nil, false
			}
		}
	default:
		return nil, false
	}
	n, ok := new(big.Rat).SetString(s)
	return n, ok
}

func validateID(value any) error {
	s, ok := value.(string)
	if !ok || strings.TrimSpace(s) == "" || strings.TrimSpace(s) != s || len(s) > 4096 || !utf8.ValidString(s) || strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return fmt.Errorf("expected nonempty ID string without surrounding whitespace")
	}
	return nil
}

func array(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case []string:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out, true
	case []map[string]any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out, true
	default:
		return nil, false
	}
}

func validateCount(n int, r ValidationRules) error {
	if n > MaxNodes {
		return fmt.Errorf("array exceeds %d items", MaxNodes)
	}
	if r.Required && n == 0 {
		return fmt.Errorf("at least one item is required")
	}
	if r.MinItems != nil && n < *r.MinItems {
		return fmt.Errorf("too few items")
	}
	if r.MaxItems != nil && n > *r.MaxItems {
		return fmt.Errorf("too many items")
	}
	return nil
}

func validateIDs(value any, r ValidationRules) error {
	items, ok := array(value)
	if !ok {
		return fmt.Errorf("expected array of ID strings")
	}
	if err := validateCount(len(items), r); err != nil {
		return err
	}
	seen, size := make(map[string]bool, len(items)), 0
	for _, item := range items {
		if err := validateID(item); err != nil {
			return err
		}
		s := item.(string)
		size += len(s)
		if size > MaxValueBytes {
			return fmt.Errorf("IDs exceed size limit")
		}
		if seen[s] {
			return fmt.Errorf("duplicate IDs are not allowed")
		}
		seen[s] = true
	}
	return nil
}

func validateComponents(value any, def FieldDefinition) error {
	items, ok := array(value)
	if !ok {
		return fmt.Errorf("expected array of component objects")
	}
	if err := validateCount(len(items), def.Rules); err != nil {
		return err
	}
	if _, err := normalizeJSON(value, false, MaxDepth); err != nil {
		return err
	}
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok || object == nil {
			return fmt.Errorf("each component must be an object")
		}
		if def.Type == TypeDynamicZone {
			name, ok := object["__component"].(string)
			if !ok || !slices.Contains(def.Rules.Components, name) {
				return fmt.Errorf("dynamic zone requires an allowed __component discriminator")
			}
		}
	}
	return nil
}

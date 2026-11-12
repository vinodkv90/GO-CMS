// Package fields defines built-in field configuration and value contracts.
// Integrity, authorization, uniqueness, secret hashing, defaults, and recursive
// component schema lookup belong to the content service.
package fields

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
)

type FieldType string

const (
	TypeShortText           FieldType = "short_text"
	TypeLongText            FieldType = "long_text"
	TypeRichText            FieldType = "rich_text"
	TypeNumber              FieldType = "number"
	TypeDecimal             FieldType = "decimal"
	TypeBoolean             FieldType = "boolean"
	TypeDate                FieldType = "date"
	TypeDateTime            FieldType = "datetime"
	TypeTime                FieldType = "time"
	TypeEmail               FieldType = "email"
	TypeSecret              FieldType = "secret"
	TypeURL                 FieldType = "url"
	TypeEnum                FieldType = "enum"
	TypeJSON                FieldType = "json"
	TypeMedia               FieldType = "media"
	TypeSingleRelation      FieldType = "relation_single"
	TypeMultipleRelation    FieldType = "relation_multiple"
	TypeComponent           FieldType = "component"
	TypeRepeatableComponent FieldType = "repeatable_component"
	TypeDynamicZone         FieldType = "dynamic_zone"
	TypeUID                 FieldType = "uid"
	TypeBlocks              FieldType = "blocks"
)

// ValidationRules preserves existing configuration JSON keys. A nil Default
// means no non-null default. Defaults are validated but never applied here.
type ValidationRules struct {
	Required            bool           `json:"required"`
	Unique              bool           `json:"unique"`
	MinLength           *int           `json:"min_length,omitempty"`
	MaxLength           *int           `json:"max_length,omitempty"`
	Min                 *float64       `json:"min,omitempty"`
	Max                 *float64       `json:"max,omitempty"`
	Regex               string         `json:"regex,omitempty"`
	EnumVals            []string       `json:"enum_values,omitempty"`
	TargetField         string         `json:"target_field,omitempty"`
	TargetType          string         `json:"target_type,omitempty"`
	Multiple            bool           `json:"multiple,omitempty"`
	AllowedTypes        []string       `json:"allowed_types,omitempty"`
	MinItems            *int           `json:"min_items,omitempty"`
	MaxItems            *int           `json:"max_items,omitempty"`
	Component           string         `json:"component,omitempty"`
	Repeatable          bool           `json:"repeatable,omitempty"`
	Components          []string       `json:"components,omitempty"`
	PreserveOrder       bool           `json:"preserve_order,omitempty"`
	Description         string         `json:"description,omitempty"`
	Placeholder         string         `json:"placeholder,omitempty"`
	Default             any            `json:"default,omitempty"`
	LayoutWidth         string         `json:"layout_width,omitempty"`
	Group               string         `json:"group,omitempty"`
	Hidden              bool           `json:"hidden,omitempty"`
	ReadOnly            bool           `json:"read_only,omitempty"`
	Private             bool           `json:"private,omitempty"`
	ReadRoles           []string       `json:"read_roles,omitempty"`
	EditRoles           []string       `json:"edit_roles,omitempty"`
	ErrorMessage        string         `json:"error_message,omitempty"`
	AllowedProtocols    []string       `json:"allowed_protocols,omitempty"`
	DateMin             string         `json:"date_min,omitempty"`
	DateMax             string         `json:"date_max,omitempty"`
	Timezone            string         `json:"timezone,omitempty"`
	AllowedBlocks       []string       `json:"allowed_blocks,omitempty"`
	JSONSchema          map[string]any `json:"json_schema,omitempty"`
	RelationCardinality string         `json:"relation_cardinality,omitempty"`
}

// FieldDefinition retains legacy top-level metadata for compatibility.
type FieldDefinition struct {
	Name        string          `json:"name"`
	Label       string          `json:"label"`
	Type        FieldType       `json:"type"`
	Description string          `json:"description,omitempty"`
	Default     any             `json:"default,omitempty"`
	Rules       ValidationRules `json:"rules"`
	ReadOnly    bool            `json:"read_only"`
	Private     bool            `json:"private"`
	TargetType  string          `json:"target_type,omitempty"`
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e FieldError) Error() string { return fmt.Sprintf("field %q: %s", e.Field, e.Message) }

// FieldValidatorFunc validates a plugin-registered custom field value.
type FieldValidatorFunc func(value any) error

// CustomFieldDescriptor advertises a plugin-registered custom field type for the builder UI.
type CustomFieldDescriptor struct {
	Type        FieldType `json:"type"`
	Label       string    `json:"label,omitempty"`
	Description string    `json:"description,omitempty"`
	PluginSlug  string    `json:"plugin_slug,omitempty"`
	PluginName  string    `json:"plugin_name,omitempty"`
	Settings    []string  `json:"settings,omitempty"`
}

type customFieldEntry struct {
	descriptor CustomFieldDescriptor
	validator  FieldValidatorFunc
}

var defaultCustomSettings = []string{
	"required",
	"unique",
	"description",
	"placeholder",
	"default",
	"layout_width",
	"group",
	"hidden",
	"read_only",
	"private",
	"read_roles",
	"edit_roles",
	"error_message",
}

// Registry tracks active built-in and plugin-registered custom field types.
// Bare Register registers names only; custom field validation in schema/entry
// services requires explicit custom registration via RegisterCustom.
type Registry struct {
	mu     sync.RWMutex
	types  map[FieldType]bool
	custom map[FieldType]customFieldEntry
}

func NewRegistry() *Registry {
	r := &Registry{
		types:  make(map[FieldType]bool),
		custom: make(map[FieldType]customFieldEntry),
	}
	for ft := range capabilities {
		r.types[ft] = true
	}
	return r
}

func (r *Registry) Register(ft FieldType) error {
	if ft == "" {
		return errors.New("field type cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.types == nil {
		r.types = make(map[FieldType]bool)
	}
	r.types[ft] = true
	return nil
}

// RegisterCustom registers a plugin custom field type with UI metadata and a runtime validator.
func (r *Registry) RegisterCustom(desc CustomFieldDescriptor, validator FieldValidatorFunc) error {
	desc.Type = FieldType(strings.TrimSpace(string(desc.Type)))
	if desc.Type == "" {
		return errors.New("field type cannot be empty")
	}
	if _, builtin := capabilities[desc.Type]; builtin {
		return fmt.Errorf("cannot override built-in field type %q", desc.Type)
	}
	if strings.TrimSpace(desc.Label) == "" {
		desc.Label = formatCustomFieldLabel(string(desc.Type))
	}
	if len(desc.Settings) == 0 {
		desc.Settings = slices.Clone(defaultCustomSettings)
	} else {
		desc.Settings = slices.Clone(desc.Settings)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.types == nil {
		r.types = make(map[FieldType]bool)
	}
	if r.custom == nil {
		r.custom = make(map[FieldType]customFieldEntry)
	}
	r.types[desc.Type] = true
	r.custom[desc.Type] = customFieldEntry{
		descriptor: desc,
		validator:  validator,
	}
	return nil
}

// UnregisterPluginFields removes all custom fields owned by pluginSlug.
func (r *Registry) UnregisterPluginFields(pluginSlug string) {
	if pluginSlug == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for ft, entry := range r.custom {
		if entry.descriptor.PluginSlug == pluginSlug {
			delete(r.custom, ft)
			if _, builtin := capabilities[ft]; !builtin {
				delete(r.types, ft)
			}
		}
	}
}

func (r *Registry) IsValid(ft FieldType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.types[ft]
}

// Capabilities returns built-in field capabilities plus active custom field capabilities.
func (r *Registry) Capabilities() map[FieldType]FieldCapabilities {
	out := Capabilities()
	r.mu.RLock()
	defer r.mu.RUnlock()
	for ft, entry := range r.custom {
		out[ft] = FieldCapabilities{Settings: slices.Clone(entry.descriptor.Settings)}
	}
	return out
}

// CustomFields returns metadata descriptors for all active plugin-registered custom fields.
func (r *Registry) CustomFields() []CustomFieldDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CustomFieldDescriptor, 0, len(r.custom))
	for _, entry := range r.custom {
		desc := entry.descriptor
		desc.Settings = slices.Clone(desc.Settings)
		out = append(out, desc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PluginSlug != out[j].PluginSlug {
			return out[i].PluginSlug < out[j].PluginSlug
		}
		return out[i].Type < out[j].Type
	})
	return out
}

// ValidateFieldConfiguration validates built-in or plugin-registered custom field configurations.
func (r *Registry) ValidateFieldConfiguration(def FieldDefinition) error {
	if _, builtin := capabilities[def.Type]; builtin {
		return ValidateConfiguration(def)
	}
	entry, ok := r.customEntry(def.Type)
	if !ok {
		return ValidateConfiguration(def)
	}
	return validateCustomRules(def, entry, true)
}

// ValidateField validates built-in or plugin-registered custom field values.
func (r *Registry) ValidateField(def FieldDefinition, val any) error {
	if _, builtin := capabilities[def.Type]; builtin {
		return ValidateValue(def, val)
	}
	entry, ok := r.customEntry(def.Type)
	if !ok {
		return ValidateValue(def, val)
	}
	if err := validateCustomRules(def, entry, false); err != nil {
		return err
	}
	if isEmptyCustomValue(val) {
		if def.Rules.Required {
			if msg := strings.TrimSpace(def.Rules.ErrorMessage); msg != "" {
				return errors.New(msg)
			}
			return errors.New("value is required")
		}
		return nil
	}
	if err := checkCustomJSONSize(val); err != nil {
		return err
	}
	if entry.validator != nil {
		if err := entry.validator(val); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) customEntry(ft FieldType) (customFieldEntry, bool) {
	if r == nil {
		return customFieldEntry{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.custom[ft]
	return entry, ok
}

func validateCustomRules(def FieldDefinition, entry customFieldEntry, checkDefault bool) error {
	switch def.Rules.LayoutWidth {
	case "", "full", "half", "third", "two_thirds", "quarter":
	default:
		return fmt.Errorf("invalid layout_width %q", def.Rules.LayoutWidth)
	}
	for _, role := range def.Rules.ReadRoles {
		if strings.TrimSpace(role) == "" {
			return errors.New("read_roles entries must not be empty")
		}
	}
	for _, role := range def.Rules.EditRoles {
		if strings.TrimSpace(role) == "" {
			return errors.New("edit_roles entries must not be empty")
		}
	}
	if !checkDefault {
		return nil
	}
	defaultVal := def.Rules.Default
	if defaultVal == nil {
		defaultVal = def.Default
	}
	if defaultVal != nil && !isEmptyCustomValue(defaultVal) {
		if err := checkCustomJSONSize(defaultVal); err != nil {
			return fmt.Errorf("invalid default value: %w", err)
		}
		if entry.validator != nil {
			if err := entry.validator(defaultVal); err != nil {
				return fmt.Errorf("invalid default value: %w", err)
			}
		}
	}
	return nil
}

func isEmptyCustomValue(val any) bool {
	if val == nil {
		return true
	}
	if s, ok := val.(string); ok && s == "" {
		return true
	}
	return false
}

func checkCustomJSONSize(val any) error {
	raw, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("value must be JSON-compatible: %w", err)
	}
	if len(raw) > MaxValueBytes {
		return fmt.Errorf("value exceeds maximum allowed size (%d bytes)", MaxValueBytes)
	}
	return nil
}

func formatCustomFieldLabel(raw string) string {
	cleaned := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(raw), "_", " "), "-", " ")
	parts := strings.Fields(cleaned)
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	if len(parts) == 0 {
		return raw
	}
	return strings.Join(parts, " ")
}

package fields

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FieldType identifies the data type and rendering behavior of a field.
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
)

// ValidationRules defines constraints applied to a field's value.
type ValidationRules struct {
	Required  bool     `json:"required"`
	Unique    bool     `json:"unique"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Regex     string   `json:"regex,omitempty"`
	EnumVals  []string `json:"enum_values,omitempty"`
	// Advanced field settings are kept alongside validation rules for backward
	// compatible schema JSON. They are validated by the content service.
	TargetField   string   `json:"target_field,omitempty"`
	TargetType    string   `json:"target_type,omitempty"`
	Multiple      bool     `json:"multiple,omitempty"`
	AllowedTypes  []string `json:"allowed_types,omitempty"`
	MinItems      *int     `json:"min_items,omitempty"`
	MaxItems      *int     `json:"max_items,omitempty"`
	Component     string   `json:"component,omitempty"`
	Repeatable    bool     `json:"repeatable,omitempty"`
	Components    []string `json:"components,omitempty"`
	PreserveOrder bool     `json:"preserve_order,omitempty"`
}

// FieldDefinition specifies the metadata and rules for a single content attribute.
type FieldDefinition struct {
	Name        string          `json:"name"`
	Label       string          `json:"label"`
	Type        FieldType       `json:"type"`
	Description string          `json:"description,omitempty"`
	Default     any             `json:"default,omitempty"`
	Rules       ValidationRules `json:"rules"`
	ReadOnly    bool            `json:"read_only"`
	Private     bool            `json:"private"`               // Excluded from unauthenticated or public API responses
	TargetType  string          `json:"target_type,omitempty"` // For relations or components (slug of target)
}

// FieldError describes a validation failure for a specific field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e FieldError) Error() string {
	return fmt.Sprintf("field %q: %s", e.Field, e.Message)
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
var urlRegex = regexp.MustCompile(`^(http|https)://[^\s/$.?#].[^\s]*$`)
var uidRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ValidateValue checks a raw value against a field's definition.
func ValidateValue(def FieldDefinition, val any) error {
	// 1. Required Check
	if val == nil || val == "" {
		if def.Rules.Required {
			return FieldError{Field: def.Name, Message: "value is required"}
		}
		return nil
	}

	// 2. Type-specific validations
	switch def.Type {
	case TypeShortText, TypeLongText, TypeRichText:
		str, ok := val.(string)
		if !ok {
			return FieldError{Field: def.Name, Message: "expected string value"}
		}
		if def.Rules.MinLength != nil && len(str) < *def.Rules.MinLength {
			return FieldError{Field: def.Name, Message: fmt.Sprintf("length must be at least %d characters", *def.Rules.MinLength)}
		}
		if def.Rules.MaxLength != nil && len(str) > *def.Rules.MaxLength {
			return FieldError{Field: def.Name, Message: fmt.Sprintf("length cannot exceed %d characters", *def.Rules.MaxLength)}
		}
		if def.Rules.Regex != "" {
			matched, err := regexp.MatchString(def.Rules.Regex, str)
			if err != nil || !matched {
				return FieldError{Field: def.Name, Message: "value does not match required pattern"}
			}
		}

	case TypeUID:
		str, ok := val.(string)
		if !ok {
			return FieldError{Field: def.Name, Message: "expected string for UID"}
		}
		if !uidRegex.MatchString(str) {
			return FieldError{Field: def.Name, Message: "UID must be lower-case alphanumeric with hyphens (e.g. my-first-post)"}
		}

	case TypeEmail:
		str, ok := val.(string)
		if !ok || !emailRegex.MatchString(str) {
			return FieldError{Field: def.Name, Message: "invalid email format"}
		}

	case TypeURL:
		str, ok := val.(string)
		if !ok || !urlRegex.MatchString(str) {
			return FieldError{Field: def.Name, Message: "invalid URL format (must begin with http:// or https://)"}
		}

	case TypeNumber:
		var num float64
		switch v := val.(type) {
		case int:
			num = float64(v)
		case int64:
			num = float64(v)
		case float64:
			num = v
		default:
			return FieldError{Field: def.Name, Message: "expected numeric value"}
		}
		if def.Rules.Min != nil && num < *def.Rules.Min {
			return FieldError{Field: def.Name, Message: fmt.Sprintf("minimum allowed value is %v", *def.Rules.Min)}
		}
		if def.Rules.Max != nil && num > *def.Rules.Max {
			return FieldError{Field: def.Name, Message: fmt.Sprintf("maximum allowed value is %v", *def.Rules.Max)}
		}

	case TypeDecimal:
		num, ok := val.(float64)
		if !ok {
			return FieldError{Field: def.Name, Message: "expected decimal number"}
		}
		if def.Rules.Min != nil && num < *def.Rules.Min {
			return FieldError{Field: def.Name, Message: fmt.Sprintf("minimum allowed value is %v", *def.Rules.Min)}
		}
		if def.Rules.Max != nil && num > *def.Rules.Max {
			return FieldError{Field: def.Name, Message: fmt.Sprintf("maximum allowed value is %v", *def.Rules.Max)}
		}

	case TypeBoolean:
		if _, ok := val.(bool); !ok {
			return FieldError{Field: def.Name, Message: "expected boolean (true/false)"}
		}

	case TypeDate:
		str, ok := val.(string)
		if !ok {
			return FieldError{Field: def.Name, Message: "expected ISO date string (YYYY-MM-DD)"}
		}
		if _, err := time.Parse("2006-01-02", str); err != nil {
			return FieldError{Field: def.Name, Message: "invalid date format (expected YYYY-MM-DD)"}
		}

	case TypeDateTime:
		str, ok := val.(string)
		if !ok {
			return FieldError{Field: def.Name, Message: "expected RFC3339 datetime string"}
		}
		if _, err := time.Parse(time.RFC3339, str); err != nil {
			return FieldError{Field: def.Name, Message: "invalid datetime format (expected RFC3339, e.g. 2026-11-08T15:00:00Z)"}
		}

	case TypeEnum:
		str, ok := val.(string)
		if !ok {
			return FieldError{Field: def.Name, Message: "expected string enumeration value"}
		}
		found := false
		for _, e := range def.Rules.EnumVals {
			if e == str {
				found = true
				break
			}
		}
		if !found {
			return FieldError{Field: def.Name, Message: fmt.Sprintf("value %q must be one of: %s", str, strings.Join(def.Rules.EnumVals, ", "))}
		}

	case TypeJSON:
		// Accept map, slice, or raw JSON string
		switch v := val.(type) {
		case map[string]any, []any:
			// Valid JSON structures
		case string:
			var js any
			if err := json.Unmarshal([]byte(v), &js); err != nil {
				return FieldError{Field: def.Name, Message: "invalid JSON syntax"}
			}
		default:
			return FieldError{Field: def.Name, Message: "expected valid JSON object or array"}
		}

	case TypeMedia:
		// Media can be a string asset ID or slice of asset IDs
		switch v := val.(type) {
		case string:
			if def.Rules.Multiple {
				return FieldError{Field: def.Name, Message: "expected an array of media asset IDs"}
			}
		case []string, []any:
			if !def.Rules.Multiple {
				return FieldError{Field: def.Name, Message: "expected a single media asset ID"}
			}
			if def.Rules.MaxItems != nil && lenSlice(v) > *def.Rules.MaxItems {
				return FieldError{Field: def.Name, Message: "too many media assets selected"}
			}
			// Valid media reference
		default:
			return FieldError{Field: def.Name, Message: "expected media asset ID or array of IDs"}
		}

	case TypeSingleRelation:
		if _, ok := val.(string); !ok {
			return FieldError{Field: def.Name, Message: "expected target record ID for single relation"}
		}

	case TypeMultipleRelation:
		switch val.(type) {
		case []string, []any:
			// Valid list of target IDs
		default:
			return FieldError{Field: def.Name, Message: "expected array of target record IDs"}
		}

	case TypeComponent:
		if _, ok := val.(map[string]any); !ok {
			return FieldError{Field: def.Name, Message: "expected a component object"}
		}

	case TypeRepeatableComponent, TypeDynamicZone:
		items, ok := val.([]any)
		if !ok {
			return FieldError{Field: def.Name, Message: "expected an array of component objects"}
		}
		if def.Rules.MinItems != nil && len(items) < *def.Rules.MinItems {
			return FieldError{Field: def.Name, Message: "not enough component items"}
		}
		if def.Rules.MaxItems != nil && len(items) > *def.Rules.MaxItems {
			return FieldError{Field: def.Name, Message: "too many component items"}
		}
		for _, item := range items {
			if _, ok := item.(map[string]any); !ok {
				return FieldError{Field: def.Name, Message: "each component item must be an object"}
			}
		}
	}

	return nil
}

func lenSlice(v any) int {
	switch x := v.(type) {
	case []string:
		return len(x)
	case []any:
		return len(x)
	default:
		return 0
	}
}

// Registry maintains allowed field types and plugins can register custom field types.
type Registry struct {
	types map[FieldType]bool
}

// NewRegistry creates a registry initialized with all built-in field types.
func NewRegistry() *Registry {
	r := &Registry{types: make(map[FieldType]bool)}
	builtins := []FieldType{
		TypeShortText, TypeLongText, TypeRichText, TypeNumber, TypeDecimal,
		TypeBoolean, TypeDate, TypeDateTime, TypeEmail, TypeSecret, TypeURL,
		TypeEnum, TypeJSON, TypeMedia, TypeSingleRelation, TypeMultipleRelation,
		TypeComponent, TypeRepeatableComponent, TypeDynamicZone, TypeUID,
	}
	for _, t := range builtins {
		r.types[t] = true
	}
	return r
}

// Register adds a new field type (used by plugins).
func (r *Registry) Register(ft FieldType) error {
	if ft == "" {
		return errors.New("field type cannot be empty")
	}
	r.types[ft] = true
	return nil
}

// IsValid checks if a field type is recognized.
func (r *Registry) IsValid(ft FieldType) bool {
	return r.types[ft]
}

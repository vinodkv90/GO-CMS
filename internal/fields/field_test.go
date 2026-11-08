package fields

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateValue_Required(t *testing.T) {
	def := FieldDefinition{
		Name:  "title",
		Type:  TypeShortText,
		Rules: ValidationRules{Required: true},
	}

	assert.Error(t, ValidateValue(def, nil))
	assert.Error(t, ValidateValue(def, ""))
	assert.NoError(t, ValidateValue(def, "Hello World"))
}

func TestValidateValue_ShortTextConstraints(t *testing.T) {
	min := 3
	max := 10
	def := FieldDefinition{
		Name:  "slug",
		Type:  TypeShortText,
		Rules: ValidationRules{MinLength: &min, MaxLength: &max},
	}

	assert.Error(t, ValidateValue(def, "ab"))          // too short
	assert.Error(t, ValidateValue(def, "12345678901")) // too long
	assert.NoError(t, ValidateValue(def, "valid"))
}

func TestValidateValue_UID(t *testing.T) {
	def := FieldDefinition{
		Name: "slug",
		Type: TypeUID,
	}

	assert.NoError(t, ValidateValue(def, "my-first-blog-post"))
	assert.NoError(t, ValidateValue(def, "article-123"))
	assert.Error(t, ValidateValue(def, "Invalid Slug With Spaces"))
	assert.Error(t, ValidateValue(def, "bad--slug"))
	assert.Error(t, ValidateValue(def, "UPPERCASE"))
}

func TestValidateValue_Email(t *testing.T) {
	def := FieldDefinition{Name: "contact", Type: TypeEmail}

	assert.NoError(t, ValidateValue(def, "test@example.com"))
	assert.Error(t, ValidateValue(def, "invalid-email"))
}

func TestValidateValue_URL(t *testing.T) {
	def := FieldDefinition{Name: "website", Type: TypeURL}

	assert.NoError(t, ValidateValue(def, "https://example.com/blog"))
	assert.NoError(t, ValidateValue(def, "http://localhost:8080"))
	assert.Error(t, ValidateValue(def, "not-a-url"))
}

func TestValidateValue_NumberAndDecimal(t *testing.T) {
	min := 5.0
	max := 100.0
	defNum := FieldDefinition{
		Name:  "stock",
		Type:  TypeNumber,
		Rules: ValidationRules{Min: &min, Max: &max},
	}

	assert.NoError(t, ValidateValue(defNum, 10))
	assert.NoError(t, ValidateValue(defNum, 5))
	assert.Error(t, ValidateValue(defNum, 2))   // below min
	assert.Error(t, ValidateValue(defNum, 105)) // above max

	defDec := FieldDefinition{
		Name:  "price",
		Type:  TypeDecimal,
		Rules: ValidationRules{Min: &min, Max: &max},
	}
	assert.NoError(t, ValidateValue(defDec, 19.99))
	assert.Error(t, ValidateValue(defDec, 2.50))
}

func TestValidateValue_DateAndDateTime(t *testing.T) {
	defDate := FieldDefinition{Name: "published_date", Type: TypeDate}
	assert.NoError(t, ValidateValue(defDate, "2026-11-08"))
	assert.Error(t, ValidateValue(defDate, "08/11/2026"))

	defDT := FieldDefinition{Name: "published_at", Type: TypeDateTime}
	assert.NoError(t, ValidateValue(defDT, "2026-11-08T15:00:00Z"))
	assert.Error(t, ValidateValue(defDT, "not a date"))
}

func TestValidateValue_Enum(t *testing.T) {
	def := FieldDefinition{
		Name:  "status",
		Type:  TypeEnum,
		Rules: ValidationRules{EnumVals: []string{"draft", "in_review", "published"}},
	}

	assert.NoError(t, ValidateValue(def, "draft"))
	assert.NoError(t, ValidateValue(def, "published"))
	assert.Error(t, ValidateValue(def, "archived"))
}

func TestValidateValue_JSON(t *testing.T) {
	def := FieldDefinition{Name: "config", Type: TypeJSON}

	assert.NoError(t, ValidateValue(def, map[string]any{"theme": "dark"}))
	assert.NoError(t, ValidateValue(def, []any{1, 2, 3}))
	assert.NoError(t, ValidateValue(def, `{"key": "value"}`))
	assert.Error(t, ValidateValue(def, `{invalid-json`))
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	assert.True(t, reg.IsValid(TypeShortText))
	assert.True(t, reg.IsValid(TypeRichText))
	assert.False(t, reg.IsValid("custom_color_picker"))

	err := reg.Register("custom_color_picker")
	assert.NoError(t, err)
	assert.True(t, reg.IsValid("custom_color_picker"))
}


package fields

import (
	"testing"
)

func BenchmarkFields_ValidateShortText(b *testing.B) {
	minLen := 5
	maxLen := 50
	def := FieldDefinition{
		Name: "title",
		Type: TypeShortText,
		Rules: ValidationRules{
			Required:  true,
			MinLength: &minLen,
			MaxLength: &maxLen,
		},
	}
	value := "Production Go CMS Field Engine"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateValue(def, value)
	}
}

func BenchmarkFields_ValidateEmail(b *testing.B) {
	def := FieldDefinition{
		Name:  "email",
		Type:  TypeEmail,
		Rules: ValidationRules{Required: true},
	}
	value := "admin.dev@enterprise-cms.io"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateValue(def, value)
	}
}

func BenchmarkFields_ValidateJSON(b *testing.B) {
	def := FieldDefinition{
		Name:  "config",
		Type:  TypeJSON,
		Rules: ValidationRules{Required: true},
	}
	value := `{"title": "Go CMS", "version": 1.0, "tags": ["cms", "golang", "enterprise"]}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateValue(def, value)
	}
}

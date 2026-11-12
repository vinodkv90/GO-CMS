package fields

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvancedFieldValidationContracts(t *testing.T) {
	tests := []struct {
		name  string
		def   FieldDefinition
		value any
		valid bool
	}{
		{"time", FieldDefinition{Type: TypeTime}, "09:30:15.125", true},
		{"invalid time", FieldDefinition{Type: TypeTime}, "25:70", false},
		{"media ids", FieldDefinition{Type: TypeMedia, Rules: ValidationRules{Multiple: true, MaxItems: ptr(2)}}, []any{"asset-1", "asset-2"}, true},
		{"duplicate media", FieldDefinition{Type: TypeMedia, Rules: ValidationRules{Multiple: true}}, []any{"asset-1", "asset-1"}, false},
		{"relation ids", FieldDefinition{Type: TypeMultipleRelation}, []any{"entry-1"}, true},
		{"component object", FieldDefinition{Type: TypeComponent}, map[string]any{"heading": "Hello"}, true},
		{"blocks document", FieldDefinition{Type: TypeBlocks}, map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Hello"}}}}}, true},
		{"unsafe block link", FieldDefinition{Type: TypeBlocks}, map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "x", "marks": []any{map[string]any{"type": "link", "attrs": map[string]any{"href": "javascript:alert(1)"}}}}}}}}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateValue(test.def, test.value)
			if test.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestRichTextSanitizationRemovesExecutableMarkup(t *testing.T) {
	clean := SanitizeRichText(`<p onclick="alert(1)">Hello<script>alert(1)</script><a href="javascript:alert(1)">bad</a></p>`)
	assert.NotContains(t, clean, "onclick")
	assert.NotContains(t, clean, "<script")
	assert.NotContains(t, strings.ToLower(clean), "javascript:")
	assert.Contains(t, clean, "Hello")
}

func TestValidateConfigurationRejectsUnsupportedOrUnsafeOptions(t *testing.T) {
	def := FieldDefinition{Type: TypeBoolean, Rules: ValidationRules{MinLength: ptr(1)}}
	require.Error(t, ValidateConfiguration(def))
	def = FieldDefinition{Type: TypeURL, Rules: ValidationRules{AllowedProtocols: []string{"javascript"}}}
	require.Error(t, ValidateConfiguration(def))
}

func ptr(value int) *int { return &value }

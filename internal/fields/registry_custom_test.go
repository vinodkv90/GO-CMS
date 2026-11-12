package fields

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_CustomFieldsAndLifecycle(t *testing.T) {
	reg := NewRegistry()

	// Bare Register does not bypass validator requirement in Registry validation methods.
	require.NoError(t, reg.Register(FieldType("bare_unknown")))
	assert.True(t, reg.IsValid(FieldType("bare_unknown")))
	bareDef := FieldDefinition{Name: "x", Type: FieldType("bare_unknown")}
	assert.Error(t, reg.ValidateFieldConfiguration(bareDef))
	assert.Error(t, reg.ValidateField(bareDef, "abc"))
	assert.Error(t, ValidateValue(bareDef, "abc"))

	// Built-in types cannot be overridden by plugins.
	err := reg.RegisterCustom(CustomFieldDescriptor{Type: TypeShortText, PluginSlug: "evil"}, nil)
	assert.Error(t, err)

	// Register custom color_hex field with validator.
	hexValidator := func(val any) error {
		s, ok := val.(string)
		if !ok || !strings.HasPrefix(s, "#") || len(s) != 7 {
			return fmt.Errorf("must be a 7-character hex color")
		}
		return nil
	}
	err = reg.RegisterCustom(CustomFieldDescriptor{
		Type:       FieldType("color_hex"),
		PluginSlug: "theme-colors",
		PluginName: "Theme Colors",
	}, hexValidator)
	require.NoError(t, err)

	assert.True(t, reg.IsValid(FieldType("color_hex")))
	caps := reg.Capabilities()
	assert.Contains(t, caps, FieldType("color_hex"))
	assert.Contains(t, caps[FieldType("color_hex")].Settings, "required")

	descs := reg.CustomFields()
	require.Len(t, descs, 1)
	assert.Equal(t, FieldType("color_hex"), descs[0].Type)
	assert.Equal(t, "Color Hex", descs[0].Label)
	assert.Equal(t, "theme-colors", descs[0].PluginSlug)
	assert.Equal(t, "Theme Colors", descs[0].PluginName)

	def := FieldDefinition{
		Name: "accent",
		Type: FieldType("color_hex"),
		Rules: ValidationRules{
			Required:     true,
			Default:      "#0284c7",
			LayoutWidth:  "half",
			ErrorMessage: "Accent color is required",
		},
	}
	require.NoError(t, reg.ValidateFieldConfiguration(def))
	require.NoError(t, reg.ValidateField(def, "#0284c7"))

	badDefault := def
	badDefault.Rules.Default = "red"
	assert.Error(t, reg.ValidateFieldConfiguration(badDefault))

	err = reg.ValidateField(def, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Accent color is required")

	err = reg.ValidateField(def, "red")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hex color")

	// Unregister removes plugin custom fields cleanly.
	reg.UnregisterPluginFields("theme-colors")
	assert.False(t, reg.IsValid(FieldType("color_hex")))
	assert.Empty(t, reg.CustomFields())
	assert.NotContains(t, reg.Capabilities(), FieldType("color_hex"))
	assert.Error(t, reg.ValidateField(def, "#0284c7"))
}


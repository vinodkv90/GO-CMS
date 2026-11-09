package pluginsdk_test

import (
	"testing"

	pluginsdk "github.com/go-cms/cms/pkg/plugin-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifest_Validation(t *testing.T) {
	// 1. Valid manifest
	valid := pluginsdk.Manifest{
		ID:          "com.cms.analytics",
		Name:        "Analytics Plugin",
		Version:     "1.2.0",
		Description: "Tracks pageviews and content analytics",
	}
	require.NoError(t, valid.Validate())

	// 2. Missing ID
	missingID := pluginsdk.Manifest{Name: "No ID"}
	assert.ErrorIs(t, missingID.Validate(), pluginsdk.ErrMissingPluginID)

	// 3. Invalid ID format (uppercase, spaces)
	invalidID := pluginsdk.Manifest{ID: "Invalid Plugin ID!", Name: "Bad ID"}
	assert.ErrorIs(t, invalidID.Validate(), pluginsdk.ErrInvalidManifest)

	// 4. Missing Name
	missingName := pluginsdk.Manifest{ID: "valid-id"}
	assert.ErrorIs(t, missingName.Validate(), pluginsdk.ErrMissingPluginName)
}

func TestParseManifest(t *testing.T) {
	jsonBlob := []byte(`{
		"id": "audit-log",
		"name": "Audit Log Plugin",
		"version": "1.0.0",
		"description": "Enterprise audit logging",
		"permissions": ["hook:content:afterCreate", "audit:write"],
		"routes": [{"method": "GET", "path": "/stats"}]
	}`)

	m, err := pluginsdk.ParseManifest(jsonBlob)
	require.NoError(t, err)
	assert.Equal(t, "audit-log", m.ID)
	assert.Equal(t, "Audit Log Plugin", m.Name)
	assert.Len(t, m.Permissions, 2)
	assert.Len(t, m.Routes, 1)

	// Invalid JSON
	_, err = pluginsdk.ParseManifest([]byte(`{not-json}`))
	assert.ErrorIs(t, err, pluginsdk.ErrInvalidManifest)
}

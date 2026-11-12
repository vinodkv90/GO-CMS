package plugins_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/fields"
	"github.com/go-cms/cms/internal/plugins"
	pluginsdk "github.com/go-cms/cms/pkg/plugin-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPlugin struct {
	pluginsdk.BasePlugin
	initCalled  bool
	startCalled bool
	stopCalled  bool
}

func (m *mockPlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:      "test-plugin",
		Name:    "Test Plugin",
		Version: "1.0.0",
		Routes: []pluginsdk.RouteDef{
			{Method: http.MethodGet, Path: "/hello"},
		},
	}
}

func (m *mockPlugin) Init(ctx context.Context, host pluginsdk.Host) error {
	m.Host = host
	m.initCalled = true
	host.MountRoute(http.MethodGet, "/hello", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from plugin"))
	})
	return nil
}

func (m *mockPlugin) Start(ctx context.Context) error {
	m.startCalled = true
	return nil
}

func (m *mockPlugin) Stop(ctx context.Context) error {
	m.stopCalled = true
	return nil
}

func TestManager_Lifecycle(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eventBus := content.NewEventBus()
	fieldReg := fields.NewRegistry()

	// 1. Create a scanned plugin on disk
	diskPluginDir := filepath.Join(tempDir, "disk-plugin")
	require.NoError(t, os.MkdirAll(diskPluginDir, 0755))
	manifestJSON := []byte(`{
		"id": "disk-plugin",
		"name": "Disk Plugin",
		"version": "1.0.0",
		"description": "Loaded from disk"
	}`)
	require.NoError(t, os.WriteFile(filepath.Join(diskPluginDir, "plugin.json"), manifestJSON, 0644))

	mgr := plugins.NewManager(tempDir, eventBus, fieldReg, nil, logger)

	// 2. Scan disk directory
	err := mgr.Scan()
	require.NoError(t, err)

	entity, err := mgr.Get("disk-plugin")
	require.NoError(t, err)
	assert.Equal(t, "disk-plugin", entity.Manifest.ID)
	assert.Equal(t, plugins.StatusDiscovered, entity.Status)

	// 3. Register native plugin
	mock := &mockPlugin{}
	err = mgr.RegisterNative(mock)
	require.NoError(t, err)

	list := mgr.List()
	assert.Len(t, list, 2)

	// 4. Enable native plugin
	ctx := context.Background()
	err = mgr.Enable(ctx, "test-plugin")
	require.NoError(t, err)
	assert.True(t, mock.initCalled)
	assert.True(t, mock.startCalled)

	entity, err = mgr.Get("test-plugin")
	require.NoError(t, err)
	assert.Equal(t, plugins.StatusEnabled, entity.Status)
	assert.NotNil(t, entity.EnabledAt)

	// Duplicate enable returns error
	err = mgr.Enable(ctx, "test-plugin")
	assert.ErrorIs(t, err, plugins.ErrPluginAlreadyRuns)

	// 5. Update settings
	err = mgr.UpdateSettings(ctx, "test-plugin", map[string]any{"apiKey": "secret123"})
	require.NoError(t, err)
	val, ok := mock.Host.GetSetting("apiKey")
	assert.True(t, ok)
	assert.Equal(t, "secret123", val)

	// 6. Disable plugin
	err = mgr.Disable(ctx, "test-plugin")
	require.NoError(t, err)
	assert.True(t, mock.stopCalled)

	entity, err = mgr.Get("test-plugin")
	require.NoError(t, err)
	assert.Equal(t, plugins.StatusDisabled, entity.Status)

	// Disabling again returns error
	err = mgr.Disable(ctx, "test-plugin")
	assert.ErrorIs(t, err, plugins.ErrPluginNotEnabled)
}

type faultyPlugin struct {
	pluginsdk.BasePlugin
}

func (f *faultyPlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:      "faulty-plugin",
		Name:    "Faulty Plugin",
		Version: "1.0.0",
	}
}

func (f *faultyPlugin) Init(ctx context.Context, host pluginsdk.Host) error {
	return nil
}

func (f *faultyPlugin) Start(ctx context.Context) error {
	return assert.AnError
}

func (f *faultyPlugin) Stop(ctx context.Context) error {
	return nil
}

func TestManager_FailureScenarios(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := plugins.NewManager(tempDir, nil, nil, nil, logger)
	ctx := context.Background()

	// 1. Enable non-existent plugin
	err := mgr.Enable(ctx, "non-existent")
	assert.ErrorIs(t, err, plugins.ErrPluginNotFound)

	// 2. Disable non-existent plugin
	err = mgr.Disable(ctx, "non-existent")
	assert.ErrorIs(t, err, plugins.ErrPluginNotFound)

	// 3. Register faulty plugin whose Start() fails
	faulty := &faultyPlugin{}
	err = mgr.RegisterNative(faulty)
	require.NoError(t, err)

	err = mgr.Enable(ctx, "faulty-plugin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start plugin")

	entity, err := mgr.Get("faulty-plugin")
	require.NoError(t, err)
	assert.Equal(t, plugins.StatusError, entity.Status)
	assert.Contains(t, entity.ErrorMsg, "start failure")
}

type customFieldPlugin struct {
	pluginsdk.BasePlugin
}

func (c *customFieldPlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:      "custom-field-plugin",
		Name:    "Custom Field Plugin",
		Version: "1.0.0",
	}
}

func (c *customFieldPlugin) Init(ctx context.Context, host pluginsdk.Host) error {
	c.Host = host
	return host.RegisterFieldType("color_hex", func(val any) error {
		s, ok := val.(string)
		if !ok || len(s) != 7 || s[0] != '#' {
			return assert.AnError
		}
		return nil
	})
}

func (c *customFieldPlugin) Start(ctx context.Context) error { return nil }
func (c *customFieldPlugin) Stop(ctx context.Context) error  { return nil }

func TestManager_CustomFieldRegistrationLifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fieldReg := fields.NewRegistry()
	mgr := plugins.NewManager(t.TempDir(), content.NewEventBus(), fieldReg, nil, logger)

	require.NoError(t, mgr.RegisterNative(&customFieldPlugin{}))
	require.NoError(t, mgr.Enable(t.Context(), "custom-field-plugin"))

	descs := fieldReg.CustomFields()
	require.Len(t, descs, 1)
	assert.Equal(t, fields.FieldType("color_hex"), descs[0].Type)
	assert.Equal(t, "custom-field-plugin", descs[0].PluginSlug)
	assert.Equal(t, "Custom Field Plugin", descs[0].PluginName)

	def := fields.FieldDefinition{Name: "accent", Type: fields.FieldType("color_hex")}
	assert.NoError(t, fieldReg.ValidateField(def, "#ff5500"))
	assert.Error(t, fieldReg.ValidateField(def, "not-hex"))

	require.NoError(t, mgr.Disable(t.Context(), "custom-field-plugin"))
	assert.Empty(t, fieldReg.CustomFields())
	assert.False(t, fieldReg.IsValid(fields.FieldType("color_hex")))
}

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

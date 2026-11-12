package plugins_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/fields"
	"github.com/go-cms/cms/internal/plugins"
	pluginsdk "github.com/go-cms/cms/pkg/plugin-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createAdminUser() *auth.User {
	return &auth.User{
		ID:       "admin-user",
		Username: "admin",
		Status:   auth.StatusActive,
		Roles: []auth.Role{
			{Slug: auth.RoleSuperAdmin},
		},
	}
}

func setupTestPluginAPI(t *testing.T) (*chi.Mux, *plugins.Manager, *mockPlugin) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := plugins.NewManager(tempDir, content.NewEventBus(), fields.NewRegistry(), nil, logger)

	mock := &mockPlugin{}
	_ = mgr.RegisterNative(mock)

	handler := plugins.NewHandler(mgr)

	router := chi.NewRouter()
	router.Route("/api/v1", func(api chi.Router) {
		handler.RegisterRoutes(api)
	})

	return router, mgr, mock
}

func TestPluginHandler_FullLifecycleAPI(t *testing.T) {
	router, _, _ := setupTestPluginAPI(t)
	admin := createAdminUser()

	// 1. List Plugins via GET /api/v1/plugins
	req := httptest.NewRequest("GET", "/api/v1/plugins", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResp struct {
		Data []*plugins.PluginEntity `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &listResp)
	require.NoError(t, err)
	assert.Len(t, listResp.Data, 1)
	assert.Equal(t, "test-plugin", listResp.Data[0].Manifest.ID)
	assert.Equal(t, plugins.StatusInstalled, listResp.Data[0].Status)

	// 2. Get Plugin Details via GET /api/v1/plugins/{id}
	req = httptest.NewRequest("GET", "/api/v1/plugins/test-plugin", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 3. Enable Plugin via POST /api/v1/plugins/{id}/enable
	req = httptest.NewRequest("POST", "/api/v1/plugins/test-plugin/enable", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 4. Access Plugin Custom Route via GET /api/v1/plugins/ext/test-plugin/hello
	req = httptest.NewRequest("GET", "/api/v1/plugins/ext/test-plugin/hello", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello from plugin", w.Body.String())

	// 5. Update Plugin Settings via PATCH /api/v1/plugins/{id}/settings
	settingsBody := []byte(`{"theme": "dark", "rateLimit": 100}`)
	req = httptest.NewRequest("PATCH", "/api/v1/plugins/test-plugin/settings", bytes.NewReader(settingsBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 6. Disable Plugin via POST /api/v1/plugins/{id}/disable
	req = httptest.NewRequest("POST", "/api/v1/plugins/test-plugin/disable", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify plugin route is now 404
	req = httptest.NewRequest("GET", "/api/v1/plugins/ext/test-plugin/hello", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

type gatedPlugin struct {
	pluginsdk.BasePlugin
}

func (g *gatedPlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:      "gated-plugin",
		Name:    "Gated Plugin",
		Version: "1.0.0",
		SettingsSchema: map[string]any{
			"threshold": map[string]any{
				"type":    "number",
				"default": 42,
			},
		},
		Routes: []pluginsdk.RouteDef{
			{Method: http.MethodGet, Path: "/secret", Permission: "read:system:audit_logs"},
		},
	}
}

func (g *gatedPlugin) Init(ctx context.Context, host pluginsdk.Host) error {
	g.Host = host
	host.MountRoute(http.MethodGet, "/secret", func(w http.ResponseWriter, r *http.Request) {
		val, _ := host.GetSetting("threshold")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "threshold": val})
	})
	return nil
}

func TestPluginHandler_RoutePermissionAndSchemaDefaults(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := plugins.NewManager(tempDir, content.NewEventBus(), fields.NewRegistry(), nil, logger)
	require.NoError(t, mgr.RegisterNative(&gatedPlugin{}))
	require.NoError(t, mgr.BootstrapPlugins(context.Background(), "gated-plugin"))

	handler := plugins.NewHandler(mgr)
	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) {
		handler.RegisterRoutes(api)
	})

	// 1. Unauthenticated request -> 401
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/ext/gated-plugin/secret", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 2. Authenticated viewer without permission -> 403
	viewer := &auth.User{
		ID:     "usr-viewer",
		Status: auth.StatusActive,
		Roles:  []auth.Role{{Slug: auth.RoleViewer}},
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/plugins/ext/gated-plugin/secret", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, viewer))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 3. SuperAdmin -> 200 with applied schema default threshold=42
	admin := createAdminUser()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/plugins/ext/gated-plugin/secret", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"threshold":42`)
}

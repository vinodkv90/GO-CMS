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


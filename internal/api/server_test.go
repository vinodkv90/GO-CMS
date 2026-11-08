package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock DB for API tests
type mockHealthDB struct {
	pingErr error
	driver  string
	caps    interfaces.Capabilities
}

func (m *mockHealthDB) DriverName() string                    { return m.driver }
func (m *mockHealthDB) Ping(ctx context.Context) error        { return m.pingErr }
func (m *mockHealthDB) Close() error                          { return nil }
func (m *mockHealthDB) Capabilities() interfaces.Capabilities { return m.caps }

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
	assert.NotEmpty(t, body["time"])
}

func TestReadinessHandler_Healthy(t *testing.T) {
	db := &mockHealthDB{driver: "postgres"}
	handler := ReadinessHandler(db)

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ready", body["status"])

	components := body["components"].(map[string]any)
	dbComp := components["database"].(map[string]any)
	assert.Equal(t, "healthy", dbComp["status"])
	assert.Equal(t, "postgres", dbComp["driver"])
}

func TestReadinessHandler_Unhealthy(t *testing.T) {
	db := &mockHealthDB{
		driver:  "postgres",
		pingErr: errors.New("connection refused"),
	}
	handler := ReadinessHandler(db)

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body map[string]any
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "not_ready", body["status"])

	components := body["components"].(map[string]any)
	dbComp := components["database"].(map[string]any)
	assert.Equal(t, "unhealthy", dbComp["status"])
	assert.Equal(t, "connection refused", dbComp["error"])
}

func TestInfoHandler(t *testing.T) {
	cfg := config.DefaultConfig()
	caps := interfaces.NewCapabilitySet(interfaces.CapTransactions, interfaces.CapForeignKeys)
	db := &mockHealthDB{driver: "postgres", caps: caps}

	handler := InfoHandler(cfg, db)
	req := httptest.NewRequest("GET", "/info", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "Go CMS Platform", body["name"])
	assert.Equal(t, CMSVersion, body["version"])
	assert.Equal(t, "development", body["environment"])
}

func TestSecurityHeadersAndCORS(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &mockHealthDB{driver: "postgres"}

	srv := NewServer(cfg, db, nil, nil, nil, nil, nil, logger)

	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	srv.Router().ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, "SAMEORIGIN", resp.Header.Get("X-Frame-Options"))
	assert.Equal(t, "1; mode=block", resp.Header.Get("X-XSS-Protection"))
	assert.NotEmpty(t, resp.Header.Get("Content-Security-Policy"))
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestApiV1Ping(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &mockHealthDB{driver: "postgres"}

	srv := NewServer(cfg, db, nil, nil, nil, nil, nil, logger)

	req := httptest.NewRequest("GET", "/api/v1/ping", nil)
	w := httptest.NewRecorder()

	srv.Router().ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "pong", body["message"])
}

func TestAdminSPARoutes(t *testing.T) {
	if findAdminDist() == "" {
		t.Skip("skipping admin SPA test: admin/dist is not built")
	}

	cfg := config.DefaultConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &mockHealthDB{driver: "postgres"}

	srv := NewServer(cfg, db, nil, nil, nil, nil, nil, logger)

	// Test 1: /admin redirects to /admin/
	req1 := httptest.NewRequest("GET", "/admin", nil)
	w1 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w1, req1)
	resp1 := w1.Result()
	defer resp1.Body.Close()
	assert.Equal(t, http.StatusMovedPermanently, resp1.StatusCode)
	assert.Equal(t, "/admin/", resp1.Header.Get("Location"))

	// Test 2: /admin/ serves index.html
	req2 := httptest.NewRequest("GET", "/admin/", nil)
	w2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w2, req2)
	resp2 := w2.Result()
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	bodyBytes, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	assert.Contains(t, string(bodyBytes), "Go CMS - Admin Console")

	// Test 3: Client-side routing fallback /admin/content-types serves index.html
	req3 := httptest.NewRequest("GET", "/admin/content-types", nil)
	w3 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w3, req3)
	resp3 := w3.Result()
	defer resp3.Body.Close()
	assert.Equal(t, http.StatusOK, resp3.StatusCode)
	bodyBytes3, err := io.ReadAll(resp3.Body)
	require.NoError(t, err)
	assert.Contains(t, string(bodyBytes3), "Go CMS - Admin Console")
}


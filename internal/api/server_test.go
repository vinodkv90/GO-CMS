package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-cms/cms/admin"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/go-cms/cms/internal/fields"
	"github.com/go-cms/cms/internal/media"
	"github.com/go-cms/cms/internal/plugins"
	"github.com/go-cms/cms/internal/storage"
	"github.com/go-cms/cms/internal/storage/local"
	seoanalyzer "github.com/go-cms/cms/plugins/seo-analyzer"
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
	if findAdminDist() == "" && !admin.HasEmbeddedAdmin() {
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

func TestProductionCapabilitiesIntegration(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &mockHealthDB{driver: "postgres"}

	srv := NewServer(cfg, db, nil, nil, nil, nil, nil, logger)

	// Verify modules initialized
	assert.NotNil(t, srv.Cache())
	assert.NotNil(t, srv.Jobs())
	assert.NotNil(t, srv.Localization())
	assert.NotNil(t, srv.Search())
	assert.NotNil(t, srv.Metrics())
	assert.NotNil(t, srv.Audit())
	assert.NotNil(t, srv.Tenancy())

	// Test 1: GET /api/v1/locales
	req1 := httptest.NewRequest("GET", "/api/v1/locales", nil)
	w1 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	var locResp map[string]any
	err := json.Unmarshal(w1.Body.Bytes(), &locResp)
	require.NoError(t, err)
	data := locResp["data"].([]any)
	assert.GreaterOrEqual(t, len(data), 1)

	// Test 2: GET /api/v1/search?q=hello
	req2 := httptest.NewRequest("GET", "/api/v1/search?q=hello", nil)
	w2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Test 3: GET /metrics (Prometheus scrape endpoint)
	req3 := httptest.NewRequest("GET", "/metrics", nil)
	w3 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)
	bodyBytes, err := io.ReadAll(w3.Body)
	require.NoError(t, err)
	assert.Contains(t, string(bodyBytes), "cms_http_requests_total")

	// Test 4: GET /docs (Interactive Swagger documentation viewer)
	reqDocs := httptest.NewRequest("GET", "/docs", nil)
	wDocs := httptest.NewRecorder()
	srv.Router().ServeHTTP(wDocs, reqDocs)
	assert.Equal(t, http.StatusOK, wDocs.Code)
	docsBody, _ := io.ReadAll(wDocs.Body)
	assert.Contains(t, string(docsBody), "Go CMS - API Documentation")

	// Test 5: GET /api/v1/openapi.yaml (Raw OpenAPI spec)
	reqSpec := httptest.NewRequest("GET", "/api/v1/openapi.yaml", nil)
	wSpec := httptest.NewRecorder()
	srv.Router().ServeHTTP(wSpec, reqSpec)
	assert.Equal(t, http.StatusOK, wSpec.Code)

	// Test 6: Clean shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err = srv.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestLiveCORSRateLimitAndAuthLimiterUpdates(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.JWTSecret = "test-live-settings-jwt-secret-123456"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &mockHealthDB{driver: "postgres"}

	authRepo := auth.NewMemoryAuthRepository()
	authSvc := auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, logger)
	_, err := authSvc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)

	srv := NewServer(cfg, db, authSvc, nil, nil, nil, nil, logger)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	_, adminJWT, err := authSvc.Authenticate(t.Context(), "admin", "Admin@CMS2026!")
	require.NoError(t, err)

	// 1. Update CORS live to allow only https://portal.example.com
	corsPayload := []byte(`{"enabled":true,"allowed_origins":["https://portal.example.com"],"allowed_methods":["GET","POST","PUT","OPTIONS"],"allowed_headers":["Authorization","Content-Type"],"allow_credentials":true,"max_age_seconds":300}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/cors", bytes.NewReader(corsPayload))
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Allowed origin gets CORS header
	reqAllowed := httptest.NewRequest(http.MethodOptions, "/api/v1/ping", nil)
	reqAllowed.Header.Set("Origin", "https://portal.example.com")
	reqAllowed.Header.Set("Access-Control-Request-Method", "GET")
	wAllowed := httptest.NewRecorder()
	srv.Router().ServeHTTP(wAllowed, reqAllowed)
	assert.Equal(t, "https://portal.example.com", wAllowed.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", wAllowed.Header().Get("Access-Control-Allow-Credentials"))

	// Disallowed origin does not get Access-Control-Allow-Origin
	reqDenied := httptest.NewRequest(http.MethodOptions, "/api/v1/ping", nil)
	reqDenied.Header.Set("Origin", "https://attacker.example.com")
	reqDenied.Header.Set("Access-Control-Request-Method", "GET")
	wDenied := httptest.NewRecorder()
	srv.Router().ServeHTTP(wDenied, reqDenied)
	assert.Empty(t, wDenied.Header().Get("Access-Control-Allow-Origin"))

	// 2. Update Auth rate limit live to burst=2 and immediately enforce 429 on 3rd POST /auth/login
	rlPayload := []byte(`{"enabled":true,"global_requests_per_sec":100,"global_burst":200,"auth_requests_per_min":10,"auth_burst":2}`)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/settings/rate-limit", bytes.NewReader(rlPayload))
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	for i := 0; i < 3; i++ {
		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte(`{"identifier":"admin","password":"wrong"}`)))
		loginReq.RemoteAddr = "198.51.100.77:54321"
		loginReq.Header.Set("Content-Type", "application/json")
		lw := httptest.NewRecorder()
		srv.Router().ServeHTTP(lw, loginReq)
		if i < 2 {
			assert.NotEqual(t, http.StatusTooManyRequests, lw.Code)
		} else {
			assert.Equal(t, http.StatusTooManyRequests, lw.Code)
		}
	}
}

func TestLiveStorageSwapAndInviteEmailDelivery(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.JWTSecret = "test-live-m4-jwt-secret-123456"
	cfg.Auth.SessionSecret = "test-live-m4-session-secret-123456"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &mockHealthDB{driver: "postgres"}

	authRepo := auth.NewMemoryAuthRepository()
	authSvc := auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, logger)
	_, err := authSvc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)

	localProvider, err := local.New(t.TempDir(), "/uploads")
	require.NoError(t, err)
	swappable := storage.NewSwappable(localProvider)
	mediaSvc := media.NewService(media.NewMemoryRepository(), swappable, cfg, logger)

	srv := NewServer(cfg, db, authSvc, nil, mediaSvc, nil, nil, logger)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	_, adminJWT, err := authSvc.Authenticate(t.Context(), "admin", "Admin@CMS2026!")
	require.NoError(t, err)

	// 1. Swapping storage settings to MinIO immediately updates SwappableProvider.Name()
	assert.Equal(t, "local", swappable.Name())
	s3Mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><Name>assets</Name></ListBucketResult>`))
	}))
	defer s3Mock.Close()

	stBody, _ := json.Marshal(map[string]any{
		"active_provider": "minio",
		"local":           map[string]any{"upload_dir": t.TempDir(), "url_prefix": "/uploads"},
		"s3": map[string]any{
			"bucket":            "assets",
			"region":            "us-east-1",
			"endpoint":          s3Mock.URL,
			"access_key_id":     "minio",
			"secret_access_key": "minio12345",
			"use_ssl":           false,
			"force_path_style":  true,
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/storage", bytes.NewReader(stBody))
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "minio", swappable.Name())

	// 2. Creating an admin invitation delivers email through wired Email service and reports email_sent:true
	role, err := authRepo.GetRoleBySlug(t.Context(), auth.RoleAuthor)
	require.NoError(t, err)
	invBody, _ := json.Marshal(map[string]any{
		"email":    "invited.writer@example.org",
		"username": "writer1",
		"role_ids": []string{role.ID},
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/invitations", bytes.NewReader(invBody))
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), `"email_sent":true`)
}

func TestLiveM5SecurityPolicyAndUsersPermIsolation(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.JWTSecret = "test-live-m5-jwt-secret-123456"
	cfg.Auth.SessionSecret = "test-live-m5-session-secret-123456"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &mockHealthDB{driver: "postgres"}

	authRepo := auth.NewMemoryAuthRepository()
	authSvc := auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, logger)
	_, err := authSvc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)

	srv := NewServer(cfg, db, authSvc, nil, nil, nil, nil, logger)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	_, adminJWT, err := authSvc.Authenticate(t.Context(), "admin", "Admin@CMS2026!")
	require.NoError(t, err)

	// 1. Live Password Policy enforces special character across /api/v1/auth/register once updated
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/password-policy", strings.NewReader(`{"min_length":10,"require_uppercase":true,"require_lowercase":true,"require_number":true,"require_special":true,"max_age_days":0}`))
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/app-auth/register", strings.NewReader(`{"username":"weakapp","email":"weak@app.test","password":"NoSpecialChar12"}`))
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 2. Valid App User registers & isolates cleanly from Admin JWT
	req = httptest.NewRequest(http.MethodPost, "/api/v1/app-auth/register", strings.NewReader(`{"username":"goodapp","email":"good@app.test","password":"Good!AppPass2026"}`))
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var appReg struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &appReg))
	require.NotEmpty(t, appReg.Token)

	// App JWT blocked from Admin RBAC route; Admin JWT blocked from /app-auth/me
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users-permissions/roles", nil)
	req.Header.Set("Authorization", "Bearer "+appReg.Token)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/app-auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Admin can read Users & Permissions roles and public SSO status
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users-permissions/roles", nil)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"authenticated"`)
}

func TestLiveM6WebhooksPluginsAndLocales(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.JWTSecret = "test-live-m6-jwt-secret-123456"
	cfg.Auth.SessionSecret = "test-live-m6-session-secret-123456"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &mockHealthDB{driver: "postgres"}

	authRepo := auth.NewMemoryAuthRepository()
	authSvc := auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, logger)
	_, err := authSvc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)

	_, adminJWT, err := authSvc.Authenticate(t.Context(), "admin", "Admin@CMS2026!")
	require.NoError(t, err)

	pluginMgr := plugins.NewManager(t.TempDir(), content.NewEventBus(), fields.NewRegistry(), db, logger)
	require.NoError(t, pluginMgr.RegisterNative(seoanalyzer.New()))
	require.NoError(t, pluginMgr.BootstrapPlugins(t.Context(), "seo-analyzer"))

	srv := NewServer(cfg, db, authSvc, nil, nil, pluginMgr, nil, logger)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	// 1. Webhooks SSRF protection + CRUD + masked secret + events endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/events", nil)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"entry.publish"`)

	ssrfReq := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewBufferString(`{"name":"Local","url":"http://127.0.0.1:9999/hook","events":["entry.create"]}`))
	ssrfReq.Header.Set("Authorization", "Bearer "+adminJWT)
	ssrfReq.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, ssrfReq)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	createWH := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewBufferString(`{"name":"Deploy Hook","url":"https://hooks.example.com/deploy","events":["entry.publish"],"secret":"topsecret"}`))
	createWH.Header.Set("Authorization", "Bearer "+adminJWT)
	createWH.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, createWH)
	require.Equal(t, http.StatusCreated, w.Code)
	assert.NotContains(t, w.Body.String(), "topsecret")
	assert.Contains(t, w.Body.String(), `"secret_configured":true`)

	// 2. Plugin custom route RBAC (/plugins/ext/seo-analyzer/analyze requires read:plugins)
	unauthExt := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/ext/seo-analyzer/analyze", bytes.NewBufferString(`{"title":"Hello World","content":"Short"}`))
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, unauthExt)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	authExt := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/ext/seo-analyzer/analyze", bytes.NewBufferString(`{"title":"Comprehensive SEO Guide For Enterprise Systems","content":"word "}`))
	authExt.Header.Set("Authorization", "Bearer "+adminJWT)
	authExt.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, authExt)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"score"`)

	// 3. Localization create + update + default guard
	addLoc := httptest.NewRequest(http.MethodPost, "/api/v1/locales", bytes.NewBufferString(`{"code":"fr","name":"French","native_name":"Français","direction":"ltr"}`))
	addLoc.Header.Set("Authorization", "Bearer "+adminJWT)
	addLoc.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, addLoc)
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), `"native_name":"Français"`)

	// Cannot delete default "en" locale
	delEn := httptest.NewRequest(http.MethodDelete, "/api/v1/locales/en", nil)
	delEn.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, delEn)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Deleting "fr" succeeds
	delFr := httptest.NewRequest(http.MethodDelete, "/api/v1/locales/fr", nil)
	delFr.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, delFr)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestLiveM7PersistentAuditLogs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.JWTSecret = "test-live-m7-jwt-secret-123456"
	cfg.Auth.SessionSecret = "test-live-m7-session-secret-123456"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &mockHealthDB{driver: "postgres"}

	authRepo := auth.NewMemoryAuthRepository()
	authSvc := auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, logger)
	_, err := authSvc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)

	pluginMgr := plugins.NewManager(t.TempDir(), content.NewEventBus(), fields.NewRegistry(), db, logger)
	srv := NewServer(cfg, db, authSvc, nil, nil, pluginMgr, nil, logger)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	// 1. Trigger a failed login -> audit failure entry
	badLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"identifier":"admin","password":"WrongPassword!"}`))
	badLogin.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, badLogin)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 2. Valid admin login
	_, adminJWT, err := authSvc.Authenticate(t.Context(), "admin", "Admin@CMS2026!")
	require.NoError(t, err)

	// 3. Filter /api/v1/audit-logs?result=failure (no unauthenticated access)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	failReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?result=failure", nil)
	failReq.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, failReq)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"result":"failure"`)

	// 4. Update Email settings with sensitive secret -> ensure /api/v1/audit-logs redacts secrets
	emailReq := httptest.NewRequest(http.MethodPut, "/api/v1/settings/email", bytes.NewBufferString(`{"provider":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_username":"mailer","smtp_password":"UltraSecretSMTPPassword99","from_email":"noreply@example.com","from_name":"Go CMS","reply_to":""}`))
	emailReq.Header.Set("Authorization", "Bearer "+adminJWT)
	emailReq.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, emailReq)
	require.Equal(t, http.StatusOK, w.Code)

	// Verify /api/v1/audit-logs never exposes raw secrets
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?limit=25", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, listReq)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "UltraSecretSMTPPassword99")

	// 5. CSV export works and returns text/csv
	expReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs/export", nil)
	expReq.Header.Set("Authorization", "Bearer "+adminJWT)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, expReq)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")
	assert.Contains(t, w.Body.String(), "actor_id,actor_type,action")
	assert.NotContains(t, w.Body.String(), "UltraSecretSMTPPassword99")
}

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSettingsTestRouter(t *testing.T, user *auth.User, apiToken *auth.APIToken) (chi.Router, *Service) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Database.Password = "DO_NOT_LEAK_DB_PASS"
	cfg.Auth.JWTSecret = "DO_NOT_LEAK_JWT_SECRET"
	svc := NewService(NewMemoryRepository(), cfg, nil, RuntimeStatusProvider{
		CMSVersion: "1.0.0",
		DatabaseStatus: func(context.Context) (string, string, string) {
			return "postgres", "healthy", ""
		},
	})
	h := NewHTTPHandler(svc)

	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)
	r.Group(func(pr chi.Router) {
		if user != nil || apiToken != nil {
			pr.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					ctx := req.Context()
					if user != nil {
						ctx = context.WithValue(ctx, auth.UserContextKey, user)
					}
					if apiToken != nil {
						ctx = context.WithValue(ctx, auth.APITokenContextKey, apiToken)
					}
					next.ServeHTTP(w, req.WithContext(ctx))
				})
			})
		}
		h.RegisterProtectedRoutes(pr)
	})
	return r, svc
}

func TestSettingsHTTPHandlersAndRBAC(t *testing.T) {
	viewer := &auth.User{
		ID:     "viewer-1",
		Status: auth.StatusActive,
		Roles: []auth.Role{
			{Slug: auth.RoleViewer, Permissions: []auth.Permission{{Action: "read", Resource: "content:*"}}},
		},
	}
	admin := &auth.User{
		ID:     "admin-1",
		Status: auth.StatusActive,
		Roles:  []auth.Role{{Slug: auth.RoleSuperAdmin}},
	}

	viewerRouter, _ := setupSettingsTestRouter(t, viewer, nil)
	adminRouter, _ := setupSettingsTestRouter(t, admin, nil)

	// Public route works unauthenticated.
	pubReq := httptest.NewRequest(http.MethodGet, "/settings/public", nil)
	pubRec := httptest.NewRecorder()
	viewerRouter.ServeHTTP(pubRec, pubReq)
	assert.Equal(t, http.StatusOK, pubRec.Code)
	assert.Contains(t, pubRec.Body.String(), "Go CMS")

	// Viewer cannot read/update settings or system info.
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/settings/general"},
		{http.MethodPut, "/settings/general"},
		{http.MethodGet, "/settings/overview"},
		{http.MethodGet, "/settings/system"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		viewerRouter.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s", tc.method, tc.path)
	}

	// Viewer CAN manage their own preferences.
	prefBody := `{"theme":"dark","preferred_language":"en","timezone":"UTC","date_format":"YYYY-MM-DD","time_format":"24h","sidebar_collapsed":true}`
	prefReq := httptest.NewRequest(http.MethodPut, "/preferences", strings.NewReader(prefBody))
	prefRec := httptest.NewRecorder()
	viewerRouter.ServeHTTP(prefRec, prefReq)
	assert.Equal(t, http.StatusOK, prefRec.Code)
	assert.Contains(t, prefRec.Body.String(), `"theme":"dark"`)

	// Super Admin updates general settings and sees persisted change + zero secret leakage in system info.
	payload := map[string]any{
		"site_name":        "Enterprise Portal",
		"site_description": "Headless CMS",
		"public_url":       "https://portal.example.com",
		"admin_path":       "/admin",
		"default_timezone": "UTC",
		"default_locale":   "en",
		"date_format":      "YYYY-MM-DD",
		"time_format":      "24h",
	}
	raw, _ := json.Marshal(payload)
	putReq := httptest.NewRequest(http.MethodPut, "/settings/general", bytes.NewReader(raw))
	putRec := httptest.NewRecorder()
	adminRouter.ServeHTTP(putRec, putReq)
	require.Equal(t, http.StatusOK, putRec.Code)

	getPubReq := httptest.NewRequest(http.MethodGet, "/settings/public", nil)
	getPubRec := httptest.NewRecorder()
	adminRouter.ServeHTTP(getPubRec, getPubReq)
	assert.Equal(t, http.StatusOK, getPubRec.Code)
	assert.Contains(t, getPubRec.Body.String(), "Enterprise Portal")

	sysReq := httptest.NewRequest(http.MethodGet, "/settings/system", nil)
	sysRec := httptest.NewRecorder()
	adminRouter.ServeHTTP(sysRec, sysReq)
	assert.Equal(t, http.StatusOK, sysRec.Code)
	assert.NotContains(t, sysRec.Body.String(), "DO_NOT_LEAK_DB_PASS")
	assert.NotContains(t, sysRec.Body.String(), "DO_NOT_LEAK_JWT_SECRET")
}

func TestAPIConfigSettingsEndpointsRBACAndValidation(t *testing.T) {
	viewerRouter, _ := setupSettingsTestRouter(t, &auth.User{
		ID:     "viewer-1",
		Status: auth.StatusActive,
		Roles: []auth.Role{
			{Slug: auth.RoleViewer, Permissions: []auth.Permission{{Action: "read", Resource: "content:*"}}},
		},
	}, nil)
	adminRouter, _ := setupSettingsTestRouter(t, &auth.User{
		ID:     "admin-1",
		Status: auth.StatusActive,
		Roles:  []auth.Role{{Slug: auth.RoleSuperAdmin}},
	}, nil)

	// Viewer is blocked from reading/updating API configuration settings
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/settings/rest-api"},
		{http.MethodPut, "/settings/rest-api"},
		{http.MethodGet, "/settings/cors"},
		{http.MethodPut, "/settings/cors"},
		{http.MethodGet, "/settings/rate-limit"},
		{http.MethodPut, "/settings/rate-limit"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		viewerRouter.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	}

	// Wildcard CORS + allow_credentials=true returns 400
	badCORS := `{"enabled":true,"allowed_origins":["*"],"allow_credentials":true,"max_age_seconds":300}`
	req := httptest.NewRequest(http.MethodPut, "/settings/cors", strings.NewReader(badCORS))
	rec := httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Valid REST API, CORS, Rate Limit updates succeed and persist
	goodRest := `{"default_limit":15,"max_limit":50,"with_count":false,"max_populate_depth":3,"expose_draft_preview":false}`
	req = httptest.NewRequest(http.MethodPut, "/settings/rest-api", strings.NewReader(goodRest))
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/settings/rest-api", nil)
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"default_limit":15`)
	assert.Contains(t, rec.Body.String(), `"with_count":false`)
}

func TestSettingsHTTP_MediaAndStorageTestEndpoint(t *testing.T) {
	// Mock S3 endpoint answering ListObjectsV2 health check
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("Authorization"), "AWS4-HMAC-SHA256")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><Name>cms-bucket</Name></ListBucketResult>`))
	}))
	defer s3Server.Close()

	adminRouter, _ := setupSettingsTestRouter(t, &auth.User{
		ID:     "admin-1",
		Status: auth.StatusActive,
		Roles:  []auth.Role{{Slug: auth.RoleSuperAdmin}},
	}, nil)

	// 1. Update Media settings via HTTP
	mediaPayload := `{"max_file_size_bytes":2048000,"allowed_mime_types":["image/png","image/jpeg"],"responsive_sizes_enabled":true,"generate_small":true,"generate_medium":true,"generate_large":false,"require_alt_text":true,"default_access":"signed","signed_url_ttl_seconds":600}`
	req := httptest.NewRequest(http.MethodPut, "/settings/media", strings.NewReader(mediaPayload))
	rec := httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"require_alt_text":true`)
	assert.Contains(t, rec.Body.String(), `"default_access":"signed"`)

	// 2. Save MinIO/S3 storage settings and ensure secret is never returned in JSON
	storagePayload, _ := json.Marshal(map[string]any{
		"active_provider": "minio",
		"local":           map[string]any{"upload_dir": t.TempDir(), "url_prefix": "/uploads"},
		"s3": map[string]any{
			"bucket":            "cms-bucket",
			"region":            "us-east-1",
			"endpoint":          s3Server.URL,
			"access_key_id":     "minioadmin",
			"secret_access_key": "minio-real-secret-key",
			"use_ssl":           false,
			"force_path_style":  true,
		},
	})
	req = httptest.NewRequest(http.MethodPut, "/settings/storage", bytes.NewReader(storagePayload))
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "minio-real-secret-key")
	assert.Contains(t, rec.Body.String(), SecretPreserveSentinel)

	// 3. Test storage connection using preserved sentinel secret
	testPayload, _ := json.Marshal(map[string]any{
		"active_provider": "minio",
		"local":           map[string]any{"upload_dir": t.TempDir(), "url_prefix": "/uploads"},
		"s3": map[string]any{
			"bucket":            "cms-bucket",
			"region":            "us-east-1",
			"endpoint":          s3Server.URL,
			"access_key_id":     "minioadmin",
			"secret_access_key": SecretPreserveSentinel,
			"use_ssl":           false,
			"force_path_style":  true,
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/settings/storage/test", bytes.NewReader(testPayload))
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok":true`)
	assert.Contains(t, rec.Body.String(), `"provider":"minio"`)
}

func TestSettingsHTTP_SecurityPasswordPolicyAndSSOEndpoints(t *testing.T) {
	adminRouter, _ := setupSettingsTestRouter(t, &auth.User{
		ID:     "admin-1",
		Status: auth.StatusActive,
		Roles:  []auth.Role{{Slug: auth.RoleSuperAdmin}},
	}, nil)

	// Security policy PUT/GET
	secPayload := `{"session_timeout_minutes":90,"max_failed_logins":3,"lockout_duration_minutes":20,"enforce_admin_mfa":true,"allowed_admin_ip_cidrs":["127.0.0.1","10.10.0.0/16"]}`
	req := httptest.NewRequest(http.MethodPut, "/settings/security", strings.NewReader(secPayload))
	rec := httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"enforce_admin_mfa":true`)
	assert.Contains(t, rec.Body.String(), `"127.0.0.1/32"`)

	req = httptest.NewRequest(http.MethodGet, "/settings/security", nil)
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"session_timeout_minutes":90`)

	// Password policy PUT/GET
	pwPayload := `{"min_length":10,"require_uppercase":true,"require_lowercase":true,"require_number":true,"require_special":true,"max_age_days":60}`
	req = httptest.NewRequest(http.MethodPut, "/settings/password-policy", strings.NewReader(pwPayload))
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"require_special":true`)

	req = httptest.NewRequest(http.MethodGet, "/settings/password-policy", nil)
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"min_length":10`)

	// SSO PUT/GET
	ssoPayload := `{"enabled":true,"auto_provision_admin":true,"default_role_id":"role-editor","allowed_domains":["@Acme.Org"],"enforce_sso_for_domains":true}`
	req = httptest.NewRequest(http.MethodPut, "/settings/sso", strings.NewReader(ssoPayload))
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"acme.org"`)
	assert.Contains(t, rec.Body.String(), `"enforce_sso_for_domains":true`)

	req = httptest.NewRequest(http.MethodGet, "/settings/sso", nil)
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"default_role_id":"role-editor"`)
}

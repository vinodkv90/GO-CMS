package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRouter(svc *Service) *chi.Mux {
	r := chi.NewRouter()
	r.Use(AuthMiddleware(svc))
	handler := NewHandler(svc)
	handler.RegisterRoutes(r)
	return r
}

func TestHandler_RegisterAndLoginFlow(t *testing.T) {
	svc, _ := setupTestAuthService(t)
	router := setupTestRouter(svc)

	// 1. Register User
	registerPayload := map[string]string{
		"username":   "alice",
		"email":      "alice@example.com",
		"password":   "AliceSecret123!",
		"first_name": "Alice",
		"last_name":  "Smith",
	}
	body, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var regResp User
	err := json.Unmarshal(w.Body.Bytes(), &regResp)
	require.NoError(t, err)
	assert.Equal(t, "alice", regResp.Username)

	// 2. Login User
	loginPayload := map[string]string{
		"identifier": "alice",
		"password":   "AliceSecret123!",
	}
	body, _ = json.Marshal(loginPayload)
	req = httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var loginResp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &loginResp)
	require.NoError(t, err)
	token := loginResp["token"].(string)
	assert.NotEmpty(t, token)

	// 3. Access Protected /auth/me with Bearer Token
	req = httptest.NewRequest("GET", "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var meResp User
	err = json.Unmarshal(w.Body.Bytes(), &meResp)
	require.NoError(t, err)
	assert.Equal(t, "alice", meResp.Username)

	// 4. Access Protected /auth/me without Token (expect 401)
	req = httptest.NewRequest("GET", "/auth/me", nil)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_APITokenFlow(t *testing.T) {
	svc, _ := setupTestAuthService(t)
	router := setupTestRouter(svc)

	// Register & Login User
	user, err := svc.Register(t.Context(), RegisterRequest{
		Username: "bob",
		Email:    "bob@example.com",
		Password: "BobSecret123!",
	})
	require.NoError(t, err)

	_, token, err := svc.Authenticate(t.Context(), "bob", "BobSecret123!")
	require.NoError(t, err)

	// Create API Token via endpoint
	tokenReq := map[string]any{
		"name":             "Bob Automation Key",
		"permissions":      []string{"read:content"},
		"duration_seconds": 3600,
	}
	body, _ := json.Marshal(tokenReq)
	req := httptest.NewRequest("POST", "/api-tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createResp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &createResp)
	require.NoError(t, err)
	patToken := createResp["token"].(string)
	assert.True(t, IsAPIToken(patToken))

	// Access /auth/me using Personal Access Token
	req = httptest.NewRequest("GET", "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+patToken)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var meResp User
	err = json.Unmarshal(w.Body.Bytes(), &meResp)
	require.NoError(t, err)
	assert.Equal(t, user.ID, meResp.ID)

	// List API Tokens
	req = httptest.NewRequest("GET", "/api-tokens", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_SecurityAndRBACEnforcement(t *testing.T) {
	svc, _ := setupTestAuthService(t)
	router := setupTestRouter(svc)

	// Seed roles & admin
	admin, err := svc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)

	_, adminToken, err := svc.Authenticate(t.Context(), "admin", "Admin@CMS2026!")
	require.NoError(t, err)

	// Register regular non-admin user
	nonAdmin, err := svc.Register(t.Context(), RegisterRequest{
		Username: "regularuser",
		Email:    "reg@example.com",
		Password: "RegUser2026!",
	})
	require.NoError(t, err)

	_, regToken, err := svc.Authenticate(t.Context(), "regularuser", "RegUser2026!")
	require.NoError(t, err)

	// 1. Regular user attempting to access admin-only /users endpoint (Forbidden 403)
	req := httptest.NewRequest("GET", "/users", nil)
	req.Header.Set("Authorization", "Bearer "+regToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Forbidden: insufficient permissions")

	// 2. Admin user accessing /users endpoint (OK 200)
	reqAdmin := httptest.NewRequest("GET", "/users", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+adminToken)
	wAdmin := httptest.NewRecorder()
	router.ServeHTTP(wAdmin, reqAdmin)
	assert.Equal(t, http.StatusOK, wAdmin.Code)

	// 3. Tampered token (header/signature modified)
	tamperedToken := adminToken[:len(adminToken)-5] + "XXXXX"
	reqTampered := httptest.NewRequest("GET", "/auth/me", nil)
	reqTampered.Header.Set("Authorization", "Bearer "+tamperedToken)
	wTampered := httptest.NewRecorder()
	router.ServeHTTP(wTampered, reqTampered)
	assert.Equal(t, http.StatusUnauthorized, wTampered.Code)

	// 4. Malformed Authorization headers
	for _, badHeader := range []string{"Bearer", "Basic xyz", "Bearer   ", "InvalidPrefix token"} {
		reqBad := httptest.NewRequest("GET", "/auth/me", nil)
		reqBad.Header.Set("Authorization", badHeader)
		wBad := httptest.NewRecorder()
		router.ServeHTTP(wBad, reqBad)
		assert.Equal(t, http.StatusUnauthorized, wBad.Code, "header %q should be unauthorized", badHeader)
	}

	_ = admin
	_ = nonAdmin
}

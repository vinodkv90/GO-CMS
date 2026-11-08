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

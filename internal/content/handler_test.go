package content

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/fields"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestHTTPRouter(t *testing.T) (*chi.Mux, *Service, *auth.Service, string) {
	// Auth Setup
	authRepo := auth.NewMemoryAuthRepository()
	cfg := config.DefaultConfig()
	cfg.Auth.JWTSecret = "jwt-secret-for-content-tests"
	authSvc := auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, nil)

	adminUser, err := authSvc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin1234!")
	require.NoError(t, err)

	_, token, err := authSvc.Authenticate(t.Context(), "admin", "Admin1234!")
	require.NoError(t, err)
	_ = adminUser

	// Content Setup
	contentRepo := NewMemoryContentRepository()
	contentSvc := NewService(contentRepo, contentRepo, contentRepo, fields.NewRegistry(), NewEventBus(), nil)

	r := chi.NewRouter()
	r.Use(auth.AuthMiddleware(authSvc))

	handler := NewHandler(contentSvc)
	handler.RegisterRoutes(r)

	return r, contentSvc, authSvc, token
}

func TestContentHTTP_FullFlow(t *testing.T) {
	router, _, _, token := setupTestHTTPRouter(t)

	// 1. Create Content Type: "product"
	typePayload := CreateTypeRequest{
		UID:  "product",
		Name: "Products",
		Kind: KindCollection,
		Fields: []ContentField{
			{Name: "Name", FieldKey: "name", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Required: true}},
			{Name: "Price", FieldKey: "price", FieldType: fields.TypeDecimal, Configuration: fields.ValidationRules{Required: true}},
		},
	}
	body, _ := json.Marshal(typePayload)
	req := httptest.NewRequest("POST", "/content-types", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// 2. Create Product Entry
	entryPayload := map[string]any{
		"name":  "Mechanical Keyboard",
		"price": 149.99,
	}
	body, _ = json.Marshal(entryPayload)
	req = httptest.NewRequest("POST", "/content/product", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var entryResp ContentEntry
	err := json.Unmarshal(w.Body.Bytes(), &entryResp)
	require.NoError(t, err)
	assert.Equal(t, StatusDraft, entryResp.Status)
	assert.Equal(t, "Mechanical Keyboard", entryResp.DraftData["name"])

	// 3. Public GET without preview returns 404 (not published yet)
	req = httptest.NewRequest("GET", "/content/product/"+entryResp.ID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 4. Authenticated preview GET returns draft
	req = httptest.NewRequest("GET", "/content/product/"+entryResp.ID+"?preview=true", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 5. Publish Entry
	req = httptest.NewRequest("POST", "/content/product/"+entryResp.ID+"/publish", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 6. Public GET succeeds now!
	req = httptest.NewRequest("GET", "/content/product/"+entryResp.ID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var pubResp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &pubResp)
	require.NoError(t, err)
	data := pubResp["data"].(map[string]any)
	assert.Equal(t, "Mechanical Keyboard", data["name"])

	// 7. Check Version History
	req = httptest.NewRequest("GET", "/content/product/"+entryResp.ID+"/versions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}


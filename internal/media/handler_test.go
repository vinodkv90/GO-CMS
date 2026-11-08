package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createAuthenticatedAdminUser() *auth.User {
	return &auth.User{
		ID:       "admin-user-id",
		Username: "admin",
		Status:   auth.StatusActive,
		Roles: []auth.Role{
			{
				Slug: auth.RoleSuperAdmin,
			},
		},
	}
}

func setupTestMediaAPI(t *testing.T, enableSignedURLs bool) (*chi.Mux, *media.Service) {
	svc, _, _, cfg := setupTestMediaService(t, enableSignedURLs)
	handler := media.NewHandler(svc, cfg)

	router := chi.NewRouter()
	router.Route("/api/v1", func(api chi.Router) {
		handler.RegisterRoutes(api)
	})
	handler.MountFileServer(router)

	return router, svc
}

func TestMediaHandler_FullFlow(t *testing.T) {
	router, _ := setupTestMediaAPI(t, false)
	admin := createAuthenticatedAdminUser()

	pngBytes := createTestPNG(200, 200)

	// 1. Upload File via POST /api/v1/media
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "company-logo.png")
	require.NoError(t, err)
	_, err = part.Write(pngBytes)
	require.NoError(t, err)
	_ = writer.WriteField("alt_text", "Company Logo")
	_ = writer.WriteField("caption", "Official brand logo")
	require.NoError(t, writer.Close())

	req := httptest.NewRequest("POST", "/api/v1/media", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var uploadResp struct {
		Data media.MediaAsset `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &uploadResp)
	require.NoError(t, err)
	assetID := uploadResp.Data.ID
	assert.NotEmpty(t, assetID)
	assert.Equal(t, "company-logo.png", uploadResp.Data.OriginalName)
	assert.Equal(t, "Company Logo", uploadResp.Data.Metadata.AltText)

	// 2. Query Asset Details via GET /api/v1/media/{id}
	req = httptest.NewRequest("GET", "/api/v1/media/"+assetID, nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 3. List Assets via GET /api/v1/media
	req = httptest.NewRequest("GET", "/api/v1/media?search=company", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResp struct {
		Data []media.MediaAsset `json:"data"`
		Meta map[string]any     `json:"meta"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &listResp)
	require.NoError(t, err)
	assert.Len(t, listResp.Data, 1)

	// 4. Update Asset via PATCH /api/v1/media/{id}
	patchPayload := []byte(`{"alt_text": "Updated Logo Alt", "caption": "Updated caption"}`)
	req = httptest.NewRequest("PATCH", "/api/v1/media/"+assetID, bytes.NewReader(patchPayload))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 5. Serve File via GET /uploads/*
	fileURL := uploadResp.Data.URL
	req = httptest.NewRequest("GET", fileURL, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age=86400")

	// 6. Delete Asset via DELETE /api/v1/media/{id}
	req = httptest.NewRequest("DELETE", "/api/v1/media/"+assetID, nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 7. Verify File is now 404
	req = httptest.NewRequest("GET", fileURL, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMediaHandler_SignedURLServing(t *testing.T) {
	router, svc := setupTestMediaAPI(t, true)
	admin := createAuthenticatedAdminUser()

	pngBytes := createTestPNG(50, 50)
	asset, err := svc.Upload(context.Background(), bytes.NewReader(pngBytes), "secure-chart.png", int64(len(pngBytes)), "", "")
	require.NoError(t, err)

	// 1. Request file with valid signed URL -> 200 OK
	req := httptest.NewRequest("GET", asset.URL, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 2. Request file without signature -> 403 Forbidden
	unsignedURL := "/uploads/" + asset.StorageKey
	req = httptest.NewRequest("GET", unsignedURL, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 3. Request file with tampered signature -> 403 Forbidden
	u, err := url.Parse(asset.URL)
	require.NoError(t, err)
	q := u.Query()
	q.Set("signature", "invalid-tampered-mac-signature")
	u.RawQuery = q.Encode()
	req = httptest.NewRequest("GET", u.String(), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 4. Test GET /api/v1/media/{id}/signed-url
	req = httptest.NewRequest("GET", "/api/v1/media/"+asset.ID+"/signed-url?ttl=300", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var sigResp struct {
		SignedURL string `json:"signed_url"`
		ExpiresIn int    `json:"expires_in"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &sigResp)
	require.NoError(t, err)
	assert.NotEmpty(t, sigResp.SignedURL)
	assert.Equal(t, 300, sigResp.ExpiresIn)
}

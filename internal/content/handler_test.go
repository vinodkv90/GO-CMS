package content

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	return setupTestHTTPRouterWithRegistry(t, fields.NewRegistry())
}

func setupTestHTTPRouterWithRegistry(t *testing.T, reg *fields.Registry) (*chi.Mux, *Service, *auth.Service, string) {
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
	contentSvc := NewService(contentRepo, contentRepo, contentRepo, reg, NewEventBus(), nil)

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

func TestContentHTTP_RedactsSecretsAndPrivatePublicFields(t *testing.T) {
	router, svc, _, token := setupTestHTTPRouter(t)
	ctx := t.Context()
	_, err := svc.CreateContentType(ctx, CreateTypeRequest{UID: "secrets", Name: "Secrets", Kind: KindCollection, Fields: []ContentField{
		{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText},
		{Name: "Password", FieldKey: "password", FieldType: fields.TypeSecret},
		{Name: "Private note", FieldKey: "private_note", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Private: true}},
	}})
	require.NoError(t, err)
	entry, err := svc.CreateEntry(ctx, "secrets", map[string]any{"title": "Safe", "password": "HiddenPassword123!", "private_note": "Internal"}, "admin")
	require.NoError(t, err)
	_, err = svc.PublishEntry(ctx, "secrets", entry.ID, "admin")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/content/secrets/"+entry.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var public map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &public))
	publicData := public["data"].(map[string]any)
	assert.Equal(t, "Safe", publicData["title"])
	assert.NotContains(t, publicData, "password")
	assert.NotContains(t, publicData, "private_note")

	req = httptest.NewRequest(http.MethodGet, "/content/secrets/"+entry.ID+"?preview=true", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var preview ContentEntry
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))
	assert.NotContains(t, preview.DraftData, "password")
	assert.Equal(t, "Internal", preview.DraftData["private_note"])
}

func TestContentHTTP_CustomFieldsSettingsOrderAndFieldErrors(t *testing.T) {
	reg := fields.NewRegistry()
	require.NoError(t, reg.RegisterCustom(fields.CustomFieldDescriptor{
		Type:       fields.FieldType("color_hex"),
		Label:      "Color Hex",
		PluginSlug: "colors",
		PluginName: "Colors",
	}, func(val any) error {
		s, ok := val.(string)
		if !ok || len(s) != 7 || s[0] != '#' {
			return fmt.Errorf("invalid hex color")
		}
		return nil
	}))

	router, _, _, token := setupTestHTTPRouterWithRegistry(t, reg)

	// 1. Custom fields endpoint + capabilities
	req := httptest.NewRequest(http.MethodGet, "/content-types/custom-fields", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var customList []fields.CustomFieldDescriptor
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &customList))
	require.Len(t, customList, 1)
	assert.Equal(t, fields.FieldType("color_hex"), customList[0].Type)

	req = httptest.NewRequest(http.MethodGet, "/content-types/field-capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var caps map[string]fields.FieldCapabilities
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &caps))
	assert.Contains(t, caps, "color_hex")

	// 2. Create component + collection type with custom field
	compPayload := CreateTypeRequest{
		UID:  "shared.badge",
		Name: "Badge",
		Kind: KindComponent,
		Fields: []ContentField{
			{Name: "Label", FieldKey: "label", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Required: true}},
		},
	}
	body, _ := json.Marshal(compPayload)
	req = httptest.NewRequest(http.MethodPost, "/content-types", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	typePayload := CreateTypeRequest{
		UID:  "themes",
		Name: "Themes",
		Kind: KindCollection,
		Fields: []ContentField{
			{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Required: true}},
			{Name: "Primary", FieldKey: "primary", FieldType: fields.FieldType("color_hex"), Configuration: fields.ValidationRules{Required: true}},
			{Name: "Badge", FieldKey: "badge", FieldType: fields.TypeComponent, Configuration: fields.ValidationRules{Component: "shared.badge"}},
		},
	}
	body, _ = json.Marshal(typePayload)
	req = httptest.NewRequest(http.MethodPost, "/content-types", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// 3. Persist settings.field_order via PUT /content-types/{uid}
	updatePayload := UpdateTypeRequest{
		Name: "Themes",
		Settings: map[string]any{
			"field_order": []string{"primary", "title", "badge"},
		},
	}
	body, _ = json.Marshal(updatePayload)
	req = httptest.NewRequest(http.MethodPut, "/content-types/themes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/content-types/themes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var fetched ContentType
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &fetched))
	assert.Equal(t, []any{"primary", "title", "badge"}, fetched.Settings["field_order"])

	// 4. Structured field error JSON for invalid custom field value
	badEntry := map[string]any{"title": "Dark", "primary": "blue"}
	body, _ = json.Marshal(badEntry)
	req = httptest.NewRequest(http.MethodPost, "/content/themes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	var errResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "primary", errResp["field"])
	fieldErrs, ok := errResp["fields"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, fieldErrs["primary"], "hex color")

	// 5. Structured nested component field error JSON
	badCompEntry := map[string]any{"title": "Dark", "primary": "#112233", "badge": map[string]any{}}
	body, _ = json.Marshal(badCompEntry)
	req = httptest.NewRequest(http.MethodPost, "/content/themes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	errResp = nil
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "badge.label", errResp["field"])
	fieldErrs, ok = errResp["fields"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, fieldErrs["badge.label"], "required")

	// 6. Valid entry succeeds
	goodEntry := map[string]any{
		"title":   "Ocean",
		"primary": "#0284c7",
		"badge":   map[string]any{"label": "New"},
	}
	body, _ = json.Marshal(goodEntry)
	req = httptest.NewRequest(http.MethodPost, "/content/themes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
}

func TestContentHTTP_RestAPISettingsEnforcement(t *testing.T) {
	authRepo := auth.NewMemoryAuthRepository()
	cfg := config.DefaultConfig()
	cfg.Auth.JWTSecret = "jwt-secret-rest-enforce"
	aSvc := auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, nil)
	_, err := aSvc.SeedDefaultRolesAndAdmin(t.Context(), "admin", "admin@cms.test", "Admin1234!")
	require.NoError(t, err)
	_, aTok, err := aSvc.Authenticate(t.Context(), "admin", "Admin1234!")
	require.NoError(t, err)

	contentRepo := NewMemoryContentRepository()
	svc := NewService(contentRepo, contentRepo, contentRepo, fields.NewRegistry(), NewEventBus(), nil)
	h := NewHandler(svc)
	liveCfg := RestAPIRuntimeConfig{
		DefaultLimit:       1,
		MaxLimit:           2,
		WithCount:          false,
		MaxPopulateDepth:   1,
		ExposeDraftPreview: false,
	}
	h.SetRestAPISettingsProvider(func(context.Context) RestAPIRuntimeConfig {
		return liveCfg
	})

	r := chi.NewRouter()
	r.Use(auth.AuthMiddleware(aSvc))
	h.RegisterRoutes(r)

	// Create content type + 2 published items + 1 draft item
	typePayload := CreateTypeRequest{
		UID:  "item",
		Name: "Items",
		Kind: KindCollection,
		Fields: []ContentField{
			{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText},
		},
	}
	ctBytes, _ := json.Marshal(typePayload)
	req := httptest.NewRequest(http.MethodPost, "/content-types", bytes.NewReader(ctBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+aTok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	for i := 1; i <= 3; i++ {
		eb, _ := json.Marshal(map[string]any{"title": fmt.Sprintf("Item %d", i)})
		creReq := httptest.NewRequest(http.MethodPost, "/content/item", bytes.NewReader(eb))
		creReq.Header.Set("Content-Type", "application/json")
		creReq.Header.Set("Authorization", "Bearer "+aTok)
		cw := httptest.NewRecorder()
		r.ServeHTTP(cw, creReq)
		require.Equal(t, http.StatusCreated, cw.Code)

		var created map[string]any
		require.NoError(t, json.Unmarshal(cw.Body.Bytes(), &created))
		id := created["id"].(string)
		if i < 3 {
			pubReq := httptest.NewRequest(http.MethodPost, "/content/item/"+id+"/publish", nil)
			pubReq.Header.Set("Authorization", "Bearer "+aTok)
			pw := httptest.NewRecorder()
			r.ServeHTTP(pw, pubReq)
			require.Equal(t, http.StatusOK, pw.Code)
		}
	}

	// 1. DefaultLimit=1, WithCount=false, ExposeDraftPreview=false
	req = httptest.NewRequest(http.MethodGet, "/content/item?preview=true", nil)
	req.Header.Set("Authorization", "Bearer "+aTok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	assert.Len(t, listResp["data"].([]any), 1)
	_, hasTotal := listResp["total"]
	assert.False(t, hasTotal)

	// 2. ?limit=99 capped to MaxLimit=2 (2 published items exist)
	req = httptest.NewRequest(http.MethodGet, "/content/item?limit=99&preview=true", nil)
	req.Header.Set("Authorization", "Bearer "+aTok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	assert.Len(t, listResp["data"].([]any), 2)

	// 3. Re-enabling WithCount & ExposeDraftPreview exposes draft + total immediately
	liveCfg.WithCount = true
	liveCfg.ExposeDraftPreview = true
	liveCfg.MaxLimit = 10
	req = httptest.NewRequest(http.MethodGet, "/content/item?limit=10&preview=true", nil)
	req.Header.Set("Authorization", "Bearer "+aTok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	assert.Len(t, listResp["data"].([]any), 3)
	assert.Equal(t, float64(3), listResp["total"])
}

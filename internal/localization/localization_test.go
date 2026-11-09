package localization

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_LocaleLifecycle(t *testing.T) {
	svc := NewService()

	// 1. Initial default locale
	assert.Equal(t, "en", svc.DefaultLocale())
	list := svc.List()
	require.Len(t, list, 1)
	assert.Equal(t, "en", list[0].Code)
	assert.True(t, list[0].IsDefault)

	// 2. Add valid locale (Spanish)
	es, err := svc.Add(Locale{
		Code:      "es",
		Name:      "Spanish",
		Direction: DirectionLTR,
	})
	require.NoError(t, err)
	assert.Equal(t, "es", es.Code)
	assert.False(t, es.IsDefault)

	// 3. Add valid RTL locale (Arabic)
	ar, err := svc.Add(Locale{
		Code:      "ar",
		Name:      "Arabic",
		Direction: DirectionRTL,
	})
	require.NoError(t, err)
	assert.Equal(t, DirectionRTL, ar.Direction)

	// 4. Reject invalid locale code
	_, err = svc.Add(Locale{Code: "invalid_123", Name: "Bad"})
	assert.Error(t, err)

	// 5. Switch default locale to Spanish
	err = svc.SetDefault("es")
	require.NoError(t, err)
	assert.Equal(t, "es", svc.DefaultLocale())

	// 6. Cannot delete default locale
	err = svc.Delete("es")
	assert.ErrorContains(t, err, "cannot delete the default locale")

	// 7. Delete non-default locale (Arabic)
	err = svc.Delete("ar")
	require.NoError(t, err)
	_, err = svc.Get("ar")
	assert.Error(t, err)
}

func TestService_ResolveLocalizedData(t *testing.T) {
	svc := NewService()

	data := map[string]any{
		"title": map[string]any{
			"en": "Hello World",
			"es": "Hola Mundo",
		},
		"description": map[string]any{
			"en": "English fallback description",
		},
		"views": 42,
	}

	// 1. Request Spanish: should resolve Spanish title, fallback for description, and keep views
	resolvedES := svc.ResolveLocalizedData(data, "es")
	assert.Equal(t, "Hola Mundo", resolvedES["title"])
	assert.Equal(t, "English fallback description", resolvedES["description"])
	assert.Equal(t, 42, resolvedES["views"])

	// 2. Request German: fallback to English for both
	resolvedDE := svc.ResolveLocalizedData(data, "de")
	assert.Equal(t, "Hello World", resolvedDE["title"])
	assert.Equal(t, "English fallback description", resolvedDE["description"])
	assert.Equal(t, 42, resolvedDE["views"])
}

func TestHTTPHandler_Endpoints(t *testing.T) {
	svc := NewService()
	handler := NewHTTPHandler(svc)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			adminUser := &auth.User{
				ID:       "admin-1",
				Username: "admin",
				Status:   auth.StatusActive,
				Roles: []auth.Role{
					{
						Slug: auth.RoleSuperAdmin,
					},
				},
			}
			ctx := context.WithValue(req.Context(), auth.UserContextKey, adminUser)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	handler.RegisterRoutes(r)

	// 1. GET /locales
	req := httptest.NewRequest("GET", "/locales", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 2. POST /locales
	payload := `{"code":"fr","name":"French","direction":"ltr"}`
	req = httptest.NewRequest("POST", "/locales", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// 3. GET /locales/fr
	req = httptest.NewRequest("GET", "/locales/fr", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 4. PUT /locales/fr/default
	req = httptest.NewRequest("PUT", "/locales/fr/default", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "fr", svc.DefaultLocale())

	// 5. DELETE /locales/en (now non-default)
	req = httptest.NewRequest("DELETE", "/locales/en", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

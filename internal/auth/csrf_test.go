package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCSRFMiddleware(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := CSRFMiddleware(dummyHandler)

	t.Run("GET request allows without CSRF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/content", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("POST with Bearer token skips CSRF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/content", nil)
		req.Header.Set("Authorization", "Bearer some-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("POST to /auth/login with existing cookie bypasses CSRF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "stale-session-token"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("POST to /auth/register with existing cookie bypasses CSRF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "stale-session-token"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("POST to /auth/invitations/accept with existing cookie bypasses CSRF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/invitations/accept", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "stale-session-token"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("Cookie-authenticated request with non-Bearer Authorization still requires CSRF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/content", nil)
		req.Header.Set("Authorization", "Basic dummy-value")
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "valid-session-token"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("Cookie-authenticated state-changing request blocked without valid CSRF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/content", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "valid-session-token"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("Cookie-authenticated state-changing request allowed with valid CSRF header", func(t *testing.T) {
		csrfBytes := make([]byte, 32)
		_, _ = rand.Read(csrfBytes)
		csrfToken := hex.EncodeToString(csrfBytes)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/content", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "valid-session-token"})
		req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfToken})
		req.Header.Set("X-CSRF-Token", csrfToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestSetAndClearAuthCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	SetAuthCookie(rec, "token123", time.Hour, false)

	cookies := rec.Result().Cookies()
	var sessionCookie, csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
		} else if c.Name == CSRFCookieName {
			csrfCookie = c
		}
	}

	assert.NotNil(t, sessionCookie)
	assert.Equal(t, "token123", sessionCookie.Value)
	assert.True(t, sessionCookie.HttpOnly)

	assert.NotNil(t, csrfCookie)
	assert.NotEmpty(t, csrfCookie.Value)
	assert.False(t, csrfCookie.HttpOnly)

	rec2 := httptest.NewRecorder()
	ClearAuthCookie(rec2)
	clearedCookies := rec2.Result().Cookies()
	for _, c := range clearedCookies {
		if c.Name == SessionCookieName || c.Name == CSRFCookieName {
			assert.Equal(t, -1, c.MaxAge)
		}
	}
}


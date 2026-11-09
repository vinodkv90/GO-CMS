package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPRateLimiter_AllowsUnderBurst(t *testing.T) {
	limiter := NewIPRateLimiter(10.0, 5)
	defer limiter.Close()

	ip := "192.168.1.50"
	// Should allow up to burst (5 requests)
	for i := 0; i < 5; i++ {
		assert.True(t, limiter.Allow(ip), "request %d should be allowed", i+1)
	}

	// 6th immediate request should be rejected
	assert.False(t, limiter.Allow(ip), "6th request exceeding burst should be rejected")

	// Different IP should still be allowed
	assert.True(t, limiter.Allow("10.0.0.1"), "different IP should have its own bucket")
}

func TestIPRateLimiter_MiddlewareBlocksWith429(t *testing.T) {
	limiter := NewIPRateLimiter(1.0, 2)
	defer limiter.Close()

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	// 1st request - ok
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "203.0.113.195:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// 2nd request - ok
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "203.0.113.195:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// 3rd request - rate limited
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "203.0.113.195:1234"
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusTooManyRequests, w3.Code)
	assert.Equal(t, "1", w3.Header().Get("Retry-After"))
	assert.Contains(t, w3.Body.String(), "too many requests")
}

func TestMaxBodySizeMiddleware_RejectsOversizedPayload(t *testing.T) {
	middleware := MaxBodySizeMiddleware(1024) // 1KB limit

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Under limit (500 bytes)
	underPayload := bytes.Repeat([]byte("a"), 500)
	reqUnder := httptest.NewRequest("POST", "/upload", bytes.NewReader(underPayload))
	reqUnder.Header.Set("Content-Length", "500")
	wUnder := httptest.NewRecorder()
	handler.ServeHTTP(wUnder, reqUnder)
	assert.Equal(t, http.StatusOK, wUnder.Code)

	// Over Content-Length limit (2000 bytes)
	overPayload := bytes.Repeat([]byte("a"), 2000)
	reqOver := httptest.NewRequest("POST", "/upload", bytes.NewReader(overPayload))
	reqOver.Header.Set("Content-Length", "2000")
	wOver := httptest.NewRecorder()
	handler.ServeHTTP(wOver, reqOver)
	assert.Equal(t, http.StatusRequestEntityTooLarge, wOver.Code)
	assert.Contains(t, wOver.Body.String(), "request body entity too large")
}


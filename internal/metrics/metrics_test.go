package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsRegistry_Initialization(t *testing.T) {
	reg := NewRegistry()
	require.NotNil(t, reg)

	assert.NotNil(t, reg.HTTPRequestsTotal)
	assert.NotNil(t, reg.HTTPRequestDuration)
	assert.NotNil(t, reg.InFlightRequests)
	assert.NotNil(t, reg.CacheHitsTotal)
	assert.NotNil(t, reg.CacheMissesTotal)
	assert.NotNil(t, reg.DBOpenConnections)
	assert.NotNil(t, reg.DBInUseConnections)
	assert.NotNil(t, reg.JobsQueueDepth)
}

func TestMetricsRegistry_HTTPMiddlewareAndHandler(t *testing.T) {
	reg := NewRegistry()

	r := chi.NewRouter()
	r.Use(reg.HTTPMiddleware)

	// Mount a sample route and metrics endpoint
	r.Get("/hello", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	})
	r.Get("/error", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	})
	r.Handle("/metrics", reg.Handler())

	// 1. Send request to /hello
	req1 := httptest.NewRequest("GET", "/hello", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// 2. Send request to /error
	req2 := httptest.NewRequest("GET", "/error", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusInternalServerError, w2.Code)

	// 3. Update cache & DB gauges
	reg.CacheHitsTotal.Inc()
	reg.CacheMissesTotal.Add(2)
	reg.DBOpenConnections.Set(10)
	reg.DBInUseConnections.Set(3)
	reg.JobsQueueDepth.Set(5)

	// 4. Scrape /metrics endpoint
	reqMetrics := httptest.NewRequest("GET", "/metrics", nil)
	wMetrics := httptest.NewRecorder()
	r.ServeHTTP(wMetrics, reqMetrics)

	assert.Equal(t, http.StatusOK, wMetrics.Code)
	bodyBytes, err := io.ReadAll(wMetrics.Body)
	require.NoError(t, err)

	body := string(bodyBytes)
	assert.Contains(t, body, "cms_http_requests_total")
	assert.Contains(t, body, `path="/hello"`)
	assert.Contains(t, body, `status="200"`)
	assert.Contains(t, body, `path="/error"`)
	assert.Contains(t, body, `status="500"`)
	assert.Contains(t, body, "cms_cache_hits_total 1")
	assert.Contains(t, body, "cms_cache_misses_total 2")
	assert.Contains(t, body, "cms_db_open_connections 10")
	assert.Contains(t, body, "cms_db_in_use_connections 3")
	assert.Contains(t, body, "cms_jobs_queue_depth 5")
}

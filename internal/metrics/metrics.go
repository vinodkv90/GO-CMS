package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry contains all Prometheus metric collectors for Go CMS.
type Registry struct {
	promRegistry *prometheus.Registry

	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	InFlightRequests    prometheus.Gauge

	CacheHitsTotal   prometheus.Counter
	CacheMissesTotal prometheus.Counter

	DBOpenConnections  prometheus.Gauge
	DBInUseConnections prometheus.Gauge

	JobsQueueDepth prometheus.Gauge
}

// NewRegistry creates and registers Prometheus metrics for Go CMS.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()

	// 1. HTTP metrics
	httpRequestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "cms",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests processed by Go CMS",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "cms",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Histogram of HTTP request latencies in seconds",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"method", "path", "status"},
	)

	inFlightRequests := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "cms",
			Subsystem: "http",
			Name:      "in_flight_requests",
			Help:      "Current number of in-flight HTTP requests",
		},
	)

	// 2. Cache metrics
	cacheHitsTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "cms",
			Subsystem: "cache",
			Name:      "hits_total",
			Help:      "Total number of cache lookup hits",
		},
	)

	cacheMissesTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "cms",
			Subsystem: "cache",
			Name:      "misses_total",
			Help:      "Total number of cache lookup misses",
		},
	)

	// 3. Database connection metrics
	dbOpenConns := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "cms",
			Subsystem: "db",
			Name:      "open_connections",
			Help:      "Number of established database connections",
		},
	)

	dbInUseConns := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "cms",
			Subsystem: "db",
			Name:      "in_use_connections",
			Help:      "Number of connections currently in use",
		},
	)

	// 4. Background jobs metrics
	jobsQueueDepth := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "cms",
			Subsystem: "jobs",
			Name:      "queue_depth",
			Help:      "Current number of pending tasks in the job queue",
		},
	)

	reg.MustRegister(
		httpRequestsTotal,
		httpRequestDuration,
		inFlightRequests,
		cacheHitsTotal,
		cacheMissesTotal,
		dbOpenConns,
		dbInUseConns,
		jobsQueueDepth,
	)

	return &Registry{
		promRegistry:        reg,
		HTTPRequestsTotal:   httpRequestsTotal,
		HTTPRequestDuration: httpRequestDuration,
		InFlightRequests:    inFlightRequests,
		CacheHitsTotal:      cacheHitsTotal,
		CacheMissesTotal:    cacheMissesTotal,
		DBOpenConnections:   dbOpenConns,
		DBInUseConnections:  dbInUseConns,
		JobsQueueDepth:      jobsQueueDepth,
	}
}

// Handler returns an HTTP handler for scraping Prometheus metrics.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.promRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusCapturingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// HTTPMiddleware instruments incoming HTTP requests with Prometheus metrics.
func (r *Registry) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Avoid instrumenting /metrics endpoint to prevent recursion
		if req.URL.Path == "/metrics" {
			next.ServeHTTP(w, req)
			return
		}

		r.InFlightRequests.Inc()
		defer r.InFlightRequests.Dec()

		start := time.Now()
		wrapped := &statusCapturingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, req)

		duration := time.Since(start).Seconds()

		// Normalize path pattern from Chi route context if available
		pattern := req.URL.Path
		if rctx := chi.RouteContext(req.Context()); rctx != nil {
			if routePattern := rctx.RoutePattern(); routePattern != "" {
				pattern = routePattern
			}
		}

		// Clean high-cardinality paths
		if strings.HasPrefix(pattern, "/uploads/") {
			pattern = "/uploads/*"
		} else if strings.HasPrefix(pattern, "/admin/") {
			pattern = "/admin/*"
		}

		statusStr := strconv.Itoa(wrapped.statusCode)
		r.HTTPRequestsTotal.WithLabelValues(req.Method, pattern, statusStr).Inc()
		r.HTTPRequestDuration.WithLabelValues(req.Method, pattern, statusStr).Observe(duration)
	})
}


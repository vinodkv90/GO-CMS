package jobs

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
)

// Handler handles HTTP requests for job inspection and management.
type HTTPHandler struct {
	pool *WorkerPool
}

// NewHTTPHandler creates a new HTTPHandler for background jobs.
func NewHTTPHandler(pool *WorkerPool) *HTTPHandler {
	return &HTTPHandler{pool: pool}
}

// RegisterRoutes registers job management endpoints onto the Chi router.
func (h *HTTPHandler) RegisterRoutes(r chi.Router) {
	r.Route("/jobs", func(jobs chi.Router) {
		// Require read permissions for viewing jobs
		jobs.With(auth.RequirePermission("read", "system:jobs")).Get("/", h.handleListJobs)
		jobs.With(auth.RequirePermission("read", "system:jobs")).Get("/stats", h.handleJobStats)
		jobs.With(auth.RequirePermission("read", "system:jobs")).Get("/{id}", h.handleGetJob)
		jobs.With(auth.RequirePermission("update", "system:jobs")).Post("/{id}/retry", h.handleRetryJob)
	})
}

func (h *HTTPHandler) handleListJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	offset := 0

	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if o, err := strconv.Atoi(q.Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	jobsList, total := h.pool.Queue().List(limit, offset)
	stats := h.pool.Queue().Stats()

	resp := map[string]any{
		"data": jobsList,
		"meta": map[string]any{
			"total":  total,
			"limit":  limit,
			"offset": offset,
			"stats":  stats,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *HTTPHandler) handleJobStats(w http.ResponseWriter, r *http.Request) {
	stats := h.pool.Queue().Stats()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": stats})
}

func (h *HTTPHandler) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, found := h.pool.Queue().Get(id)
	if !found {
		http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": job})
}

func (h *HTTPHandler) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, found := h.pool.Queue().Get(id)
	if !found {
		http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		return
	}

	// Reset job status to pending with immediate run time
	job.Status = StatusPending
	job.Attempts = 0
	job.Error = ""
	job.RunAt = time.Now().UTC()
	h.pool.Queue().Update(job)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": "job queued for immediate retry",
		"data":    job,
	})
}

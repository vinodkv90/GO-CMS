package search

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// HTTPHandler exposes full-text search over HTTP.
type HTTPHandler struct {
	engine *Engine
}

// NewHTTPHandler creates a new search HTTPHandler.
func NewHTTPHandler(engine *Engine) *HTTPHandler {
	return &HTTPHandler{engine: engine}
}

// RegisterRoutes mounts search endpoints on the Chi router.
func (h *HTTPHandler) RegisterRoutes(r chi.Router) {
	r.Get("/search", h.handleSearch)
}

func (h *HTTPHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	typeFilter := r.URL.Query().Get("type")
	limit := 20

	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}

	results := h.engine.Search(q, typeFilter, limit)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": results,
		"meta": map[string]any{
			"query": q,
			"type":  typeFilter,
			"total": len(results),
			"limit": limit,
		},
	})
}


package audit

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
)

// HTTPHandler exposes audit log query endpoints over HTTP.
type HTTPHandler struct {
	svc *Service
}

// NewHTTPHandler creates a new audit HTTPHandler.
func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// RegisterRoutes registers audit endpoints on the Chi router.
func (h *HTTPHandler) RegisterRoutes(r chi.Router) {
	r.Route("/audit-logs", func(logs chi.Router) {
		logs.With(auth.RequirePermission("read", "system:audit_logs")).Get("/", h.handleListAuditLogs)
	})
}

func (h *HTTPHandler) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	offset := 0

	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if o, err := strconv.Atoi(q.Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	filter := Filter{
		ActorID:  q.Get("actor_id"),
		Action:   q.Get("action"),
		Resource: q.Get("resource"),
		Limit:    limit,
		Offset:   offset,
	}

	logs, total, err := h.svc.List(r.Context(), filter)
	if err != nil {
		http.Error(w, `{"error":"failed to retrieve audit logs"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": logs,
		"meta": map[string]any{
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

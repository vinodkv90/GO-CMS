package webhooks

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
)

// Handler exposes RBAC-protected REST endpoints for outgoing webhooks.
type Handler struct {
	svc *Service
}

// NewHandler creates a webhooks HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts webhook management endpoints under /api/v1/webhooks.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/webhooks", func(wr chi.Router) {
		wr.Use(auth.RequireAuth)

		wr.With(auth.RequirePermission("read", "webhooks")).Get("/", h.List)
		wr.With(auth.RequirePermission("read", "webhooks")).Get("/events", h.ListEvents)
		wr.With(auth.RequirePermission("create", "webhooks")).Post("/", h.Create)
		wr.With(auth.RequirePermission("read", "webhooks")).Get("/{id}", h.Get)
		wr.With(auth.RequirePermission("update", "webhooks")).Put("/{id}", h.Update)
		wr.With(auth.RequirePermission("delete", "webhooks")).Delete("/{id}", h.Delete)
		wr.With(auth.RequirePermission("update", "webhooks")).Post("/{id}/trigger", h.TriggerTest)
		wr.With(auth.RequirePermission("read", "webhooks")).Get("/{id}/deliveries", h.ListDeliveries)
	})
}

func (h *Handler) ListEvents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": EventCatalog()})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*Webhook{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	wh, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "Webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": wh})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	var actorID string
	if u := auth.GetUserFromContext(r.Context()); u != nil {
		actorID = u.ID
	}
	created, err := h.svc.Create(r.Context(), req, actorID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req UpdateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	var actorID string
	if u := auth.GetUserFromContext(r.Context()); u != nil {
		actorID = u.ID
	}
	updated, err := h.svc.Update(r.Context(), id, req, actorID)
	if err != nil {
		if errors.Is(err, ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "Webhook not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var actorID string
	if u := auth.GetUserFromContext(r.Context()); u != nil {
		actorID = u.ID
	}
	if err := h.svc.Delete(r.Context(), id, actorID); err != nil {
		if errors.Is(err, ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "Webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "Webhook deleted"})
}

func (h *Handler) TriggerTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var actorID string
	if u := auth.GetUserFromContext(r.Context()); u != nil {
		actorID = u.ID
	}
	delivery, err := h.svc.TriggerTest(r.Context(), id, actorID)
	if err != nil {
		if errors.Is(err, ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "Webhook not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": delivery})
}

func (h *Handler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 20
	}
	items, total, err := h.svc.ListDeliveries(r.Context(), id, limit, offset)
	if err != nil {
		if errors.Is(err, ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "Webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}


package tenancy

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
)

// HTTPHandler exposes tenant management endpoints over HTTP.
type HTTPHandler struct {
	svc *Service
}

// NewHTTPHandler creates a new tenant HTTPHandler.
func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// RegisterRoutes registers tenant management endpoints on the Chi router.
func (h *HTTPHandler) RegisterRoutes(r chi.Router) {
	r.Route("/tenants", func(tenants chi.Router) {
		tenants.With(auth.RequirePermission("read", "system:tenants")).Get("/", h.handleListTenants)
		tenants.With(auth.RequirePermission("read", "system:tenants")).Get("/{id}", h.handleGetTenant)
		tenants.With(auth.RequirePermission("create", "system:tenants")).Post("/", h.handleCreateTenant)
		tenants.With(auth.RequirePermission("update", "system:tenants")).Patch("/{id}", h.handleUpdateTenant)
		tenants.With(auth.RequirePermission("delete", "system:tenants")).Delete("/{id}", h.handleDeleteTenant)
	})
}

func (h *HTTPHandler) handleListTenants(w http.ResponseWriter, r *http.Request) {
	list := h.svc.List()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": list,
		"meta": map[string]any{"total": len(list)},
	})
}

func (h *HTTPHandler) handleGetTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.svc.Get(id)
	if err != nil {
		t, err = h.svc.GetBySlug(id)
	}
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": t})
}

func (h *HTTPHandler) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	var body Tenant
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}

	created, err := h.svc.Create(r.Context(), body)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": created})
}

func (h *HTTPHandler) handleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body Tenant
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}

	updated, err := h.svc.Update(r.Context(), id, body)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": updated})
}

func (h *HTTPHandler) handleDeleteTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"message": "tenant deleted successfully"})
}

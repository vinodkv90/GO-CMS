package localization

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
)

// HTTPHandler manages HTTP requests for locales.
type HTTPHandler struct {
	svc *Service
}

// NewHTTPHandler creates a new HTTPHandler.
func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// RegisterRoutes registers localization endpoints on the Chi router.
func (h *HTTPHandler) RegisterRoutes(r chi.Router) {
	r.Route("/locales", func(locales chi.Router) {
		locales.Get("/", h.handleListLocales)
		locales.Get("/{code}", h.handleGetLocale)

		// Administrative mutations require write access
		locales.With(auth.RequirePermission("create", "system:locales")).Post("/", h.handleCreateLocale)
		locales.With(auth.RequirePermission("update", "system:locales")).Put("/{code}/default", h.handleSetDefaultLocale)
		locales.With(auth.RequirePermission("delete", "system:locales")).Delete("/{code}", h.handleDeleteLocale)
	})
}

func (h *HTTPHandler) handleListLocales(w http.ResponseWriter, r *http.Request) {
	list := h.svc.List()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": list})
}

func (h *HTTPHandler) handleGetLocale(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	loc, err := h.svc.Get(code)
	if err != nil {
		http.Error(w, `{"error":"locale not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": loc})
}

func (h *HTTPHandler) handleCreateLocale(w http.ResponseWriter, r *http.Request) {
	var body Locale
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}

	created, err := h.svc.Add(body)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": created})
}

func (h *HTTPHandler) handleSetDefaultLocale(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if err := h.svc.SetDefault(code); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"message": "default locale updated successfully"})
}

func (h *HTTPHandler) handleDeleteLocale(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if err := h.svc.Delete(code); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"message": "locale deleted successfully"})
}

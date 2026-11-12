package localization

import (
	"encoding/json"
	"errors"
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
		locales.With(auth.RequirePermission("update", "system:locales")).Put("/{code}", h.handleUpdateLocale)
		locales.With(auth.RequirePermission("update", "system:locales")).Put("/{code}/default", h.handleSetDefaultLocale)
		locales.With(auth.RequirePermission("delete", "system:locales")).Delete("/{code}", h.handleDeleteLocale)
	})
}

func (h *HTTPHandler) handleListLocales(w http.ResponseWriter, _ *http.Request) {
	list := h.svc.List()
	writeLocJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *HTTPHandler) handleGetLocale(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	loc, err := h.svc.Get(code)
	if err != nil {
		writeLocJSON(w, http.StatusNotFound, map[string]string{"error": "locale not found"})
		return
	}

	writeLocJSON(w, http.StatusOK, map[string]any{"data": loc})
}

func (h *HTTPHandler) handleCreateLocale(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code         string    `json:"code"`
		Name         string    `json:"name"`
		NativeName   string    `json:"native_name"`
		IsDefault    bool      `json:"is_default"`
		IsEnabled    *bool     `json:"is_enabled"`
		FallbackCode string    `json:"fallback_code"`
		Direction    Direction `json:"direction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeLocJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	isEnabled := true
	if body.IsEnabled != nil {
		isEnabled = *body.IsEnabled
	}

	created, err := h.svc.Add(Locale{
		Code:         body.Code,
		Name:         body.Name,
		NativeName:   body.NativeName,
		IsDefault:    body.IsDefault,
		IsEnabled:    isEnabled,
		FallbackCode: body.FallbackCode,
		Direction:    body.Direction,
	})
	if err != nil {
		writeLocJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeLocJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (h *HTTPHandler) handleUpdateLocale(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	var body LocaleUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeLocJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	updated, err := h.svc.Update(code, body)
	if err != nil {
		writeLocJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeLocJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (h *HTTPHandler) handleSetDefaultLocale(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if err := h.svc.SetDefault(code); err != nil {
		writeLocJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeLocJSON(w, http.StatusOK, map[string]any{"message": "default locale updated successfully"})
}

func (h *HTTPHandler) handleDeleteLocale(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if err := h.svc.Delete(code); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrLocaleInUse) {
			status = http.StatusConflict
		}
		writeLocJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	writeLocJSON(w, http.StatusOK, map[string]any{"message": "locale deleted successfully"})
}

func writeLocJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

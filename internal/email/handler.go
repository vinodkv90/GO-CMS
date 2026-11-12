package email

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/settings"
)

// HTTPHandler exposes RBAC-protected email configuration and template management APIs.
type HTTPHandler struct {
	svc *Service
}

// NewHTTPHandler constructs an email HTTPHandler.
func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// RegisterProtectedRoutes mounts email configuration and template endpoints under /api/v1/settings.
func (h *HTTPHandler) RegisterProtectedRoutes(r chi.Router) {
	r.Group(func(protected chi.Router) {
		protected.Use(auth.RequireAuth)

		protected.With(auth.RequirePermission("read", "settings")).Get("/settings/email", h.GetSettings)
		protected.With(auth.RequirePermission("update", "settings")).Put("/settings/email", h.UpdateSettings)
		protected.With(auth.RequirePermission("update", "settings")).Post("/settings/email/test-connection", h.TestConnection)
		protected.With(auth.RequirePermission("update", "settings")).Post("/settings/email/test-send", h.TestSend)

		protected.With(auth.RequirePermission("read", "settings")).Get("/settings/email-templates", h.ListTemplates)
		protected.With(auth.RequirePermission("read", "settings")).Get("/settings/email-templates/{key}", h.GetTemplate)
		protected.With(auth.RequirePermission("update", "settings")).Put("/settings/email-templates/{key}", h.UpdateTemplate)
		protected.With(auth.RequirePermission("read", "settings")).Post("/settings/email-templates/{key}/preview", h.PreviewTemplate)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func auditMetaFromRequest(r *http.Request) settings.AuditMeta {
	meta := settings.AuditMeta{
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	if u := auth.GetUserFromContext(r.Context()); u != nil {
		meta.ActorID = u.ID
		meta.ActorType = "admin"
	}
	return meta
}

func (h *HTTPHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.GetSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load email settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": cfg})
}

func (h *HTTPHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var input EmailSettings
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	updated, err := h.svc.UpdateSettings(r.Context(), input, auditMetaFromRequest(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (h *HTTPHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	var override *EmailSettings
	if r.Body != nil {
		var payload EmailSettings
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil && payload.Provider != "" {
			override = &payload
		}
	}
	res, err := h.svc.TestConnection(r.Context(), override)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (h *HTTPHandler) TestSend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To       string         `json:"to"`
		Override *EmailSettings `json:"override,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	res, err := h.svc.SendTestEmail(r.Context(), body.To, body.Override)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (h *HTTPHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListTemplates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load email templates")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *HTTPHandler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	tmpl, err := h.svc.GetTemplate(r.Context(), key)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load email template")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tmpl})
}

func (h *HTTPHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var input UpdateTemplateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	updated, err := h.svc.UpdateTemplate(r.Context(), key, input, auditMetaFromRequest(r))
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (h *HTTPHandler) PreviewTemplate(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var body struct {
		Override  *UpdateTemplateInput `json:"override,omitempty"`
		Variables map[string]string    `json:"variables,omitempty"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	res, err := h.svc.PreviewTemplate(r.Context(), key, body.Override, body.Variables)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

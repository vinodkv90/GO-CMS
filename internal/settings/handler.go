package settings

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
)

// HTTPHandler serves settings, overview, system info, and user preferences endpoints.
type HTTPHandler struct {
	svc *Service
}

// NewHTTPHandler constructs a settings HTTPHandler.
func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// RegisterPublicRoutes mounts unauthenticated public settings metadata.
func (h *HTTPHandler) RegisterPublicRoutes(r chi.Router) {
	r.Get("/settings/public", h.GetPublicSettings)
}

// RegisterProtectedRoutes mounts RBAC-protected settings and authenticated user preference endpoints.
func (h *HTTPHandler) RegisterProtectedRoutes(r chi.Router) {
	r.Group(func(protected chi.Router) {
		protected.Use(auth.RequireAuth)

		protected.Route("/settings", func(sr chi.Router) {
			sr.With(auth.RequirePermission("read", "settings")).Get("/overview", h.GetOverview)
			sr.With(auth.RequirePermission("read", "settings")).Get("/general", h.GetGeneral)
			sr.With(auth.RequirePermission("update", "settings")).Put("/general", h.UpdateGeneral)
			sr.With(auth.RequirePermission("read", "settings")).Get("/rest-api", h.GetRestAPI)
			sr.With(auth.RequirePermission("update", "settings")).Put("/rest-api", h.UpdateRestAPI)
			sr.With(auth.RequirePermission("read", "settings")).Get("/cors", h.GetCORS)
			sr.With(auth.RequirePermission("update", "settings")).Put("/cors", h.UpdateCORS)
			sr.With(auth.RequirePermission("read", "settings")).Get("/rate-limit", h.GetRateLimit)
			sr.With(auth.RequirePermission("update", "settings")).Put("/rate-limit", h.UpdateRateLimit)
			sr.With(auth.RequirePermission("read", "settings")).Get("/media", h.GetMedia)
			sr.With(auth.RequirePermission("update", "settings")).Put("/media", h.UpdateMedia)
			sr.With(auth.RequirePermission("read", "settings")).Get("/storage", h.GetStorage)
			sr.With(auth.RequirePermission("update", "settings")).Put("/storage", h.UpdateStorage)
			sr.With(auth.RequirePermission("update", "settings")).Post("/storage/test", h.TestStorage)
			sr.With(auth.RequirePermission("read", "settings")).Get("/security", h.GetSecurity)
			sr.With(auth.RequirePermission("update", "settings")).Put("/security", h.UpdateSecurity)
			sr.With(auth.RequirePermission("read", "settings")).Get("/password-policy", h.GetPasswordPolicy)
			sr.With(auth.RequirePermission("update", "settings")).Put("/password-policy", h.UpdatePasswordPolicy)
			sr.With(auth.RequirePermission("read", "settings")).Get("/sso", h.GetSSO)
			sr.With(auth.RequirePermission("update", "settings")).Put("/sso", h.UpdateSSO)
			sr.With(auth.RequirePermission("read", "system:info")).Get("/system", h.GetSystemInfo)
		})

		protected.Route("/preferences", func(pr chi.Router) {
			pr.Get("/", h.GetPreferences)
			pr.Put("/", h.UpdatePreferences)
		})
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

func auditMetaFromRequest(r *http.Request) AuditMeta {
	meta := AuditMeta{
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	if u := auth.GetUserFromContext(r.Context()); u != nil {
		meta.ActorID = u.ID
		meta.ActorType = "admin"
	}
	if tok := auth.GetAPITokenFromContext(r.Context()); tok != nil {
		meta.ActorType = "api_token"
	}
	return meta
}

func (h *HTTPHandler) GetPublicSettings(w http.ResponseWriter, r *http.Request) {
	pub := h.svc.GetPublicGeneral(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"data": pub})
}

func (h *HTTPHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.svc.GetOverview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings overview")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": overview})
}

func (h *HTTPHandler) GetGeneral(w http.ResponseWriter, r *http.Request) {
	gen, err := h.svc.GetGeneral(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load general settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": gen})
}

func (h *HTTPHandler) UpdateGeneral(w http.ResponseWriter, r *http.Request) {
	var input GeneralSettings
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	updated, err := h.svc.UpdateGeneral(r.Context(), input, auditMetaFromRequest(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (h *HTTPHandler) GetSystemInfo(w http.ResponseWriter, r *http.Request) {
	info := h.svc.GetSystemInfo(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"data": info})
}

func (h *HTTPHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	prefs, err := h.svc.GetPreferences(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load user preferences")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": prefs})
}

func (h *HTTPHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if auth.GetAPITokenFromContext(r.Context()) != nil {
		writeError(w, http.StatusForbidden, "api tokens cannot modify user preferences")
		return
	}

	var input UserPreferences
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	updated, err := h.svc.UpdatePreferences(r.Context(), user.ID, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func handleGetTyped[T any](w http.ResponseWriter, r *http.Request, getter func(context.Context) (*T, error), label string) {
	val, err := getter(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load "+label+" settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": val})
}

func handleUpdateTyped[T any](w http.ResponseWriter, r *http.Request, updater func(context.Context, T, AuditMeta) (*T, error)) {
	var input T
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	updated, err := updater(r.Context(), input, auditMetaFromRequest(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (h *HTTPHandler) GetRestAPI(w http.ResponseWriter, r *http.Request) {
	handleGetTyped(w, r, h.svc.GetRestAPISettings, "rest-api")
}

func (h *HTTPHandler) UpdateRestAPI(w http.ResponseWriter, r *http.Request) {
	handleUpdateTyped(w, r, h.svc.UpdateRestAPISettings)
}

func (h *HTTPHandler) GetCORS(w http.ResponseWriter, r *http.Request) {
	handleGetTyped(w, r, h.svc.GetCORSSettings, "cors")
}

func (h *HTTPHandler) UpdateCORS(w http.ResponseWriter, r *http.Request) {
	handleUpdateTyped(w, r, h.svc.UpdateCORSSettings)
}

func (h *HTTPHandler) GetRateLimit(w http.ResponseWriter, r *http.Request) {
	handleGetTyped(w, r, h.svc.GetRateLimitSettings, "rate-limit")
}

func (h *HTTPHandler) UpdateRateLimit(w http.ResponseWriter, r *http.Request) {
	handleUpdateTyped(w, r, h.svc.UpdateRateLimitSettings)
}

func (h *HTTPHandler) GetMedia(w http.ResponseWriter, r *http.Request) {
	handleGetTyped(w, r, h.svc.GetMediaSettings, "media")
}

func (h *HTTPHandler) UpdateMedia(w http.ResponseWriter, r *http.Request) {
	handleUpdateTyped(w, r, h.svc.UpdateMediaSettings)
}

func (h *HTTPHandler) GetStorage(w http.ResponseWriter, r *http.Request) {
	handleGetTyped(w, r, h.svc.GetStorageSettings, "storage")
}

func (h *HTTPHandler) UpdateStorage(w http.ResponseWriter, r *http.Request) {
	handleUpdateTyped(w, r, h.svc.UpdateStorageSettings)
}

func (h *HTTPHandler) TestStorage(w http.ResponseWriter, r *http.Request) {
	var override *StorageSettings
	if r.Body != nil {
		var payload StorageSettings
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			override = &payload
		}
	}
	res, err := h.svc.TestStorageConnection(r.Context(), override)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (h *HTTPHandler) GetSecurity(w http.ResponseWriter, r *http.Request) {
	handleGetTyped(w, r, h.svc.GetSecuritySettings, "security")
}

func (h *HTTPHandler) UpdateSecurity(w http.ResponseWriter, r *http.Request) {
	handleUpdateTyped(w, r, h.svc.UpdateSecuritySettings)
}

func (h *HTTPHandler) GetPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	handleGetTyped(w, r, h.svc.GetPasswordPolicySettings, "password-policy")
}

func (h *HTTPHandler) UpdatePasswordPolicy(w http.ResponseWriter, r *http.Request) {
	handleUpdateTyped(w, r, h.svc.UpdatePasswordPolicySettings)
}

func (h *HTTPHandler) GetSSO(w http.ResponseWriter, r *http.Request) {
	handleGetTyped(w, r, h.svc.GetSSOSettings, "sso")
}

func (h *HTTPHandler) UpdateSSO(w http.ResponseWriter, r *http.Request) {
	handleUpdateTyped(w, r, h.svc.UpdateSSOSettings)
}

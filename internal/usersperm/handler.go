package usersperm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/settings"
)

type appContextKey string

const (
	appUserContextKey appContextKey = "app_user"
)

// GetAppUserFromContext retrieves the authenticated isolated application user from context.
func GetAppUserFromContext(ctx context.Context) *AppUser {
	if ctx == nil {
		return nil
	}
	if u, ok := ctx.Value(appUserContextKey).(*AppUser); ok {
		return u
	}
	return nil
}

// Handler exposes strictly segregated public /app-auth/*, admin SSO /auth/sso/*, and RBAC /admin/users-permissions/* routes.
type Handler struct {
	svc *Service
}

// NewHandler creates a usersperm HTTP Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
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
		meta.ActorType = "user"
		if auth.GetAPITokenFromContext(r.Context()) != nil {
			meta.ActorType = "api_token"
		}
	}
	return meta
}

func mapServiceErr(err error) int {
	switch {
	case errors.Is(err, ErrAppUnauthorized), errors.Is(err, ErrInvalidAppCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, ErrAppForbidden), errors.Is(err, ErrAppAccountBlocked), errors.Is(err, ErrAppAccountUnconfirmed), errors.Is(err, ErrRegistrationDisabled), errors.Is(err, ErrProviderDisabled):
		return http.StatusForbidden
	case errors.Is(err, ErrAppUserNotFound), errors.Is(err, ErrAppRoleNotFound), errors.Is(err, ErrProviderNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrAppUserExists), errors.Is(err, ErrAppRoleExists), errors.Is(err, ErrAppRoleInUse), errors.Is(err, ErrSystemAppRole):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}
	return scheme + "://" + host
}

// requireAppAuth strictly authenticates end-user JWTs minted for "go-cms-app-users" and rejects Admin JWTs.
func (h *Handler) requireAppAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			writeError(w, http.StatusUnauthorized, "app user authentication required")
			return
		}
		rawTok := strings.TrimSpace(header[7:])
		claims, err := ValidateAppAccessToken(rawTok, h.svc.jwtSecret())
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired application token")
			return
		}
		user, err := h.svc.GetMe(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, mapServiceErr(err), err.Error())
			return
		}
		ctx := context.WithValue(r.Context(), appUserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RegisterRoutes mounts all App User auth, Admin SSO, and Admin Users & Permissions management routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// 1. Public Isolated Application Auth (/app-auth/*)
	r.Route("/app-auth", func(app chi.Router) {
		app.Post("/register", h.AppRegister)
		app.Post("/login", h.AppLogin)
		app.Post("/confirm-email", h.AppConfirmEmail)
		app.Post("/send-confirmation", h.AppSendConfirmation)
		app.Post("/forgot-password", h.AppForgotPassword)
		app.Post("/reset-password", h.AppResetPassword)
		app.Get("/providers", h.ListPublicAppProviders)
		app.Get("/providers/{name}/authorize", h.AppOAuthAuthorize)
		app.Get("/providers/{name}/callback", h.AppOAuthCallback)

		app.With(h.requireAppAuth).Get("/me", h.AppMe)
	})

	// 2. Public Enterprise Admin SSO entrypoints (/auth/sso/*)
	r.Route("/auth/sso", func(sso chi.Router) {
		sso.Get("/providers", h.ListPublicAdminSSOProviders)
		sso.Get("/{name}/authorize", h.AdminSSOAuthorize)
		sso.Get("/{name}/callback", h.AdminSSOCallback)
	})

	// 3. Admin Panel RBAC-protected Users & Permissions management (/admin/users-permissions/*)
	r.Route("/admin/users-permissions", func(adm chi.Router) {
		adm.Use(auth.RequireAuth)

		// Roles
		adm.With(auth.RequirePermission("read", "users_permissions")).Get("/roles", h.ListRoles)
		adm.With(auth.RequirePermission("read", "users_permissions")).Get("/roles/{id}", h.GetRole)
		adm.With(auth.RequirePermission("update", "users_permissions")).Post("/roles", h.CreateRole)
		adm.With(auth.RequirePermission("update", "users_permissions")).Put("/roles/{id}", h.UpdateRole)
		adm.With(auth.RequirePermission("update", "users_permissions")).Delete("/roles/{id}", h.DeleteRole)

		// App Users
		adm.With(auth.RequirePermission("read", "users_permissions")).Get("/users", h.ListUsers)
		adm.With(auth.RequirePermission("read", "users_permissions")).Get("/users/{id}", h.GetUser)
		adm.With(auth.RequirePermission("update", "users_permissions")).Post("/users", h.CreateUser)
		adm.With(auth.RequirePermission("update", "users_permissions")).Put("/users/{id}", h.UpdateUser)
		adm.With(auth.RequirePermission("update", "users_permissions")).Delete("/users/{id}", h.DeleteUser)

		// OAuth / SSO Providers (both "app" and "admin" scope configurable by authorized admin)
		adm.With(auth.RequirePermission("read", "users_permissions")).Get("/providers", h.ListProviders)
		adm.With(auth.RequirePermission("update", "users_permissions")).Put("/providers/{name}", h.UpsertProvider)
		adm.With(auth.RequirePermission("update", "users_permissions")).Post("/providers/discover", h.DiscoverOIDC)
		adm.With(auth.RequirePermission("update", "users_permissions")).Delete("/providers/{name}", h.DeleteProvider)

		// App Auth Advanced Settings
		adm.With(auth.RequirePermission("read", "users_permissions")).Get("/settings", h.GetSettings)
		adm.With(auth.RequirePermission("update", "users_permissions")).Put("/settings", h.UpdateSettings)
	})
}

// --- Public App Auth Handlers ---

func (h *Handler) AppRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterAppUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.svc.Register(r.Context(), req)
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (h *Handler) AppLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginAppUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.svc.Login(r.Context(), req)
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) AppConfirmEmail(w http.ResponseWriter, r *http.Request) {
	var req ConfirmEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.svc.ConfirmEmail(r.Context(), req.Token)
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) AppSendConfirmation(w http.ResponseWriter, r *http.Request) {
	var req SendConfirmationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.SendConfirmation(r.Context(), req.Email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "If an unconfirmed account exists for that email, a confirmation link has been sent."})
}

func (h *Handler) AppForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), req.Email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "If an account exists for that email, a password reset link has been sent."})
}

func (h *Handler) AppResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req); err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Password reset successfully"})
}

func (h *Handler) AppMe(w http.ResponseWriter, r *http.Request) {
	u := GetAppUserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": u})
}

func (h *Handler) ListPublicAppProviders(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListEnabledPublicProviders(r.Context(), "app")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) AppOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	h.handleOAuthAuthorize(w, r, "app")
}

func (h *Handler) AppOAuthCallback(w http.ResponseWriter, r *http.Request) {
	h.handleOAuthCallback(w, r, "app")
}

// --- Public Enterprise Admin SSO Handlers ---

func (h *Handler) ListPublicAdminSSOProviders(w http.ResponseWriter, r *http.Request) {
	enabled := false
	if h.svc.settingsSvc != nil {
		if ssoCfg, err := h.svc.settingsSvc.GetSSOSettings(r.Context()); err == nil && ssoCfg != nil {
			enabled = ssoCfg.Enabled
		}
	}
	if !enabled {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"enabled":   false,
			"providers": []PublicProviderInfo{},
		}})
		return
	}
	items, err := h.svc.ListEnabledPublicProviders(r.Context(), "admin")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"enabled":   true,
		"providers": items,
	}})
}

func (h *Handler) AdminSSOAuthorize(w http.ResponseWriter, r *http.Request) {
	h.handleOAuthAuthorize(w, r, "admin")
}

func (h *Handler) AdminSSOCallback(w http.ResponseWriter, r *http.Request) {
	h.handleOAuthCallback(w, r, "admin")
}

func (h *Handler) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request, scope string) {
	name := chi.URLParam(r, "name")
	redirectAfter := r.URL.Query().Get("redirect_after")
	res, err := h.svc.StartOAuthAuthorize(r.Context(), name, scope, redirectAfter, requestBaseURL(r))
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	if r.URL.Query().Get("format") == "json" {
		writeJSON(w, http.StatusOK, map[string]any{"data": res})
		return
	}
	http.Redirect(w, r, res.AuthorizationURL, http.StatusFound)
}

func (h *Handler) handleOAuthCallback(w http.ResponseWriter, r *http.Request, scope string) {
	name := chi.URLParam(r, "name")
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	res, err := h.svc.HandleOAuthCallback(r.Context(), name, scope, code, state, requestBaseURL(r), r.RemoteAddr)
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}

	if scope == "admin" && res.AdminAuth != nil && res.AdminAuth.Token != "" {
		isProd := h.svc.cfg != nil && h.svc.cfg.Server.Env == "production"
		ttl := 24 * time.Hour
		if h.svc.settingsSvc != nil {
			if secCfg, err := h.svc.settingsSvc.GetSecuritySettings(r.Context()); err == nil && secCfg != nil && secCfg.SessionTimeoutMinutes > 0 {
				ttl = time.Duration(secCfg.SessionTimeoutMinutes) * time.Minute
			}
		}
		auth.SetAuthCookie(w, res.AdminAuth.Token, ttl, isProd)
		if res.RedirectAfter != "" && isRelativePathRedirect(res.RedirectAfter) {
			http.Redirect(w, r, res.RedirectAfter, http.StatusFound)
			return
		}
		writeJSON(w, http.StatusOK, res.AdminAuth)
		return
	}

	if res.RedirectAfter != "" && isRelativePathRedirect(res.RedirectAfter) && res.AppAuth != nil {
		sep := "?"
		if strings.Contains(res.RedirectAfter, "?") {
			sep = "&"
		}
		http.Redirect(w, r, res.RedirectAfter+sep+"token="+url.QueryEscape(res.AppAuth.Token), http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, res.AppAuth)
}

func isRelativePathRedirect(target string) bool {
	trimmed := strings.TrimSpace(target)
	return strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "//")
}

// --- Admin Management Handlers ---

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.ListRoles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": roles})
}

func (h *Handler) GetRole(w http.ResponseWriter, r *http.Request) {
	role, err := h.svc.GetRole(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": role})
}

func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req CreateAppRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	role, err := h.svc.CreateRole(r.Context(), req, auditMetaFromRequest(r))
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": role})
}

func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	var req UpdateAppRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	role, err := h.svc.UpdateRole(r.Context(), chi.URLParam(r, "id"), req, auditMetaFromRequest(r))
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": role})
}

func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteRole(r.Context(), chi.URLParam(r, "id"), auditMetaFromRequest(r)); err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "role deleted"})
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 20
	}
	users, total, err := h.svc.ListUsers(r.Context(), AppUserListFilter{
		Search: q.Get("search"),
		RoleID: q.Get("role_id"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": users,
		"meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.GetUser(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": u})
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateAppUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := h.svc.CreateUser(r.Context(), req, auditMetaFromRequest(r))
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": u})
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	var req UpdateAppUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := h.svc.UpdateUser(r.Context(), chi.URLParam(r, "id"), req, auditMetaFromRequest(r))
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": u})
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteUser(r.Context(), chi.URLParam(r, "id"), auditMetaFromRequest(r)); err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "user deleted"})
}

func (h *Handler) ListProviders(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	items, err := h.svc.ListProviders(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) UpsertProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req UpsertOAuthProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.svc.UpsertProvider(r.Context(), name, req, auditMetaFromRequest(r))
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (h *Handler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteProvider(r.Context(), chi.URLParam(r, "name"), auditMetaFromRequest(r)); err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "provider deleted"})
}

func (h *Handler) DiscoverOIDC(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IssuerURL string `json:"issuer_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.svc.DiscoverOIDC(r.Context(), body.IssuerURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.GetSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req AppAuthSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.svc.UpdateSettings(r.Context(), req, auditMetaFromRequest(r))
	if err != nil {
		writeError(w, mapServiceErr(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

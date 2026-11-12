package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Handler exposes HTTP handlers for authentication and identity management.
type Handler struct {
	svc *Service
}

// NewHandler initializes a new Auth HTTP Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func auditMetaFromRequest(r *http.Request) AuditMeta {
	meta := AuditMeta{
		IP:        r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	if u := GetUserFromContext(r.Context()); u != nil {
		meta.ActorID = u.ID
		meta.ActorEmail = u.Email
	}
	return meta
}

func mapAuthServiceErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrIPNotAllowed), errors.Is(err, ErrSSOOnlyAccount), errors.Is(err, ErrPasswordExpired):
		return http.StatusForbidden
	case errors.Is(err, ErrUnauthorized), errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrTokenExpired), errors.Is(err, ErrInvalidMFACode):
		return http.StatusUnauthorized
	case errors.Is(err, ErrAccountLocked):
		return http.StatusTooManyRequests
	case errors.Is(err, ErrUserNotFound), errors.Is(err, ErrRoleNotFound), errors.Is(err, ErrPermissionNotFound), errors.Is(err, ErrInvitationNotFound), errors.Is(err, ErrTokenNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrUserAlreadyExists), errors.Is(err, ErrRoleAlreadyExists), errors.Is(err, ErrLastSuperAdmin), errors.Is(err, ErrRoleInUse), errors.Is(err, ErrSystemRoleProtected), errors.Is(err, ErrInvitationRevoked):
		return http.StatusConflict
	case errors.Is(err, ErrInvitationExpired):
		return http.StatusGone
	default:
		return http.StatusBadRequest
	}
}

// RegisterRoutes mounts all authentication, user, role, and API token routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Public Auth Endpoints
	r.Group(func(public chi.Router) {
		public.Post("/auth/login", h.handleLogin)
		public.Post("/auth/register", h.Register)
		public.Post("/auth/logout", h.Logout)
		public.Post("/auth/mfa/verify", h.VerifyMFALogin)
		public.Post("/auth/mfa/enroll", h.EnrollMFA)
		public.Post("/auth/mfa/confirm", h.ConfirmMFA)
		public.Post("/auth/reset-expired-password", h.ResetExpiredPassword)
		public.Get("/auth/invitations/preview", h.PreviewInvitation)
		public.Post("/auth/invitations/accept", h.AcceptInvitation)
	})

	// Protected Endpoints
	r.Group(func(protected chi.Router) {
		protected.Use(RequireAuth)

		// Current User
		protected.Get("/auth/me", h.Me)
		protected.Patch("/auth/me", h.UpdateMe)
		protected.Post("/auth/change-password", h.ChangePassword)
		protected.Post("/auth/mfa/disable", h.DisableMFA)
		protected.Post("/auth/mfa/regenerate-recovery-codes", h.RegenerateRecoveryCodes)

		// Admin Invitations
		protected.With(RequirePermission("read", "users")).Get("/users/invitations", h.ListInvitations)
		protected.With(RequirePermission("create", "users")).Post("/users/invitations", h.CreateInvitation)
		protected.With(RequirePermission("create", "users")).Post("/users/invitations/{id}/resend", h.ResendInvitation)
		protected.With(RequirePermission("update", "users")).Post("/users/invitations/{id}/revoke", h.RevokeInvitation)
		protected.With(RequirePermission("delete", "users")).Delete("/users/invitations/{id}", h.DeleteInvitation)

		// User Management
		protected.With(RequirePermission("read", "users")).Get("/users", h.ListUsers)
		protected.With(RequirePermission("create", "users")).Post("/users", h.CreateUser)
		protected.Get("/users/{id}", h.GetUser)
		protected.Patch("/users/{id}", h.UpdateUser)
		protected.With(RequirePermission("update", "users")).Post("/users/{id}/activate", h.ActivateUser)
		protected.With(RequirePermission("update", "users")).Post("/users/{id}/deactivate", h.DeactivateUser)
		protected.With(RequirePermission("update", "users")).Post("/users/{id}/unlock", h.UnlockUser)
		protected.With(RequirePermission("delete", "users")).Delete("/users/{id}", h.DeleteUser)

		// Roles & Permissions
		protected.With(RequirePermission("read", "roles")).Get("/roles", h.ListRoles)
		protected.With(RequirePermission("read", "roles")).Get("/roles/{id}", h.GetRole)
		protected.With(RequirePermission("create", "roles")).Post("/roles", h.CreateRole)
		protected.With(RequirePermission("update", "roles")).Put("/roles/{id}", h.UpdateRole)
		protected.With(RequirePermission("create", "roles")).Post("/roles/{id}/duplicate", h.DuplicateRole)
		protected.With(RequirePermission("delete", "roles")).Delete("/roles/{id}", h.DeleteRole)
		protected.With(RequirePermission("read", "roles")).Get("/permissions", h.ListPermissions)

		// API, Admin & Transfer Tokens
		protected.With(RequirePermission("create", "api_tokens")).Post("/api-tokens", h.CreateAPIToken)
		protected.With(RequirePermission("read", "api_tokens")).Get("/api-tokens", h.ListAPITokens)
		protected.With(RequirePermission("create", "api_tokens")).Post("/api-tokens/{id}/rotate", h.RotateAPIToken)
		protected.With(RequirePermission("delete", "api_tokens")).Post("/api-tokens/{id}/revoke", h.RevokeAPIToken)
		protected.With(RequirePermission("delete", "api_tokens")).Delete("/api-tokens/{id}", h.DeleteAPIToken)

		protected.With(RequirePermission("create", "api_tokens")).Post("/transfer-tokens", h.CreateTransferToken)
		protected.With(RequirePermission("read", "api_tokens")).Get("/transfer-tokens", h.ListTransferTokens)
		protected.With(RequirePermission("create", "api_tokens")).Post("/transfer-tokens/{id}/rotate", h.RotateAPIToken)
		protected.With(RequirePermission("delete", "api_tokens")).Post("/transfer-tokens/{id}/revoke", h.RevokeAPIToken)
		protected.With(RequirePermission("delete", "api_tokens")).Delete("/transfer-tokens/{id}", h.DeleteAPIToken)
	})
}

type LoginRequestBody struct {
	Identifier string `json:"identifier"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

// Login authenticates a user and returns a signed access token.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var body LoginRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	identifier := body.Identifier
	if identifier == "" {
		identifier = body.Username
	}

	user, token, err := h.svc.Authenticate(r.Context(), identifier, body.Password)
	if err != nil {
		status := http.StatusUnauthorized
		if err == ErrUserSuspended {
			status = http.StatusForbidden
		}
		respondJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	isProduction := h.svc.cfg.Server.Env == "production"
	SetAuthCookie(w, token, h.svc.cfg.Auth.TokenExpiration, isProduction)

	respondJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}

// Register creates a new user account.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	user, err := h.svc.Register(r.Context(), req)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusCreated, user)
}

// Logout clears the authentication cookie.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	ClearAuthCookie(w)
	respondJSON(w, http.StatusOK, map[string]string{"message": "successfully logged out"})
}

// Me returns the profile of the currently authenticated user.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	respondJSON(w, http.StatusOK, user)
}

// UpdateMe updates the currently authenticated user's profile fields.
func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if GetAPITokenFromContext(r.Context()) != nil {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "api tokens cannot modify user profiles"})
		return
	}
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	updated, err := h.svc.UpdateProfile(r.Context(), user.ID, req)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, updated)
}

// ChangePassword allows the currently authenticated user to change their own password.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if GetAPITokenFromContext(r.Context()) != nil {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "api tokens cannot change passwords"})
		return
	}
	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.svc.ChangePassword(r.Context(), user.ID, req); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "current password is incorrect" {
			status = http.StatusUnauthorized
		}
		respondJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "password updated successfully"})
}

// ListUsers returns a filtered, paginated list of users.
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	filter := UserListFilter{
		Search: r.URL.Query().Get("search"),
		Status: r.URL.Query().Get("status"),
		RoleID: r.URL.Query().Get("role_id"),
		Limit:  limit,
		Offset: offset,
	}
	users, total, err := h.svc.ListUsersFiltered(r.Context(), filter)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data":  users,
		"total": total,
	})
}

// CreateUser creates a new user with optional roles and status (admin endpoint).
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	currentUser := GetUserFromContext(r.Context())
	var req CreateAdminUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	user, err := h.svc.CreateAdminUser(r.Context(), currentUser, req, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusCreated, user)
}

// GetUser returns user details by ID.
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())
	if currentUser == nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// Non-admin can only view their own profile
	if currentUser.ID != targetID && !HasPermission(currentUser, "read", "users") {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	user, err := h.svc.GetUserProfile(r.Context(), targetID)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	respondJSON(w, http.StatusOK, user)
}

// UpdateUser modifies a user's profile, status, or roles.
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())
	if currentUser == nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// Non-admin can only update their own profile and cannot change roles
	if currentUser.ID != targetID && !HasPermission(currentUser, "update", "users") {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updated, err := h.svc.UpdateUserByActor(r.Context(), currentUser, targetID, req, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

// ActivateUser sets a user account status to active.
func (h *Handler) ActivateUser(w http.ResponseWriter, r *http.Request) {
	h.setUserStatus(w, r, StatusActive)
}

// DeactivateUser sets a user account status to inactive.
func (h *Handler) DeactivateUser(w http.ResponseWriter, r *http.Request) {
	h.setUserStatus(w, r, StatusInactive)
}

func (h *Handler) setUserStatus(w http.ResponseWriter, r *http.Request, status UserStatus) {
	targetID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())
	updated, err := h.svc.SetUserStatus(r.Context(), currentUser, targetID, status, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, updated)
}

// DeleteUser removes a user account.
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())

	if err := h.svc.DeleteUserByActor(r.Context(), currentUser, targetID, auditMetaFromRequest(r)); err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "user deleted successfully"})
}

// ListInvitations returns all admin onboarding invitations.
func (h *Handler) ListInvitations(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListInvitations(r.Context())
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"data": list})
}

// CreateInvitation issues a hashed expiring invitation token and returns the plain token once.
func (h *Handler) CreateInvitation(w http.ResponseWriter, r *http.Request) {
	currentUser := GetUserFromContext(r.Context())
	var req CreateInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	rawToken, inv, err := h.svc.CreateInvitation(r.Context(), currentUser, req, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	resp := map[string]any{
		"invitation": inv,
		"token":      rawToken,
	}
	if inv.EmailSent != nil {
		resp["email_sent"] = *inv.EmailSent
	}
	respondJSON(w, http.StatusCreated, resp)
}

// ResendInvitation rotates an invitation token and extends expiration.
func (h *Handler) ResendInvitation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())
	var body struct {
		ExpiresInHours int `json:"expires_in_hours"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	rawToken, inv, err := h.svc.ResendInvitation(r.Context(), currentUser, id, body.ExpiresInHours, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	resp := map[string]any{
		"invitation": inv,
		"token":      rawToken,
	}
	if inv.EmailSent != nil {
		resp["email_sent"] = *inv.EmailSent
	}
	respondJSON(w, http.StatusOK, resp)
}

// RevokeInvitation marks an invitation revoked so it cannot be redeemed.
func (h *Handler) RevokeInvitation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inv, err := h.svc.RevokeInvitation(r.Context(), id, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, inv)
}

// DeleteInvitation removes an invitation record.
func (h *Handler) DeleteInvitation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.DeleteInvitation(r.Context(), id, auditMetaFromRequest(r)); err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "invitation deleted"})
}

// PreviewInvitation validates a raw invitation token and returns safe onboarding metadata.
func (h *Handler) PreviewInvitation(w http.ResponseWriter, r *http.Request) {
	rawToken := strings.TrimSpace(r.URL.Query().Get("token"))
	inv, err := h.svc.PreviewInvitation(r.Context(), rawToken)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"email":      inv.Email,
		"username":   inv.Username,
		"first_name": inv.FirstName,
		"last_name":  inv.LastName,
		"expires_at": inv.ExpiresAt,
		"roles":      inv.Roles,
	})
}

// AcceptInvitation redeems an invitation token, creates the admin account, and signs the user in.
func (h *Handler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var req AcceptInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	user, token, err := h.svc.AcceptInvitation(r.Context(), req, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	isProduction := h.svc.cfg.Server.Env == "production"
	SetAuthCookie(w, token, h.svc.cfg.Auth.TokenExpiration, isProduction)
	respondJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"user":  user,
	})
}

// ListRoles returns all registered system roles.
func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.ListRoles(r.Context())
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"data": roles})
}

// GetRole returns details and permissions for a single role.
func (h *Handler) GetRole(w http.ResponseWriter, r *http.Request) {
	role, err := h.svc.GetRole(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, role)
}

// CreateRole creates a custom admin role.
func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req RoleMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	role, err := h.svc.CreateRole(r.Context(), req, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusCreated, role)
}

// UpdateRole updates a role and its permissions.
func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	var req RoleMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	role, err := h.svc.UpdateRole(r.Context(), chi.URLParam(r, "id"), req, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, role)
}

// DuplicateRole clones an existing role and its permissions.
func (h *Handler) DuplicateRole(w http.ResponseWriter, r *http.Request) {
	var req RoleMutationRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	role, err := h.svc.DuplicateRole(r.Context(), chi.URLParam(r, "id"), req, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusCreated, role)
}

// DeleteRole deletes an unused custom role.
func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteRole(r.Context(), chi.URLParam(r, "id"), auditMetaFromRequest(r)); err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "role deleted successfully"})
}

// ListPermissions returns the catalog of assignable RBAC permissions.
func (h *Handler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.svc.ListPermissions(r.Context())
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"data": perms})
}

type CreateTokenRequestBody struct {
	Name            string    `json:"name"`
	Kind            TokenKind `json:"kind"`
	Description     string    `json:"description"`
	TokenType       string    `json:"token_type"`
	Permissions     []string  `json:"permissions"`
	DurationSeconds *int64    `json:"duration_seconds"`
	NeverExpires    bool      `json:"never_expires"`
}

func (h *Handler) handleCreateToken(w http.ResponseWriter, r *http.Request, forcedKind TokenKind) {
	currentUser := GetUserFromContext(r.Context())
	var body CreateTokenRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if strings.TrimSpace(body.Name) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	kind := body.Kind
	if forcedKind != "" {
		kind = forcedKind
	}

	rawToken, token, err := h.svc.CreateTokenWithOptions(r.Context(), currentUser, CreateTokenRequest{
		Name:            body.Name,
		Kind:            kind,
		Description:     body.Description,
		TokenType:       body.TokenType,
		Permissions:     body.Permissions,
		DurationSeconds: body.DurationSeconds,
		NeverExpires:    body.NeverExpires,
	}, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"token":    rawToken, // Plain token returned only once upon creation
		"metadata": token,
	})
}

// CreateAPIToken generates a new API, Admin, or Transfer Token for the authenticated user.
func (h *Handler) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	h.handleCreateToken(w, r, "")
}

// CreateTransferToken generates a new Transfer Token for CLI/migration sync operations.
func (h *Handler) CreateTransferToken(w http.ResponseWriter, r *http.Request) {
	h.handleCreateToken(w, r, TokenKindTransfer)
}

// ListAPITokens returns API/Admin/Transfer tokens visible to the authenticated user.
func (h *Handler) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	currentUser := GetUserFromContext(r.Context())
	kind := TokenKind(strings.TrimSpace(r.URL.Query().Get("kind")))
	tokens, err := h.svc.ListTokensByKind(r.Context(), currentUser, kind)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"data": tokens})
}

// ListTransferTokens returns Transfer tokens visible to the authenticated user.
func (h *Handler) ListTransferTokens(w http.ResponseWriter, r *http.Request) {
	currentUser := GetUserFromContext(r.Context())
	tokens, err := h.svc.ListTokensByKind(r.Context(), currentUser, TokenKindTransfer)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"data": tokens})
}

// RotateAPIToken rotates an active token and returns its replacement plaintext secret once.
func (h *Handler) RotateAPIToken(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())
	rawToken, token, err := h.svc.RotateAPIToken(r.Context(), currentUser, tokenID, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"token":    rawToken,
		"metadata": token,
	})
}

// RevokeAPIToken marks an API token as revoked so it can no longer authenticate.
func (h *Handler) RevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())
	if err := h.svc.RevokeAPITokenByActor(r.Context(), currentUser, tokenID, auditMetaFromRequest(r)); err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "token revoked successfully"})
}

// DeleteAPIToken permanently removes an API, Admin, or Transfer token.
func (h *Handler) DeleteAPIToken(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())
	if err := h.svc.DeleteAPITokenByActor(r.Context(), currentUser, tokenID, auditMetaFromRequest(r)); err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "token deleted successfully"})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

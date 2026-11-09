package auth

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

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

// RegisterRoutes mounts all authentication, user, role, and API token routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Public Auth Endpoints
	r.Group(func(public chi.Router) {
		public.Post("/auth/login", h.Login)
		public.Post("/auth/register", h.Register)
		public.Post("/auth/logout", h.Logout)
	})

	// Protected Endpoints
	r.Group(func(protected chi.Router) {
		protected.Use(RequireAuth)

		// Current User
		protected.Get("/auth/me", h.Me)

		// User Management
		protected.With(RequirePermission("read", "users")).Get("/users", h.ListUsers)
		protected.With(RequirePermission("create", "users")).Post("/users", h.CreateUser)
		protected.Get("/users/{id}", h.GetUser)
		protected.Patch("/users/{id}", h.UpdateUser)
		protected.With(RequirePermission("delete", "users")).Delete("/users/{id}", h.DeleteUser)

		// Roles
		protected.With(RequirePermission("read", "roles")).Get("/roles", h.ListRoles)

		// API Tokens
		protected.Post("/api-tokens", h.CreateAPIToken)
		protected.Get("/api-tokens", h.ListAPITokens)
		protected.Delete("/api-tokens/{id}", h.RevokeAPIToken)
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

// ListUsers returns a paginated list of users.
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	users, total, err := h.svc.ListUsers(r.Context(), limit, offset)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data":  users,
		"total": total,
	})
}

// CreateUser creates a new user (admin endpoint).
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
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

// GetUser returns user details by ID.
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())

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

// UpdateUser modifies a user's profile.
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())

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

	// Disallow privilege escalation
	if currentUser.ID == targetID && !HasPermission(currentUser, "*", "users") {
		req.RoleIDs = nil
		req.Status = ""
	}

	updated, err := h.svc.UpdateUser(r.Context(), targetID, req)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

// DeleteUser removes a user account.
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())

	if currentUser.ID == targetID {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete your own account"})
		return
	}

	if err := h.svc.DeleteUser(r.Context(), targetID); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "user deleted successfully"})
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

type CreateTokenRequestBody struct {
	Name            string   `json:"name"`
	Permissions     []string `json:"permissions"`
	DurationSeconds *int64   `json:"duration_seconds"`
}

// CreateAPIToken generates a new Personal Access Token for the authenticated user.
func (h *Handler) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	currentUser := GetUserFromContext(r.Context())
	var body CreateTokenRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if body.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	var duration *time.Duration
	if body.DurationSeconds != nil && *body.DurationSeconds > 0 {
		d := time.Duration(*body.DurationSeconds) * time.Second
		duration = &d
	}

	rawToken, token, err := h.svc.CreateAPIToken(r.Context(), currentUser.ID, body.Name, body.Permissions, duration)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"token":    rawToken, // Plain token returned only once upon creation
		"metadata": token,
	})
}

// ListAPITokens returns active API tokens for the authenticated user.
func (h *Handler) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	currentUser := GetUserFromContext(r.Context())
	tokens, err := h.svc.ListAPITokens(r.Context(), currentUser.ID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"data": tokens})
}

// RevokeAPIToken deletes an API token.
func (h *Handler) RevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "id")
	currentUser := GetUserFromContext(r.Context())

	if err := h.svc.RevokeAPIToken(r.Context(), tokenID, currentUser.ID); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "token revoked successfully"})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}


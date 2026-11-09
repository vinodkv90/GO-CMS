package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("invalid username/email or password")
	ErrUserSuspended      = errors.New("user account is suspended or inactive")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden: insufficient permissions")
)

// RegisterRequest holds data for registering a new user.
type RegisterRequest struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// UpdateUserRequest holds data for updating user details.
type UpdateUserRequest struct {
	Email     string     `json:"email"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Status    UserStatus `json:"status"`
	RoleIDs   []string   `json:"role_ids"`
}

// Service provides business logic for user authentication, registration, RBAC, and token management.
type Service struct {
	userRepo  UserRepository
	roleRepo  RoleRepository
	permRepo  PermissionRepository
	tokenRepo APITokenRepository
	cfg       *config.Config
	logger    *slog.Logger
}

// NewService creates a new authentication service.
func NewService(
	userRepo UserRepository,
	roleRepo RoleRepository,
	permRepo PermissionRepository,
	tokenRepo APITokenRepository,
	cfg *config.Config,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		userRepo:  userRepo,
		roleRepo:  roleRepo,
		permRepo:  permRepo,
		tokenRepo: tokenRepo,
		cfg:       cfg,
		logger:    logger,
	}
}

// Authenticate verifies user credentials and returns the user entity along with a signed JWT access token.
func (s *Service) Authenticate(ctx context.Context, identifier, password string) (*User, string, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || password == "" {
		return nil, "", ErrInvalidCredentials
	}

	var user *User
	var err error

	if strings.Contains(identifier, "@") {
		user, err = s.userRepo.GetUserByEmail(ctx, identifier)
	} else {
		user, err = s.userRepo.GetUserByUsername(ctx, identifier)
	}

	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", fmt.Errorf("failed to lookup user: %w", err)
	}

	// Verify Argon2id password hash
	match, err := ComparePassword(password, user.PasswordHash)
	if err != nil || !match {
		return nil, "", ErrInvalidCredentials
	}

	if user.Status != StatusActive {
		return nil, "", ErrUserSuspended
	}

	// Generate JWT Access Token
	token, err := GenerateAccessToken(user, s.cfg.Auth.JWTSecret, s.cfg.Auth.TokenExpiration)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate access token: %w", err)
	}

	return user, token, nil
}

// Register creates a new user account.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*User, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Username == "" || req.Email == "" {
		return nil, errors.New("username and email are required")
	}

	if err := ValidatePasswordStrength(req.Password); err != nil {
		return nil, err
	}

	passwordHash, err := HashPassword(req.Password, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC()
	user := &User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		Status:       StatusActive,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// SeedDefaultRolesAndAdmin seeds standard roles/permissions and an initial super_admin user if no users exist.
func (s *Service) SeedDefaultRolesAndAdmin(ctx context.Context, adminUser, adminEmail, adminPass string) (*User, error) {
	// 1. Seed Permissions
	permissions := []Permission{
		{ID: "perm_all", Action: "*", Resource: "*", Description: "Full administrative access"},
		{ID: "perm_content_read", Action: "read", Resource: "content", Description: "Read content items"},
		{ID: "perm_content_create", Action: "create", Resource: "content", Description: "Create content items"},
		{ID: "perm_content_update", Action: "update", Resource: "content", Description: "Update content items"},
		{ID: "perm_content_delete", Action: "delete", Resource: "content", Description: "Delete content items"},
		{ID: "perm_content_publish", Action: "publish", Resource: "content", Description: "Publish content items"},
		{ID: "perm_media_read", Action: "read", Resource: "media", Description: "View media files"},
		{ID: "perm_media_create", Action: "create", Resource: "media", Description: "Upload media files"},
		{ID: "perm_media_delete", Action: "delete", Resource: "media", Description: "Delete media files"},
		{ID: "perm_users_read", Action: "read", Resource: "users", Description: "Read user profiles"},
		{ID: "perm_users_manage", Action: "*", Resource: "users", Description: "Manage system users"},
		{ID: "perm_settings_manage", Action: "*", Resource: "settings", Description: "Manage system settings"},
	}

	for _, p := range permissions {
		p.CreatedAt = time.Now().UTC()
		_ = s.permRepo.CreatePermission(ctx, &p)
	}

	// 2. Seed Roles
	roles := []struct {
		Role  Role
		Perms []string
	}{
		{
			Role: Role{
				ID:          "role_super_admin",
				Name:        "Super Administrator",
				Slug:        RoleSuperAdmin,
				Description: "Unrestricted system access",
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			},
			Perms: []string{"perm_all"},
		},
		{
			Role: Role{
				ID:          "role_editor",
				Name:        "Editor",
				Slug:        RoleEditor,
				Description: "Can manage and publish all content and media",
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			},
			Perms: []string{"perm_content_read", "perm_content_create", "perm_content_update", "perm_content_delete", "perm_content_publish", "perm_media_read", "perm_media_create", "perm_media_delete"},
		},
		{
			Role: Role{
				ID:          "role_author",
				Name:        "Author",
				Slug:        RoleAuthor,
				Description: "Can create and edit own content and upload media",
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			},
			Perms: []string{"perm_content_read", "perm_content_create", "perm_content_update", "perm_media_read", "perm_media_create"},
		},
		{
			Role: Role{
				ID:          "role_viewer",
				Name:        "Viewer",
				Slug:        RoleViewer,
				Description: "Read-only access to published content",
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			},
			Perms: []string{"perm_content_read", "perm_media_read"},
		},
	}

	for _, r := range roles {
		_ = s.roleRepo.CreateRole(ctx, &r.Role)
		for _, permID := range r.Perms {
			_ = s.permRepo.AssignPermissionToRole(ctx, r.Role.ID, permID)
		}
	}

	// 3. Seed Super Admin user if no users exist
	count, err := s.userRepo.CountUsers(ctx)
	if err == nil && count == 0 {
		if adminUser == "" {
			adminUser = "admin"
		}
		if adminEmail == "" {
			adminEmail = "admin@example.com"
		}
		if adminPass == "" {
			adminPass = "Admin@CMS2026!"
		}

		admin, err := s.Register(ctx, RegisterRequest{
			Username:  adminUser,
			Email:     adminEmail,
			Password:  adminPass,
			FirstName: "System",
			LastName:  "Admin",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create default admin: %w", err)
		}

		_ = s.roleRepo.AssignRoleToUser(ctx, admin.ID, "role_super_admin")
		s.logger.Info("Seeded initial Super Admin account", "username", adminUser, "email", adminEmail)
		return admin, nil
	}

	return nil, nil
}

// CreateAPIToken generates a new Personal Access Token for headless API requests.
func (s *Service) CreateAPIToken(ctx context.Context, userID, name string, permissions []string, duration *time.Duration) (string, *APIToken, error) {
	rawToken, hash, err := GenerateAPIToken()
	if err != nil {
		return "", nil, err
	}

	now := time.Now().UTC()
	var expiresAt *time.Time
	if duration != nil && *duration > 0 {
		exp := now.Add(*duration)
		expiresAt = &exp
	}

	token := &APIToken{
		ID:          uuid.New().String(),
		Name:        name,
		TokenHash:   hash,
		UserID:      userID,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}

	if err := s.tokenRepo.CreateToken(ctx, token); err != nil {
		return "", nil, fmt.Errorf("failed to save api token: %w", err)
	}

	return rawToken, token, nil
}

// ValidateAPIToken checks a raw Personal Access Token and returns the token model and user.
func (s *Service) ValidateAPIToken(ctx context.Context, rawToken string) (*APIToken, *User, error) {
	hash := HashAPIToken(rawToken)
	token, err := s.tokenRepo.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, nil, ErrUnauthorized
	}

	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now().UTC()) {
		return nil, nil, ErrTokenExpired
	}

	user, err := s.userRepo.GetUserByID(ctx, token.UserID)
	if err != nil || user.Status != StatusActive {
		return nil, nil, ErrUnauthorized
	}

	// Update last used timestamp asynchronously
	go func() {
		_ = s.tokenRepo.UpdateLastUsed(context.Background(), token.ID, time.Now().UTC())
	}()

	return token, user, nil
}

// GetUserProfile returns a user by ID.
func (s *Service) GetUserProfile(ctx context.Context, userID string) (*User, error) {
	return s.userRepo.GetUserByID(ctx, userID)
}

// UpdateUser updates a user profile and role assignments.
func (s *Service) UpdateUser(ctx context.Context, userID string, req UpdateUserRequest) (*User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if req.Email != "" {
		user.Email = strings.TrimSpace(strings.ToLower(req.Email))
	}
	if req.FirstName != "" {
		user.FirstName = req.FirstName
	}
	if req.LastName != "" {
		user.LastName = req.LastName
	}
	if req.Status != "" {
		user.Status = req.Status
	}
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	// Reassign roles if provided
	if req.RoleIDs != nil {
		for _, r := range user.Roles {
			_ = s.roleRepo.RemoveRoleFromUser(ctx, user.ID, r.ID)
		}
		for _, rid := range req.RoleIDs {
			_ = s.roleRepo.AssignRoleToUser(ctx, user.ID, rid)
		}
	}

	return s.userRepo.GetUserByID(ctx, userID)
}

// DeleteUser removes a user account.
func (s *Service) DeleteUser(ctx context.Context, userID string) error {
	return s.userRepo.DeleteUser(ctx, userID)
}

// ListUsers returns a paginated list of users.
func (s *Service) ListUsers(ctx context.Context, limit, offset int) ([]*User, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.userRepo.ListUsers(ctx, limit, offset)
}

// ListRoles returns all roles in the system.
func (s *Service) ListRoles(ctx context.Context) ([]*Role, error) {
	return s.roleRepo.ListRoles(ctx)
}

// ListAPITokens returns active API tokens for a user.
func (s *Service) ListAPITokens(ctx context.Context, userID string) ([]*APIToken, error) {
	return s.tokenRepo.ListTokensByUserID(ctx, userID)
}

// RevokeAPIToken deletes an API token.
func (s *Service) RevokeAPIToken(ctx context.Context, tokenID, userID string) error {
	return s.tokenRepo.DeleteToken(ctx, tokenID, userID)
}

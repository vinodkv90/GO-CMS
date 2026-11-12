package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials  = errors.New("invalid username/email or password")
	ErrUserSuspended       = errors.New("user account is suspended or inactive")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden: insufficient permissions")
	ErrCannotDeleteSelf    = errors.New("cannot delete your own account")
	ErrLastSuperAdmin      = errors.New("cannot remove or deactivate the last active super administrator")
	ErrSystemRoleProtected = errors.New("system role cannot be modified or deleted in this way")
	ErrRoleInUse           = errors.New("role is currently assigned to one or more users")
)

var roleSlugSanitizer = regexp.MustCompile(`[^a-z0-9_]+`)

// AuditMeta holds actor context for audit logging without importing internal/audit.
type AuditMeta struct {
	ActorID    string
	ActorEmail string
	IP         string
	UserAgent  string
}

// AuditRecorder is a callback invoked by Service for administrative audit trails.
type AuditRecorder func(ctx context.Context, meta AuditMeta, action, resourceType, resourceID string, details map[string]any, result string)

// RegisterRequest holds data for registering a new user.
type RegisterRequest struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// CreateAdminUserRequest holds admin-initiated user creation fields.
type CreateAdminUserRequest struct {
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	Password  string     `json:"password"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Status    UserStatus `json:"status"`
	RoleIDs   []string   `json:"role_ids"`
}

// UpdateUserRequest holds data for updating user details.
type UpdateUserRequest struct {
	Email     string     `json:"email"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Status    UserStatus `json:"status"`
	RoleIDs   []string   `json:"role_ids"`
}

// RoleMutationRequest holds payload for creating, updating, or duplicating a role.
type RoleMutationRequest struct {
	Name          string   `json:"name"`
	Slug          string   `json:"slug"`
	Description   string   `json:"description"`
	PermissionIDs []string `json:"permission_ids"`
}

// CreateInvitationRequest holds payload for creating an admin onboarding invitation.
type CreateInvitationRequest struct {
	Email          string   `json:"email"`
	Username       string   `json:"username"`
	FirstName      string   `json:"first_name"`
	LastName       string   `json:"last_name"`
	RoleIDs        []string `json:"role_ids"`
	ExpiresInHours int      `json:"expires_in_hours"`
}

// AcceptInvitationRequest holds payload to accept an invitation and set initial credentials.
type AcceptInvitationRequest struct {
	Token     string `json:"token"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// InviteNotifier delivers onboarding invitation emails when invitations are created or resent.
type InviteNotifier func(ctx context.Context, inv *AdminInvitation, rawToken string, actor *User, expiresInHours int) error

// Service provides business logic for user authentication, registration, RBAC, and token management.
type Service struct {
	userRepo         UserRepository
	roleRepo         RoleRepository
	permRepo         PermissionRepository
	tokenRepo        APITokenRepository
	inviteRepo       InvitationRepository
	auditFn          AuditRecorder
	inviteNotifier   InviteNotifier
	securityPolicyFn SecurityPolicyProvider
	cfg              *config.Config
	logger           *slog.Logger
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
	svc := &Service{
		userRepo:  userRepo,
		roleRepo:  roleRepo,
		permRepo:  permRepo,
		tokenRepo: tokenRepo,
		cfg:       cfg,
		logger:    logger,
	}
	if ir, ok := userRepo.(InvitationRepository); ok {
		svc.inviteRepo = ir
	}
	return svc
}

// SetInvitationRepository attaches a dedicated InvitationRepository implementation.
func (s *Service) SetInvitationRepository(repo InvitationRepository) {
	s.inviteRepo = repo
}

// SetAuditRecorder attaches an audit callback for administrative security events.
func (s *Service) SetAuditRecorder(fn AuditRecorder) {
	s.auditFn = fn
}

// SetInviteNotifier registers an outbound delivery callback for admin invitation emails.
func (s *Service) SetInviteNotifier(fn InviteNotifier) {
	s.inviteNotifier = fn
}

func (s *Service) recordAudit(ctx context.Context, meta AuditMeta, action, resourceType, resourceID string, details map[string]any, result string) {
	if s.auditFn != nil {
		s.auditFn(ctx, meta, action, resourceType, resourceID, details, result)
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

	// Record last login timestamp
	now := time.Now().UTC()
	user.LastLoginAt = &now
	user.UpdatedAt = now
	_ = s.userRepo.UpdateUser(ctx, user)

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

	if err := s.validatePasswordForPolicy(ctx, req.Password); err != nil {
		return nil, err
	}

	passwordHash, err := HashPassword(req.Password, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC()
	user := &User{
		ID:                uuid.New().String(),
		Username:          req.Username,
		Email:             req.Email,
		PasswordHash:      passwordHash,
		Status:            StatusActive,
		FirstName:         req.FirstName,
		LastName:          req.LastName,
		PasswordChangedAt: &now,
		CreatedAt:         now,
		UpdatedAt:         now,
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
		// Content Manager & Content-Type Builder
		{ID: "perm_content_read", Action: "read", Resource: "content", Description: "Read content items"},
		{ID: "perm_content_create", Action: "create", Resource: "content", Description: "Create content items"},
		{ID: "perm_content_update", Action: "update", Resource: "content", Description: "Update content items"},
		{ID: "perm_content_delete", Action: "delete", Resource: "content", Description: "Delete content items"},
		{ID: "perm_content_publish", Action: "publish", Resource: "content", Description: "Publish content items"},
		{ID: "perm_content_types_manage", Action: "manage", Resource: "content_types", Description: "Manage content types and components"},
		// Media Library
		{ID: "perm_media_read", Action: "read", Resource: "media", Description: "View media files"},
		{ID: "perm_media_create", Action: "create", Resource: "media", Description: "Upload media files"},
		{ID: "perm_media_update", Action: "update", Resource: "media", Description: "Update media metadata and folders"},
		{ID: "perm_media_delete", Action: "delete", Resource: "media", Description: "Delete media files and folders"},
		// Admin Users & Roles
		{ID: "perm_users_read", Action: "read", Resource: "users", Description: "Read user profiles"},
		{ID: "perm_users_create", Action: "create", Resource: "users", Description: "Create and invite admin users"},
		{ID: "perm_users_update", Action: "update", Resource: "users", Description: "Update admin users"},
		{ID: "perm_users_delete", Action: "delete", Resource: "users", Description: "Delete admin users"},
		{ID: "perm_users_manage", Action: "*", Resource: "users", Description: "Manage system users"},
		{ID: "perm_roles_read", Action: "read", Resource: "roles", Description: "Read admin roles"},
		{ID: "perm_roles_create", Action: "create", Resource: "roles", Description: "Create admin roles"},
		{ID: "perm_roles_update", Action: "update", Resource: "roles", Description: "Update admin roles and permissions"},
		{ID: "perm_roles_delete", Action: "delete", Resource: "roles", Description: "Delete admin roles"},
		// Plugins, API Tokens, Webhooks, Users & Permissions
		{ID: "perm_plugins_read", Action: "read", Resource: "plugins", Description: "View plugins and plugin settings"},
		{ID: "perm_plugins_update", Action: "update", Resource: "plugins", Description: "Enable, disable, and configure plugins"},
		{ID: "perm_api_tokens_read", Action: "read", Resource: "api_tokens", Description: "View API and transfer tokens"},
		{ID: "perm_api_tokens_create", Action: "create", Resource: "api_tokens", Description: "Create API and transfer tokens"},
		{ID: "perm_api_tokens_delete", Action: "delete", Resource: "api_tokens", Description: "Revoke and delete API tokens"},
		{ID: "perm_webhooks_read", Action: "read", Resource: "webhooks", Description: "View webhooks and delivery logs"},
		{ID: "perm_webhooks_create", Action: "create", Resource: "webhooks", Description: "Create webhooks"},
		{ID: "perm_webhooks_update", Action: "update", Resource: "webhooks", Description: "Update webhooks"},
		{ID: "perm_webhooks_delete", Action: "delete", Resource: "webhooks", Description: "Delete webhooks"},
		{ID: "perm_users_permissions_read", Action: "read", Resource: "users_permissions", Description: "View end-user roles, users, and providers"},
		{ID: "perm_users_permissions_update", Action: "update", Resource: "users_permissions", Description: "Manage end-user roles, users, and providers"},
		// Settings & System
		{ID: "perm_settings_read", Action: "read", Resource: "settings", Description: "Read system settings"},
		{ID: "perm_settings_update", Action: "update", Resource: "settings", Description: "Update system settings"},
		{ID: "perm_settings_manage", Action: "*", Resource: "settings", Description: "Manage system settings"},
		{ID: "perm_system_info_read", Action: "read", Resource: "system:info", Description: "View system information"},
		{ID: "perm_system_audit_read", Action: "read", Resource: "system:audit_logs", Description: "View audit logs"},
		{ID: "perm_system_jobs_read", Action: "read", Resource: "system:jobs", Description: "View background jobs"},
		{ID: "perm_system_jobs_update", Action: "update", Resource: "system:jobs", Description: "Retry background jobs"},
		{ID: "perm_system_locales_read", Action: "read", Resource: "system:locales", Description: "View locales"},
		{ID: "perm_system_locales_create", Action: "create", Resource: "system:locales", Description: "Create locales"},
		{ID: "perm_system_locales_update", Action: "update", Resource: "system:locales", Description: "Update locales"},
		{ID: "perm_system_locales_delete", Action: "delete", Resource: "system:locales", Description: "Delete locales"},
		{ID: "perm_system_tenants_read", Action: "read", Resource: "system:tenants", Description: "View tenants"},
		{ID: "perm_system_tenants_create", Action: "create", Resource: "system:tenants", Description: "Create tenants"},
		{ID: "perm_system_tenants_update", Action: "update", Resource: "system:tenants", Description: "Update tenants"},
		{ID: "perm_system_tenants_delete", Action: "delete", Resource: "system:tenants", Description: "Delete tenants"},
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
			Perms: []string{"perm_content_read", "perm_content_create", "perm_content_update", "perm_content_delete", "perm_content_publish", "perm_media_read", "perm_media_create", "perm_media_update", "perm_media_delete"},
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

// CreateTokenRequest specifies parameters for creating an API, Admin, or Transfer token.
type CreateTokenRequest struct {
	Name            string    `json:"name"`
	Kind            TokenKind `json:"kind"`
	Description     string    `json:"description"`
	TokenType       string    `json:"token_type"`
	Permissions     []string  `json:"permissions"`
	DurationSeconds *int64    `json:"duration_seconds"`
	NeverExpires    bool      `json:"never_expires"`
}

// CreateAPIToken generates a new Personal Access Token for headless API requests.
func (s *Service) CreateAPIToken(ctx context.Context, userID, name string, permissions []string, duration *time.Duration) (string, *APIToken, error) {
	actor, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return "", nil, ErrUnauthorized
	}
	var durSec *int64
	if duration != nil && *duration > 0 {
		sec := int64(duration.Seconds())
		durSec = &sec
	}
	return s.CreateTokenWithOptions(ctx, actor, CreateTokenRequest{
		Name:            name,
		Kind:            TokenKindAPI,
		TokenType:       "custom",
		Permissions:     permissions,
		DurationSeconds: durSec,
		NeverExpires:    duration == nil || *duration <= 0,
	}, AuditMeta{})
}

// CreateTokenWithOptions generates a scoped token (API, Admin PAT, or Transfer) enforcing anti-escalation.
func (s *Service) CreateTokenWithOptions(ctx context.Context, actor *User, req CreateTokenRequest, meta AuditMeta) (string, *APIToken, error) {
	if actor == nil {
		return "", nil, ErrUnauthorized
	}
	freshActor, err := s.userRepo.GetUserByID(ctx, actor.ID)
	if err != nil || freshActor.Status != StatusActive {
		return "", nil, ErrUnauthorized
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return "", nil, errors.New("token name is required")
	}

	kind := req.Kind
	if kind == "" {
		kind = TokenKindAPI
	}
	switch kind {
	case TokenKindAPI, TokenKindAdmin, TokenKindTransfer:
	default:
		return "", nil, fmt.Errorf("unsupported token kind %q", kind)
	}

	if !HasPermission(freshActor, "create", "api_tokens") {
		return "", nil, ErrForbidden
	}

	tokenType, resolvedPerms, err := resolveTokenPermissions(ctx, freshActor, kind, req.TokenType, req.Permissions)
	if err != nil {
		return "", nil, err
	}

	rawToken, prefix, hash, err := GenerateTokenForKind(kind)
	if err != nil {
		return "", nil, err
	}

	now := time.Now().UTC()
	var expiresAt *time.Time
	if !req.NeverExpires && req.DurationSeconds != nil && *req.DurationSeconds > 0 {
		exp := now.Add(time.Duration(*req.DurationSeconds) * time.Second)
		expiresAt = &exp
	}

	token := &APIToken{
		ID:          uuid.New().String(),
		Name:        name,
		Kind:        kind,
		Description: strings.TrimSpace(req.Description),
		TokenType:   tokenType,
		TokenPrefix: prefix,
		TokenHash:   hash,
		UserID:      freshActor.ID,
		Permissions: resolvedPerms,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}

	if err := s.tokenRepo.CreateToken(ctx, token); err != nil {
		return "", nil, fmt.Errorf("failed to save api token: %w", err)
	}

	if meta.ActorID == "" {
		meta.ActorID = freshActor.ID
	}
	if meta.ActorEmail == "" {
		meta.ActorEmail = freshActor.Email
	}
	s.recordAudit(ctx, meta, "token.create", "api_token", token.ID, map[string]any{
		"name":        token.Name,
		"kind":        string(token.Kind),
		"token_type":  token.TokenType,
		"prefix":      token.TokenPrefix,
		"permissions": token.Permissions,
	}, "success")

	return rawToken, token, nil
}

func resolveTokenPermissions(ctx context.Context, actor *User, kind TokenKind, requestedType string, requestedPerms []string) (string, []string, error) {
	callerToken := GetAPITokenFromContext(ctx)
	allows := func(action, resource string) bool {
		if !HasPermission(actor, action, resource) {
			return false
		}
		if callerToken != nil && !TokenAllowsPermission(callerToken, action, resource) {
			return false
		}
		return true
	}

	filterAllowed := func(candidates []string) []string {
		out := make([]string, 0, len(candidates))
		seen := make(map[string]struct{}, len(candidates))
		for _, p := range candidates {
			if _, ok := seen[p]; ok {
				continue
			}
			parts := strings.SplitN(p, ":", 2)
			if len(parts) != 2 {
				continue
			}
			if allows(parts[0], parts[1]) {
				seen[p] = struct{}{}
				out = append(out, p)
			}
		}
		return out
	}

	tokenType := strings.TrimSpace(requestedType)
	if tokenType == "" {
		if len(requestedPerms) > 0 {
			tokenType = "custom"
		} else {
			tokenType = "read_only"
		}
	}

	switch kind {
	case TokenKindTransfer:
		switch tokenType {
		case "push":
			if !allows("push", "transfer") {
				return "", nil, ErrForbidden
			}
			return "push", []string{"push:transfer"}, nil
		case "pull":
			if !allows("pull", "transfer") {
				return "", nil, ErrForbidden
			}
			return "pull", []string{"pull:transfer"}, nil
		case "push_pull", "full_access":
			if !allows("push", "transfer") || !allows("pull", "transfer") {
				return "", nil, ErrForbidden
			}
			return "push_pull", []string{"push:transfer", "pull:transfer"}, nil
		case "custom":
			clean, err := validateCustomScopes(requestedPerms, allows, TokenKindTransfer)
			if err != nil {
				return "", nil, err
			}
			return "custom", clean, nil
		default:
			return "", nil, fmt.Errorf("invalid transfer token type %q", tokenType)
		}

	case TokenKindAPI:
		switch tokenType {
		case "read_only":
			perms := filterAllowed([]string{"read:content", "read:media", "read:system:locales"})
			if len(perms) == 0 {
				return "", nil, ErrForbidden
			}
			return "read_only", perms, nil
		case "full_access":
			var perms []string
			if allows("*", "*") {
				perms = []string{"*:content", "*:content_types", "*:media", "read:system:locales"}
			} else {
				perms = filterAllowed([]string{
					"read:content", "create:content", "update:content", "delete:content", "publish:content",
					"read:content_types", "create:content_types", "update:content_types", "delete:content_types",
					"read:media", "create:media", "update:media", "delete:media",
					"read:system:locales",
				})
			}
			if len(perms) == 0 {
				return "", nil, ErrForbidden
			}
			return "full_access", perms, nil
		case "custom":
			clean, err := validateCustomScopes(requestedPerms, allows, TokenKindAPI)
			if err != nil {
				return "", nil, err
			}
			return "custom", clean, nil
		default:
			return "", nil, fmt.Errorf("invalid api token type %q", tokenType)
		}

	case TokenKindAdmin:
		switch tokenType {
		case "read_only":
			perms := filterAllowed([]string{
				"read:content", "read:content_types", "read:media",
				"read:users", "read:roles", "read:api_tokens",
				"read:webhooks", "read:plugins",
				"read:system:locales", "read:system:settings", "read:system:audit", "read:system:info",
				"read:users_permissions",
			})
			if len(perms) == 0 {
				return "", nil, ErrForbidden
			}
			return "read_only", perms, nil
		case "full_access":
			if !allows("*", "*") {
				return "", nil, ErrForbidden
			}
			return "full_access", []string{"*:*"}, nil
		case "custom":
			clean, err := validateCustomScopes(requestedPerms, allows, TokenKindAdmin)
			if err != nil {
				return "", nil, err
			}
			return "custom", clean, nil
		default:
			return "", nil, fmt.Errorf("invalid admin token type %q", tokenType)
		}
	}

	return "", nil, fmt.Errorf("unsupported token kind %q", kind)
}

func validateCustomScopes(requested []string, allows func(action, resource string) bool, kind TokenKind) ([]string, error) {
	if len(requested) == 0 {
		return nil, errors.New("at least one permission scope is required")
	}
	clean := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		scope := strings.TrimSpace(raw)
		if scope == "" {
			continue
		}
		parts := strings.SplitN(scope, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid permission format %q", scope)
		}
		action, resource := parts[0], parts[1]
		if kind == TokenKindTransfer && resource != "transfer" {
			return nil, fmt.Errorf("transfer tokens may only hold transfer permissions, got %q", scope)
		}
		if kind != TokenKindTransfer && resource == "transfer" {
			return nil, fmt.Errorf("transfer permissions require a transfer token")
		}
		if !allows(action, resource) {
			return nil, fmt.Errorf("%w: cannot grant %q", ErrForbidden, scope)
		}
		if _, ok := seen[scope]; !ok {
			seen[scope] = struct{}{}
			clean = append(clean, scope)
		}
	}
	if len(clean) == 0 {
		return nil, errors.New("at least one permission scope is required")
	}
	return clean, nil
}

// ValidateAPIToken checks a raw Personal Access Token and returns the token model and user.
func (s *Service) ValidateAPIToken(ctx context.Context, rawToken string) (*APIToken, *User, error) {
	hash := HashAPIToken(rawToken)
	token, err := s.tokenRepo.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, nil, ErrUnauthorized
	}

	if token.RevokedAt != nil {
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

// UpdateProfileRequest contains self-service profile fields an authenticated admin may change.
type UpdateProfileRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// ChangePasswordRequest contains credentials required for a self-service password change.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// UpdateProfile updates the authenticated user's own email and display name safely without altering roles or status.
func (s *Service) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (*User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	nextEmail := strings.TrimSpace(strings.ToLower(req.Email))
	if nextEmail != "" && nextEmail != strings.ToLower(user.Email) {
		parsed, err := mail.ParseAddress(nextEmail)
		if err != nil || !strings.Contains(parsed.Address, "@") {
			return nil, fmt.Errorf("invalid email address format")
		}
		if existing, err := s.userRepo.GetUserByEmail(ctx, nextEmail); err == nil && existing != nil && existing.ID != user.ID {
			return nil, fmt.Errorf("email %q is already in use", nextEmail)
		}
		user.Email = nextEmail
	}
	user.FirstName = strings.TrimSpace(req.FirstName)
	user.LastName = strings.TrimSpace(req.LastName)
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return s.userRepo.GetUserByID(ctx, userID)
}

// ChangePassword verifies the current password, enforces strength policy, and stores the new hash.
func (s *Service) ChangePassword(ctx context.Context, userID string, req ChangePasswordRequest) error {
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return fmt.Errorf("current_password and new_password are required")
	}
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	ok, err := ComparePassword(req.CurrentPassword, user.PasswordHash)
	if err != nil || !ok {
		return fmt.Errorf("current password is incorrect")
	}
	if err := s.validatePasswordForPolicy(ctx, req.NewPassword); err != nil {
		return err
	}
	hash, err := HashPassword(req.NewPassword, nil)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	now := time.Now().UTC()
	user.PasswordHash = hash
	user.PasswordChangedAt = &now
	user.UpdatedAt = now
	return s.userRepo.UpdateUser(ctx, user)
}

func isValidUserStatus(s UserStatus) bool {
	return s == StatusActive || s == StatusInactive || s == StatusSuspended
}

func userHasRoleSlug(u *User, slug string) bool {
	if u == nil {
		return false
	}
	for _, r := range u.Roles {
		if r.Slug == slug {
			return true
		}
	}
	return false
}

func roleGrantsSuperAdmin(r *Role) bool {
	if r == nil {
		return false
	}
	if r.Slug == RoleSuperAdmin {
		return true
	}
	for _, p := range r.Permissions {
		if p.Action == "*" && p.Resource == "*" {
			return true
		}
	}
	return false
}

func isSuperAdminUser(u *User) bool {
	if u == nil {
		return false
	}
	for i := range u.Roles {
		if roleGrantsSuperAdmin(&u.Roles[i]) {
			return true
		}
	}
	return false
}

func (s *Service) validateAndResolveRoles(ctx context.Context, actor *User, roleIDs []string) ([]Role, error) {
	resolved := make([]Role, 0, len(roleIDs))
	seen := make(map[string]struct{}, len(roleIDs))
	actorIsSuper := actor == nil || isSuperAdminUser(actor)

	for _, rid := range roleIDs {
		rid = strings.TrimSpace(rid)
		if rid == "" {
			continue
		}
		if _, dup := seen[rid]; dup {
			continue
		}
		seen[rid] = struct{}{}
		role, err := s.roleRepo.GetRoleByID(ctx, rid)
		if err != nil {
			return nil, fmt.Errorf("invalid role %q: %w", rid, err)
		}
		if !actorIsSuper && roleGrantsSuperAdmin(role) {
			return nil, ErrForbidden
		}
		resolved = append(resolved, *role)
	}
	return resolved, nil
}

// CreateAdminUser creates an admin user with explicit initial status and roles.
func (s *Service) CreateAdminUser(ctx context.Context, actor *User, req CreateAdminUserRequest, meta AuditMeta) (*User, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Username == "" || req.Email == "" {
		return nil, errors.New("username and email are required")
	}
	parsed, err := mail.ParseAddress(req.Email)
	if err != nil || !strings.Contains(parsed.Address, "@") {
		return nil, errors.New("invalid email address format")
	}
	if err := s.validatePasswordForPolicy(ctx, req.Password); err != nil {
		return nil, err
	}

	status := req.Status
	if status == "" {
		status = StatusActive
	}
	if !isValidUserStatus(status) {
		return nil, fmt.Errorf("invalid status %q", status)
	}

	roleIDs := req.RoleIDs
	if len(roleIDs) == 0 {
		if viewer, err := s.roleRepo.GetRoleBySlug(ctx, RoleViewer); err == nil {
			roleIDs = []string{viewer.ID}
		}
	}
	if len(roleIDs) > 0 {
		if _, err := s.validateAndResolveRoles(ctx, actor, roleIDs); err != nil {
			return nil, err
		}
	}

	hash, err := HashPassword(req.Password, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC()
	user := &User{
		ID:                uuid.New().String(),
		Username:          req.Username,
		Email:             req.Email,
		PasswordHash:      hash,
		Status:            status,
		FirstName:         strings.TrimSpace(req.FirstName),
		LastName:          strings.TrimSpace(req.LastName),
		PasswordChangedAt: &now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	if len(roleIDs) > 0 {
		if err := s.roleRepo.ReplaceUserRoles(ctx, user.ID, roleIDs); err != nil {
			_ = s.userRepo.DeleteUser(ctx, user.ID)
			return nil, fmt.Errorf("failed to assign user roles: %w", err)
		}
	}

	created, err := s.userRepo.GetUserByID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, meta, "user.create", "user", user.ID, map[string]any{
		"username": user.Username,
		"email":    user.Email,
		"status":   user.Status,
		"role_ids": roleIDs,
	}, "success")
	return created, nil
}

// UpdateUser updates a user profile and role assignments (service-level call without actor scoping).
func (s *Service) UpdateUser(ctx context.Context, userID string, req UpdateUserRequest) (*User, error) {
	return s.UpdateUserByActor(ctx, nil, userID, req, AuditMeta{})
}

// UpdateUserByActor updates a user while enforcing RBAC anti-escalation and last-Super-Admin protections.
func (s *Service) UpdateUserByActor(ctx context.Context, actor *User, userID string, req UpdateUserRequest, meta AuditMeta) (*User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if actor != nil {
		if !isSuperAdminUser(actor) && isSuperAdminUser(user) {
			return nil, ErrForbidden
		}
		if actor.ID == user.ID && !HasPermission(actor, "*", "users") {
			req.RoleIDs = nil
			req.Status = ""
		}
	}

	nextEmail := strings.TrimSpace(strings.ToLower(req.Email))
	if nextEmail != "" && nextEmail != strings.ToLower(user.Email) {
		parsed, err := mail.ParseAddress(nextEmail)
		if err != nil || !strings.Contains(parsed.Address, "@") {
			return nil, errors.New("invalid email address format")
		}
		if existing, err := s.userRepo.GetUserByEmail(ctx, nextEmail); err == nil && existing != nil && existing.ID != user.ID {
			return nil, ErrUserAlreadyExists
		}
		user.Email = nextEmail
	}
	if req.FirstName != "" {
		user.FirstName = strings.TrimSpace(req.FirstName)
	}
	if req.LastName != "" {
		user.LastName = strings.TrimSpace(req.LastName)
	}

	nextStatus := user.Status
	if req.Status != "" {
		if !isValidUserStatus(req.Status) {
			return nil, fmt.Errorf("invalid status %q", req.Status)
		}
		nextStatus = req.Status
	}

	var validatedRoles []Role
	if req.RoleIDs != nil {
		if len(req.RoleIDs) == 0 {
			return nil, errors.New("user must be assigned at least one role")
		}
		validatedRoles, err = s.validateAndResolveRoles(ctx, actor, req.RoleIDs)
		if err != nil {
			return nil, err
		}
	}

	// Prevent removing or deactivating the last active Super Administrator.
	wasActiveSuper := user.Status == StatusActive && userHasRoleSlug(user, RoleSuperAdmin)
	if wasActiveSuper {
		willHaveSuper := userHasRoleSlug(user, RoleSuperAdmin)
		if req.RoleIDs != nil {
			willHaveSuper = false
			for _, r := range validatedRoles {
				if r.Slug == RoleSuperAdmin {
					willHaveSuper = true
					break
				}
			}
		}
		if nextStatus != StatusActive || !willHaveSuper {
			activeSupers, cntErr := s.roleRepo.CountActiveUsersWithRoleSlug(ctx, RoleSuperAdmin)
			if cntErr == nil && activeSupers <= 1 {
				return nil, ErrLastSuperAdmin
			}
		}
	}

	user.Status = nextStatus
	user.UpdatedAt = time.Now().UTC()
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	if req.RoleIDs != nil {
		cleanIDs := make([]string, 0, len(validatedRoles))
		for _, r := range validatedRoles {
			cleanIDs = append(cleanIDs, r.ID)
		}
		if err := s.roleRepo.ReplaceUserRoles(ctx, user.ID, cleanIDs); err != nil {
			return nil, fmt.Errorf("failed to update user roles: %w", err)
		}
	}

	updated, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, meta, "user.update", "user", user.ID, map[string]any{
		"status":   updated.Status,
		"role_ids": req.RoleIDs,
	}, "success")
	return updated, nil
}

// SetUserStatus activates or deactivates a user account with self-lockout and last-Super-Admin protections.
func (s *Service) SetUserStatus(ctx context.Context, actor *User, userID string, status UserStatus, meta AuditMeta) (*User, error) {
	if actor != nil && actor.ID == userID && status != StatusActive {
		return nil, errors.New("cannot deactivate your own account")
	}
	return s.UpdateUserByActor(ctx, actor, userID, UpdateUserRequest{Status: status}, meta)
}

// DeleteUser removes a user account (service-level call).
func (s *Service) DeleteUser(ctx context.Context, userID string) error {
	return s.DeleteUserByActor(ctx, nil, userID, AuditMeta{})
}

// DeleteUserByActor safely removes a user account with self-delete and last-Super-Admin guards.
func (s *Service) DeleteUserByActor(ctx context.Context, actor *User, userID string, meta AuditMeta) error {
	if actor != nil && actor.ID == userID {
		return ErrCannotDeleteSelf
	}
	target, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if actor != nil && !isSuperAdminUser(actor) && isSuperAdminUser(target) {
		return ErrForbidden
	}
	if target.Status == StatusActive && userHasRoleSlug(target, RoleSuperAdmin) {
		activeSupers, cntErr := s.roleRepo.CountActiveUsersWithRoleSlug(ctx, RoleSuperAdmin)
		if cntErr == nil && activeSupers <= 1 {
			return ErrLastSuperAdmin
		}
	}
	if err := s.userRepo.DeleteUser(ctx, userID); err != nil {
		return err
	}
	s.recordAudit(ctx, meta, "user.delete", "user", userID, map[string]any{
		"username": target.Username,
		"email":    target.Email,
	}, "success")
	return nil
}

// ListUsers returns a paginated list of users.
func (s *Service) ListUsers(ctx context.Context, limit, offset int) ([]*User, int, error) {
	return s.ListUsersFiltered(ctx, UserListFilter{Limit: limit, Offset: offset})
}

// ListUsersFiltered returns filtered, paginated users.
func (s *Service) ListUsersFiltered(ctx context.Context, filter UserListFilter) ([]*User, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.userRepo.ListUsersFiltered(ctx, filter)
}

// ListRoles returns all roles in the system.
func (s *Service) ListRoles(ctx context.Context) ([]*Role, error) {
	return s.roleRepo.ListRoles(ctx)
}

// GetRole returns a role with permissions and user count.
func (s *Service) GetRole(ctx context.Context, id string) (*Role, error) {
	return s.roleRepo.GetRoleByID(ctx, id)
}

// ListPermissions returns all system permissions available for RBAC assignment.
func (s *Service) ListPermissions(ctx context.Context) ([]Permission, error) {
	return s.permRepo.ListPermissions(ctx)
}

func normalizeRoleSlug(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = roleSlugSanitizer.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func (s *Service) validatePermissionIDs(ctx context.Context, permissionIDs []string) ([]string, error) {
	allPerms, err := s.permRepo.ListPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load permissions catalog: %w", err)
	}
	valid := make(map[string]struct{}, len(allPerms))
	for _, p := range allPerms {
		valid[p.ID] = struct{}{}
	}
	clean := make([]string, 0, len(permissionIDs))
	seen := make(map[string]struct{}, len(permissionIDs))
	for _, pid := range permissionIDs {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}
		if _, ok := valid[pid]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrPermissionNotFound, pid)
		}
		if _, dup := seen[pid]; dup {
			continue
		}
		seen[pid] = struct{}{}
		clean = append(clean, pid)
	}
	return clean, nil
}

// CreateRole creates a custom role with validated permissions.
func (s *Service) CreateRole(ctx context.Context, req RoleMutationRequest, meta AuditMeta) (*Role, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("role name is required")
	}
	slug := normalizeRoleSlug(req.Slug)
	if slug == "" {
		slug = normalizeRoleSlug(name)
	}
	if slug == "" {
		return nil, errors.New("role slug is required")
	}

	permIDs, err := s.validatePermissionIDs(ctx, req.PermissionIDs)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	role := &Role{
		ID:          "role_" + uuid.New().String(),
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(req.Description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.roleRepo.CreateRole(ctx, role); err != nil {
		return nil, err
	}
	if err := s.permRepo.ReplaceRolePermissions(ctx, role.ID, permIDs); err != nil {
		_ = s.roleRepo.DeleteRole(ctx, role.ID)
		return nil, fmt.Errorf("failed to assign permissions: %w", err)
	}

	created, err := s.roleRepo.GetRoleByID(ctx, role.ID)
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, meta, "role.create", "role", role.ID, map[string]any{
		"name":             created.Name,
		"slug":             created.Slug,
		"permission_count": len(permIDs),
	}, "success")
	return created, nil
}

// UpdateRole updates a role and its permissions while safeguarding super_admin and system role slugs.
func (s *Service) UpdateRole(ctx context.Context, roleID string, req RoleMutationRequest, meta AuditMeta) (*Role, error) {
	role, err := s.roleRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role.Slug == RoleSuperAdmin {
		return nil, fmt.Errorf("%w: super_admin permissions and identity are locked", ErrSystemRoleProtected)
	}

	if name := strings.TrimSpace(req.Name); name != "" {
		role.Name = name
	}
	if req.Slug != "" {
		nextSlug := normalizeRoleSlug(req.Slug)
		if nextSlug != role.Slug {
			if isSystemRoleSlug(role.Slug) {
				return nil, fmt.Errorf("%w: system role slug cannot be renamed", ErrSystemRoleProtected)
			}
			if nextSlug == "" {
				return nil, errors.New("role slug cannot be empty")
			}
			role.Slug = nextSlug
		}
	}
	if req.Description != "" || !isSystemRoleSlug(role.Slug) {
		role.Description = strings.TrimSpace(req.Description)
	}
	role.UpdatedAt = time.Now().UTC()

	var cleanPermIDs []string
	if req.PermissionIDs != nil {
		cleanPermIDs, err = s.validatePermissionIDs(ctx, req.PermissionIDs)
		if err != nil {
			return nil, err
		}
	}

	if err := s.roleRepo.UpdateRole(ctx, role); err != nil {
		return nil, err
	}
	if req.PermissionIDs != nil {
		if err := s.permRepo.ReplaceRolePermissions(ctx, role.ID, cleanPermIDs); err != nil {
			return nil, fmt.Errorf("failed to update role permissions: %w", err)
		}
	}

	updated, err := s.roleRepo.GetRoleByID(ctx, role.ID)
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, meta, "role.update", "role", role.ID, map[string]any{
		"name":             updated.Name,
		"slug":             updated.Slug,
		"permission_count": len(updated.Permissions),
	}, "success")
	return updated, nil
}

// DuplicateRole clones an existing role and its permissions under a new name and slug.
func (s *Service) DuplicateRole(ctx context.Context, roleID string, req RoleMutationRequest, meta AuditMeta) (*Role, error) {
	src, err := s.roleRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = src.Name + " Copy"
	}
	slug := normalizeRoleSlug(req.Slug)
	if slug == "" {
		slug = normalizeRoleSlug(name)
	}
	// Ensure unique slug if duplicate already exists
	if _, lookupErr := s.roleRepo.GetRoleBySlug(ctx, slug); lookupErr == nil {
		slug = slug + "_" + uuid.New().String()[:6]
	}

	permIDs := make([]string, 0, len(src.Permissions))
	for _, p := range src.Permissions {
		permIDs = append(permIDs, p.ID)
	}

	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = src.Description
	}

	return s.CreateRole(ctx, RoleMutationRequest{
		Name:          name,
		Slug:          slug,
		Description:   desc,
		PermissionIDs: permIDs,
	}, meta)
}

// DeleteRole deletes a custom role if it is not a system role and has zero assigned users.
func (s *Service) DeleteRole(ctx context.Context, roleID string, meta AuditMeta) error {
	role, err := s.roleRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if isSystemRoleSlug(role.Slug) {
		return fmt.Errorf("%w: built-in system roles cannot be deleted", ErrSystemRoleProtected)
	}
	count, err := s.roleRepo.CountUsersByRoleID(ctx, roleID)
	if err == nil && count > 0 {
		return fmt.Errorf("%w (%d assigned)", ErrRoleInUse, count)
	}
	if err := s.roleRepo.DeleteRole(ctx, roleID); err != nil {
		return err
	}
	s.recordAudit(ctx, meta, "role.delete", "role", roleID, map[string]any{
		"name": role.Name,
		"slug": role.Slug,
	}, "success")
	return nil
}

func (s *Service) hydrateInvitationRoles(ctx context.Context, inv *AdminInvitation) {
	if inv == nil || len(inv.RoleIDs) == 0 {
		return
	}
	roles := make([]Role, 0, len(inv.RoleIDs))
	for _, rid := range inv.RoleIDs {
		if r, err := s.roleRepo.GetRoleByID(ctx, rid); err == nil {
			r.Permissions = nil // lightweight summary
			roles = append(roles, *r)
		}
	}
	inv.Roles = roles
}

// CreateInvitation creates a hashed, expiring onboarding invitation and returns the raw token once.
func (s *Service) CreateInvitation(ctx context.Context, actor *User, req CreateInvitationRequest, meta AuditMeta) (string, *AdminInvitation, error) {
	if s.inviteRepo == nil {
		return "", nil, errors.New("invitation repository is not configured")
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		return "", nil, errors.New("email is required")
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || !strings.Contains(parsed.Address, "@") {
		return "", nil, errors.New("invalid email address format")
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = strings.Split(email, "@")[0]
	}

	if existing, err := s.userRepo.GetUserByEmail(ctx, email); err == nil && existing != nil {
		return "", nil, ErrUserAlreadyExists
	}
	if existing, err := s.userRepo.GetUserByUsername(ctx, username); err == nil && existing != nil {
		return "", nil, ErrUserAlreadyExists
	}

	roleIDs := req.RoleIDs
	if len(roleIDs) == 0 {
		if viewer, err := s.roleRepo.GetRoleBySlug(ctx, RoleViewer); err == nil {
			roleIDs = []string{viewer.ID}
		}
	}
	if len(roleIDs) > 0 {
		if _, err := s.validateAndResolveRoles(ctx, actor, roleIDs); err != nil {
			return "", nil, err
		}
	}

	hours := req.ExpiresInHours
	if hours <= 0 {
		hours = 72
	}
	if hours > 720 {
		hours = 720
	}

	rawToken, tokenHash, err := GenerateInviteToken()
	if err != nil {
		return "", nil, err
	}

	now := time.Now().UTC()
	invitedBy := ""
	if actor != nil {
		invitedBy = actor.ID
	}
	inv := &AdminInvitation{
		ID:        uuid.New().String(),
		Email:     email,
		Username:  username,
		FirstName: strings.TrimSpace(req.FirstName),
		LastName:  strings.TrimSpace(req.LastName),
		RoleIDs:   roleIDs,
		TokenHash: tokenHash,
		Status:    InvitationPending,
		InvitedBy: invitedBy,
		ExpiresAt: now.Add(time.Duration(hours) * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.inviteRepo.CreateInvitation(ctx, inv); err != nil {
		return "", nil, err
	}
	s.hydrateInvitationRoles(ctx, inv)
	if s.inviteNotifier != nil {
		if mailErr := s.inviteNotifier(ctx, inv, rawToken, actor, hours); mailErr != nil {
			s.logger.Warn("Failed to deliver admin invitation email", "email", inv.Email, "error", mailErr)
			sent := false
			inv.EmailSent = &sent
		} else {
			sent := true
			inv.EmailSent = &sent
		}
	}
	s.recordAudit(ctx, meta, "user.invite", "invitation", inv.ID, map[string]any{
		"email":    inv.Email,
		"username": inv.Username,
		"role_ids": inv.RoleIDs,
	}, "success")
	return rawToken, inv, nil
}

// ListInvitations lists invitations sorted newest-first with effective expiration state.
func (s *Service) ListInvitations(ctx context.Context) ([]*AdminInvitation, error) {
	if s.inviteRepo == nil {
		return []*AdminInvitation{}, nil
	}
	list, err := s.inviteRepo.ListInvitations(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, inv := range list {
		if inv.Status == InvitationPending && now.After(inv.ExpiresAt) {
			inv.Status = InvitationExpired
		}
		s.hydrateInvitationRoles(ctx, inv)
	}
	return list, nil
}

// ResendInvitation rotates the invitation token and extends expiration.
func (s *Service) ResendInvitation(ctx context.Context, actor *User, id string, expiresInHours int, meta AuditMeta) (string, *AdminInvitation, error) {
	if s.inviteRepo == nil {
		return "", nil, errors.New("invitation repository is not configured")
	}
	inv, err := s.inviteRepo.GetInvitationByID(ctx, id)
	if err != nil {
		return "", nil, err
	}
	if inv.Status == InvitationAccepted {
		return "", nil, ErrInvitationRevoked
	}
	if expiresInHours <= 0 {
		expiresInHours = 72
	}
	if expiresInHours > 720 {
		expiresInHours = 720
	}

	rawToken, tokenHash, err := GenerateInviteToken()
	if err != nil {
		return "", nil, err
	}
	now := time.Now().UTC()
	inv.TokenHash = tokenHash
	inv.Status = InvitationPending
	inv.ExpiresAt = now.Add(time.Duration(expiresInHours) * time.Hour)
	inv.UpdatedAt = now
	if actor != nil {
		inv.InvitedBy = actor.ID
	}
	if err := s.inviteRepo.UpdateInvitation(ctx, inv); err != nil {
		return "", nil, err
	}
	s.hydrateInvitationRoles(ctx, inv)
	if s.inviteNotifier != nil {
		if mailErr := s.inviteNotifier(ctx, inv, rawToken, actor, expiresInHours); mailErr != nil {
			s.logger.Warn("Failed to re-deliver admin invitation email", "email", inv.Email, "error", mailErr)
			sent := false
			inv.EmailSent = &sent
		} else {
			sent := true
			inv.EmailSent = &sent
		}
	}
	s.recordAudit(ctx, meta, "user.invite.resend", "invitation", inv.ID, map[string]any{
		"email": inv.Email,
	}, "success")
	return rawToken, inv, nil
}

// RevokeInvitation revokes a pending invitation so its token can no longer be accepted.
func (s *Service) RevokeInvitation(ctx context.Context, id string, meta AuditMeta) (*AdminInvitation, error) {
	if s.inviteRepo == nil {
		return nil, errors.New("invitation repository is not configured")
	}
	inv, err := s.inviteRepo.GetInvitationByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if inv.Status == InvitationAccepted {
		return nil, ErrInvitationRevoked
	}
	inv.Status = InvitationRevoked
	inv.UpdatedAt = time.Now().UTC()
	if err := s.inviteRepo.UpdateInvitation(ctx, inv); err != nil {
		return nil, err
	}
	s.hydrateInvitationRoles(ctx, inv)
	s.recordAudit(ctx, meta, "user.invite.revoke", "invitation", inv.ID, map[string]any{
		"email": inv.Email,
	}, "success")
	return inv, nil
}

// DeleteInvitation permanently removes an invitation record.
func (s *Service) DeleteInvitation(ctx context.Context, id string, meta AuditMeta) error {
	if s.inviteRepo == nil {
		return errors.New("invitation repository is not configured")
	}
	if err := s.inviteRepo.DeleteInvitation(ctx, id); err != nil {
		return err
	}
	s.recordAudit(ctx, meta, "user.invite.delete", "invitation", id, nil, "success")
	return nil
}

// PreviewInvitation validates an invitation token without consuming it.
func (s *Service) PreviewInvitation(ctx context.Context, rawToken string) (*AdminInvitation, error) {
	if s.inviteRepo == nil {
		return nil, errors.New("invitation repository is not configured")
	}
	if strings.TrimSpace(rawToken) == "" {
		return nil, ErrInvitationNotFound
	}
	inv, err := s.inviteRepo.GetInvitationByTokenHash(ctx, HashOpaqueToken(rawToken))
	if err != nil {
		return nil, err
	}
	if inv.Status != InvitationPending {
		return nil, ErrInvitationRevoked
	}
	if time.Now().UTC().After(inv.ExpiresAt) {
		return nil, ErrInvitationExpired
	}
	s.hydrateInvitationRoles(ctx, inv)
	return inv, nil
}

// AcceptInvitation validates an invitation token, creates the active admin account with roles, and signs them in.
func (s *Service) AcceptInvitation(ctx context.Context, req AcceptInvitationRequest, meta AuditMeta) (*User, string, error) {
	if s.inviteRepo == nil {
		return nil, "", errors.New("invitation repository is not configured")
	}
	if strings.TrimSpace(req.Token) == "" {
		return nil, "", ErrInvitationNotFound
	}
	inv, err := s.inviteRepo.GetInvitationByTokenHash(ctx, HashOpaqueToken(req.Token))
	if err != nil {
		return nil, "", err
	}
	if inv.Status != InvitationPending {
		return nil, "", ErrInvitationRevoked
	}
	now := time.Now().UTC()
	if now.After(inv.ExpiresAt) {
		inv.Status = InvitationExpired
		inv.UpdatedAt = now
		_ = s.inviteRepo.UpdateInvitation(ctx, inv)
		return nil, "", ErrInvitationExpired
	}

	if err := s.validatePasswordForPolicy(ctx, req.Password); err != nil {
		return nil, "", err
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = inv.Username
	}
	firstName := strings.TrimSpace(req.FirstName)
	if firstName == "" {
		firstName = inv.FirstName
	}
	lastName := strings.TrimSpace(req.LastName)
	if lastName == "" {
		lastName = inv.LastName
	}

	hash, err := HashPassword(req.Password, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to hash password: %w", err)
	}

	user := &User{
		ID:                uuid.New().String(),
		Username:          username,
		Email:             inv.Email,
		PasswordHash:      hash,
		Status:            StatusActive,
		FirstName:         firstName,
		LastName:          lastName,
		LastLoginAt:       &now,
		PasswordChangedAt: &now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, "", err
	}
	if len(inv.RoleIDs) > 0 {
		if err := s.roleRepo.ReplaceUserRoles(ctx, user.ID, inv.RoleIDs); err != nil {
			_ = s.userRepo.DeleteUser(ctx, user.ID)
			return nil, "", fmt.Errorf("failed to assign invited roles: %w", err)
		}
	}

	inv.Status = InvitationAccepted
	inv.AcceptedAt = &now
	inv.UpdatedAt = now
	if err := s.inviteRepo.UpdateInvitation(ctx, inv); err != nil {
		s.logger.Warn("failed to mark invitation accepted", "invitation_id", inv.ID, "error", err)
	}

	fullUser, err := s.userRepo.GetUserByID(ctx, user.ID)
	if err != nil {
		return nil, "", err
	}
	jwtToken, err := GenerateAccessToken(fullUser, s.cfg.Auth.JWTSecret, s.cfg.Auth.TokenExpiration)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate access token: %w", err)
	}
	s.recordAudit(ctx, meta, "user.invite.accept", "user", fullUser.ID, map[string]any{
		"invitation_id": inv.ID,
		"email":         fullUser.Email,
	}, "success")
	return fullUser, jwtToken, nil
}

// ListAPITokens returns active API tokens for a user.
func (s *Service) ListAPITokens(ctx context.Context, userID string) ([]*APIToken, error) {
	return s.tokenRepo.ListTokensByUserID(ctx, userID)
}

// ListTokensByKind lists API, Admin, or Transfer tokens visible to the actor.
func (s *Service) ListTokensByKind(ctx context.Context, actor *User, kind TokenKind) ([]*APIToken, error) {
	if actor == nil {
		return nil, ErrUnauthorized
	}
	freshActor, err := s.userRepo.GetUserByID(ctx, actor.ID)
	if err != nil || freshActor.Status != StatusActive {
		return nil, ErrUnauthorized
	}
	if !HasPermission(freshActor, "read", "api_tokens") {
		return nil, ErrForbidden
	}
	if kind != "" && kind != TokenKindAPI && kind != TokenKindAdmin && kind != TokenKindTransfer {
		return nil, fmt.Errorf("unsupported token kind %q", kind)
	}
	includeAll := userHasRoleSlug(freshActor, RoleSuperAdmin) || HasPermission(freshActor, "*", "api_tokens")
	return s.tokenRepo.ListTokensByKind(ctx, freshActor.ID, kind, includeAll)
}

func (s *Service) resolveActorTokenForMutation(ctx context.Context, actor *User, tokenID, requiredAction string) (*User, *APIToken, error) {
	if actor == nil {
		return nil, nil, ErrUnauthorized
	}
	freshActor, err := s.userRepo.GetUserByID(ctx, actor.ID)
	if err != nil || freshActor.Status != StatusActive {
		return nil, nil, ErrUnauthorized
	}
	if !HasPermission(freshActor, requiredAction, "api_tokens") {
		return nil, nil, ErrForbidden
	}
	tok, err := s.tokenRepo.GetByID(ctx, tokenID)
	if err != nil {
		return nil, nil, err
	}
	if tok.UserID != freshActor.ID && !userHasRoleSlug(freshActor, RoleSuperAdmin) && !HasPermission(freshActor, "*", "api_tokens") {
		return nil, nil, ErrForbidden
	}
	return freshActor, tok, nil
}

// RotateAPIToken generates a replacement secret for an existing token and returns the new plaintext once.
func (s *Service) RotateAPIToken(ctx context.Context, actor *User, tokenID string, meta AuditMeta) (string, *APIToken, error) {
	freshActor, tok, err := s.resolveActorTokenForMutation(ctx, actor, tokenID, "create")
	if err != nil {
		return "", nil, err
	}
	if tok.RevokedAt != nil {
		return "", nil, errors.New("cannot rotate a revoked token")
	}

	kind := tok.Kind
	if kind == "" {
		kind = TokenKindAPI
	}
	callerToken := GetAPITokenFromContext(ctx)
	for _, perm := range tok.Permissions {
		parts := strings.SplitN(perm, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if !HasPermission(freshActor, parts[0], parts[1]) {
			return "", nil, ErrForbidden
		}
		if callerToken != nil && !TokenAllowsPermission(callerToken, parts[0], parts[1]) {
			return "", nil, ErrForbidden
		}
	}

	rawToken, prefix, hash, err := GenerateTokenForKind(kind)
	if err != nil {
		return "", nil, err
	}
	tok.Kind = kind
	tok.TokenPrefix = prefix
	tok.TokenHash = hash
	tok.LastUsedAt = nil

	if err := s.tokenRepo.UpdateToken(ctx, tok); err != nil {
		return "", nil, err
	}

	if meta.ActorID == "" {
		meta.ActorID = freshActor.ID
	}
	if meta.ActorEmail == "" {
		meta.ActorEmail = freshActor.Email
	}
	s.recordAudit(ctx, meta, "token.rotate", "api_token", tok.ID, map[string]any{
		"name":   tok.Name,
		"kind":   string(tok.Kind),
		"prefix": tok.TokenPrefix,
	}, "success")

	return rawToken, tok, nil
}

// RevokeAPITokenByActor marks a token as revoked so it is retained for history but unusable.
func (s *Service) RevokeAPITokenByActor(ctx context.Context, actor *User, tokenID string, meta AuditMeta) error {
	freshActor, tok, err := s.resolveActorTokenForMutation(ctx, actor, tokenID, "delete")
	if err != nil {
		return err
	}
	if tok.RevokedAt != nil {
		return nil
	}
	if err := s.tokenRepo.RevokeToken(ctx, tok.ID, time.Now().UTC()); err != nil {
		return err
	}
	if meta.ActorID == "" {
		meta.ActorID = freshActor.ID
	}
	if meta.ActorEmail == "" {
		meta.ActorEmail = freshActor.Email
	}
	s.recordAudit(ctx, meta, "token.revoke", "api_token", tok.ID, map[string]any{
		"name":   tok.Name,
		"kind":   string(tok.Kind),
		"prefix": tok.TokenPrefix,
	}, "success")
	return nil
}

// DeleteAPITokenByActor permanently deletes an API, Admin, or Transfer token.
func (s *Service) DeleteAPITokenByActor(ctx context.Context, actor *User, tokenID string, meta AuditMeta) error {
	freshActor, tok, err := s.resolveActorTokenForMutation(ctx, actor, tokenID, "delete")
	if err != nil {
		return err
	}
	if err := s.tokenRepo.DeleteToken(ctx, tok.ID, ""); err != nil {
		return err
	}
	if meta.ActorID == "" {
		meta.ActorID = freshActor.ID
	}
	if meta.ActorEmail == "" {
		meta.ActorEmail = freshActor.Email
	}
	s.recordAudit(ctx, meta, "token.delete", "api_token", tok.ID, map[string]any{
		"name":   tok.Name,
		"kind":   string(tok.Kind),
		"prefix": tok.TokenPrefix,
	}, "success")
	return nil
}

// RevokeAPIToken deletes an API token.
func (s *Service) RevokeAPIToken(ctx context.Context, tokenID, userID string) error {
	return s.tokenRepo.DeleteToken(ctx, tokenID, userID)
}

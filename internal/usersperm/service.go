package usersperm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/audit"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/settings"
	"github.com/google/uuid"
)

var (
	usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,64}$`)
	slugCleaner     = regexp.MustCompile(`[^a-z0-9_]+`)
)

// EmailSender sends transactional emails using an email template key.
type EmailSender func(ctx context.Context, templateKey string, to string, vars map[string]string) error

// AdminSSOLoginFunc delegates verified Enterprise SSO identity login/provisioning into the admin auth module.
type AdminSSOLoginFunc func(ctx context.Context, req auth.SSOLoginRequest) (*auth.LoginResult, error)

// OAuthAuthorizeResult holds the authorization redirect URL and CSRF state token.
type OAuthAuthorizeResult struct {
	AuthorizationURL string `json:"authorization_url"`
	State            string `json:"state"`
}

// AppAuthResult contains a newly issued App JWT alongside sanitized AppUser info.
type AppAuthResult struct {
	Token                string   `json:"token,omitempty"`
	User                 *AppUser `json:"user"`
	RequiresConfirmation bool     `json:"requires_confirmation,omitempty"`
	RedirectAfter        string   `json:"redirect_after,omitempty"`
}

// OAuthCallbackResult unifies app or admin login results returned by OAuth/SSO callback handling.
type OAuthCallbackResult struct {
	Scope         string            `json:"scope"`
	AppAuth       *AppAuthResult    `json:"app_auth,omitempty"`
	AdminAuth     *auth.LoginResult `json:"admin_auth,omitempty"`
	RedirectAfter string            `json:"redirect_after,omitempty"`
}

// Service coordinates strictly isolated end-user roles, application users, app auth flows, and OAuth/SSO providers.
type Service struct {
	repo                Repository
	settingsSvc         *settings.Service
	auditSvc            *audit.Service
	cfg                 *config.Config
	logger              *slog.Logger
	emailSendFn         EmailSender
	adminSSOFn          AdminSSOLoginFunc
	httpClient          *http.Client
	allowUnsafeTestURLs bool
}

// NewService constructs a usersperm Service.
func NewService(
	repo Repository,
	settingsSvc *settings.Service,
	auditSvc *audit.Service,
	cfg *config.Config,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:        repo,
		settingsSvc: settingsSvc,
		auditSvc:    auditSvc,
		cfg:         cfg,
		logger:      logger,
		httpClient:  newSSRFSafeHTTPClient(),
	}
}

// SetEmailSender attaches transactional email template dispatch for confirmation & reset links.
func (s *Service) SetEmailSender(fn EmailSender) {
	s.emailSendFn = fn
}

// SetAdminSSOLoginFunc attaches admin authentication/provisioning for Enterprise SSO logins.
func (s *Service) SetAdminSSOLoginFunc(fn AdminSSOLoginFunc) {
	s.adminSSOFn = fn
}

// SetHTTPClient overrides the HTTP client (primarily for local httptest servers in unit tests).
func (s *Service) SetHTTPClient(client *http.Client, allowLocalURLs bool) {
	if client != nil {
		s.httpClient = client
	}
	s.allowUnsafeTestURLs = allowLocalURLs
}

func (s *Service) jwtSecret() string {
	if s.cfg != nil && strings.TrimSpace(s.cfg.Auth.JWTSecret) != "" {
		return s.cfg.Auth.JWTSecret
	}
	return "default-insecure-jwt-secret-change-me"
}

func (s *Service) encryptionKey() string {
	if s.settingsSvc != nil {
		return s.settingsSvc.EncryptionKey()
	}
	return s.jwtSecret()
}

func (s *Service) recordAudit(ctx context.Context, meta settings.AuditMeta, action, resource, targetID string, metadata map[string]any) {
	if s.auditSvc == nil {
		return
	}
	actorID := meta.ActorID
	actorType := meta.ActorType
	if actorType == "" {
		actorType = "user"
	}
	if actorID == "" {
		actorID = "system"
		actorType = "system"
	}
	_, _ = s.auditSvc.Record(ctx, audit.RecordParams{
		ActorID:   actorID,
		ActorType: actorType,
		Action:    action,
		Resource:  resource,
		TargetID:  targetID,
		Metadata:  metadata,
		IPAddress: meta.IPAddress,
		UserAgent: meta.UserAgent,
	})
}

func (s *Service) siteInfo(ctx context.Context) (siteName, publicURL string) {
	siteName = "Go CMS"
	publicURL = "http://localhost:8080"
	if s.settingsSvc != nil {
		if gen, err := s.settingsSvc.GetGeneral(ctx); err == nil && gen != nil {
			if strings.TrimSpace(gen.SiteName) != "" {
				siteName = gen.SiteName
			}
			if strings.TrimSpace(gen.PublicURL) != "" {
				publicURL = strings.TrimRight(gen.PublicURL, "/")
			}
		}
	}
	return siteName, publicURL
}

// SeedDefaults creates built-in Public & Authenticated app roles and default disabled OAuth/SSO templates if absent.
func (s *Service) SeedDefaults(ctx context.Context) error {
	now := time.Now().UTC()

	defaultRoles := []AppRole{
		{
			ID:          uuid.New().String(),
			Name:        "Authenticated",
			Slug:        "authenticated",
			Description: "Default role granted to signed-in end-users.",
			Type:        "authenticated",
			IsDefault:   true,
			Permissions: map[string]map[string]bool{
				"content":   {"find": true, "findOne": true, "create": true, "update": true},
				"media":     {"find": true, "findOne": true, "upload": true},
				"app-users": {"me": true},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          uuid.New().String(),
			Name:        "Public",
			Slug:        "public",
			Description: "Default role defining unauthenticated public API access.",
			Type:        "public",
			IsDefault:   false,
			Permissions: map[string]map[string]bool{
				"content": {"find": true, "findOne": true},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	for _, role := range defaultRoles {
		if _, err := s.repo.GetRoleBySlug(ctx, role.Slug); errors.Is(err, ErrAppRoleNotFound) {
			r := role
			if err := s.repo.CreateRole(ctx, &r); err != nil {
				return fmt.Errorf("failed to seed app role %s: %w", role.Slug, err)
			}
		}
	}

	defaultProviders := []OAuthProvider{
		{
			ID:           uuid.New().String(),
			Name:         "google",
			DisplayName:  "Google",
			ProviderType: "google",
			Scope:        "app",
			Enabled:      false,
			AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:     "https://oauth2.googleapis.com/token",
			UserinfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
			IssuerURL:    "https://accounts.google.com",
			Scopes:       "openid email profile",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           uuid.New().String(),
			Name:         "github",
			DisplayName:  "GitHub",
			ProviderType: "github",
			Scope:        "app",
			Enabled:      false,
			AuthURL:      "https://github.com/login/oauth/authorize",
			TokenURL:     "https://github.com/login/oauth/access_token",
			UserinfoURL:  "https://api.github.com/user",
			Scopes:       "read:user user:email",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           uuid.New().String(),
			Name:         "discord",
			DisplayName:  "Discord",
			ProviderType: "discord",
			Scope:        "app",
			Enabled:      false,
			AuthURL:      "https://discord.com/api/oauth2/authorize",
			TokenURL:     "https://discord.com/api/oauth2/token",
			UserinfoURL:  "https://discord.com/api/users/@me",
			Scopes:       "identify email",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           uuid.New().String(),
			Name:         "oidc",
			DisplayName:  "OpenID Connect",
			ProviderType: "oidc",
			Scope:        "app",
			Enabled:      false,
			Scopes:       "openid email profile",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           uuid.New().String(),
			Name:         "admin_oidc",
			DisplayName:  "Enterprise OIDC SSO",
			ProviderType: "oidc",
			Scope:        "admin",
			Enabled:      false,
			Scopes:       "openid email profile",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}

	for _, prov := range defaultProviders {
		if _, err := s.repo.GetProviderByName(ctx, prov.Name); errors.Is(err, ErrProviderNotFound) {
			p := prov
			if err := s.repo.UpsertProvider(ctx, &p); err != nil {
				return fmt.Errorf("failed to seed oauth provider %s: %w", prov.Name, err)
			}
		}
	}
	return nil
}

// --- Settings ---

// GetSettings loads AppAuthSettings persisted under settings.KeyAppAuth or returns defaults.
func (s *Service) GetSettings(ctx context.Context) (*AppAuthSettings, error) {
	def := DefaultAppAuthSettings()
	if s.settingsSvc != nil {
		var loaded AppAuthSettings
		ts, found, err := s.settingsSvc.LoadJSONSetting(ctx, settings.KeyAppAuth, &loaded)
		if err != nil {
			return nil, err
		}
		if found {
			_ = loaded.NormalizeAndValidate()
			loaded.UpdatedAt = ts
			if loaded.DefaultRoleID == "" {
				if dr, rErr := s.repo.GetDefaultRole(ctx); rErr == nil {
					loaded.DefaultRoleID = dr.ID
				}
			}
			return &loaded, nil
		}
	}
	if dr, rErr := s.repo.GetDefaultRole(ctx); rErr == nil {
		def.DefaultRoleID = dr.ID
	}
	return &def, nil
}

// UpdateSettings validates and persists AppAuthSettings.
func (s *Service) UpdateSettings(ctx context.Context, input AppAuthSettings, meta settings.AuditMeta) (*AppAuthSettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	if input.DefaultRoleID != "" {
		if _, err := s.repo.GetRoleByID(ctx, input.DefaultRoleID); err != nil {
			return nil, err
		}
	}
	if s.settingsSvc != nil {
		ts, err := s.settingsSvc.SaveJSONSetting(ctx, settings.KeyAppAuth, input, meta, "settings:users_permissions", map[string]any{
			"allow_register":             input.AllowRegister,
			"email_confirmation":         input.EmailConfirmation,
			"default_role_id":            input.DefaultRoleID,
			"session_expiration_minutes": input.SessionExpirationMinutes,
		})
		if err != nil {
			return nil, err
		}
		input.UpdatedAt = ts
	}
	return &input, nil
}

func (s *Service) validateAppPassword(ctx context.Context, password string) error {
	if s.settingsSvc != nil {
		if pp, err := s.settingsSvc.GetPasswordPolicySettings(ctx); err == nil && pp != nil {
			return auth.ValidatePasswordWithPolicy(password, auth.PasswordPolicy{
				MinLength:        pp.MinLength,
				RequireUppercase: pp.RequireUppercase,
				RequireLowercase: pp.RequireLowercase,
				RequireNumber:    pp.RequireNumber,
				RequireSpecial:   pp.RequireSpecial,
				MaxAgeDays:       pp.MaxAgeDays,
			})
		}
	}
	return auth.ValidatePasswordStrength(password)
}

func generateOpaqueHexToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(buf)
	hash = auth.HashOpaqueToken(raw)
	return raw, hash, nil
}

// --- Public App Authentication ---

// Register creates a new isolated application user when registration is enabled.
func (s *Service) Register(ctx context.Context, req RegisterAppUserRequest) (*AppAuthResult, error) {
	cfg, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.AllowRegister {
		return nil, ErrRegistrationDisabled
	}

	username := strings.TrimSpace(req.Username)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !usernamePattern.MatchString(username) {
		return nil, errors.New("username must be 3-64 alphanumeric characters (._- allowed)")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, errors.New("invalid email address")
	}
	if err := s.validateAppPassword(ctx, req.Password); err != nil {
		return nil, err
	}

	pwHash, err := auth.HashPassword(req.Password, nil)
	if err != nil {
		return nil, err
	}

	roleID := cfg.DefaultRoleID
	if roleID == "" {
		if dr, rErr := s.repo.GetDefaultRole(ctx); rErr == nil {
			roleID = dr.ID
		}
	}

	now := time.Now().UTC()
	user := &AppUser{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		PasswordHash: pwHash,
		Provider:     "local",
		Confirmed:    !cfg.EmailConfirmation,
		Blocked:      false,
		RoleID:       roleID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	var confirmToken string
	if cfg.EmailConfirmation {
		rawTok, tokHash, tokErr := generateOpaqueHexToken()
		if tokErr != nil {
			return nil, tokErr
		}
		confirmToken = rawTok
		exp := now.Add(24 * time.Hour)
		user.ConfirmationTokenHash = tokHash
		user.ConfirmationExpiresAt = &exp
	} else {
		user.LastLoginAt = &now
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	hydrated, err := s.repo.GetUserByID(ctx, user.ID)
	if err != nil {
		hydrated = user
	}

	siteName, publicURL := s.siteInfo(ctx)
	if cfg.EmailConfirmation {
		if s.emailSendFn != nil {
			confirmBase := publicURL + "/confirm-email"
			if cfg.EmailConfirmationRedirectionURL != "" {
				confirmBase = cfg.EmailConfirmationRedirectionURL
			}
			confirmURL := confirmBase + "?token=" + url.QueryEscape(confirmToken)
			if err := s.emailSendFn(ctx, "email_confirmation", hydrated.Email, map[string]string{
				"site_name":   siteName,
				"username":    hydrated.Username,
				"confirm_url": confirmURL,
			}); err != nil {
				s.logger.Warn("failed to dispatch app confirmation email", "email", hydrated.Email, "error", err)
			}
		}
		return &AppAuthResult{
			User:                 hydrated,
			RequiresConfirmation: true,
		}, nil
	}

	if s.emailSendFn != nil {
		_ = s.emailSendFn(ctx, "welcome", hydrated.Email, map[string]string{
			"site_name": siteName,
			"username":  hydrated.Username,
			"login_url": publicURL,
		})
	}

	tok, err := GenerateAppAccessToken(hydrated, s.jwtSecret(), time.Duration(cfg.SessionExpirationMinutes)*time.Minute)
	if err != nil {
		return nil, err
	}
	return &AppAuthResult{Token: tok, User: hydrated}, nil
}

// Login authenticates an application user strictly against cms_app_users and issues an App JWT.
func (s *Service) Login(ctx context.Context, req LoginAppUserRequest) (*AppAuthResult, error) {
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" {
		identifier = strings.TrimSpace(req.Email)
	}
	if identifier == "" {
		identifier = strings.TrimSpace(req.Username)
	}
	if identifier == "" || req.Password == "" {
		return nil, ErrInvalidAppCredentials
	}

	var user *AppUser
	var err error
	if strings.Contains(identifier, "@") {
		user, err = s.repo.GetUserByEmail(ctx, strings.ToLower(identifier))
	} else {
		user, err = s.repo.GetUserByUsername(ctx, identifier)
	}
	if err != nil {
		return nil, ErrInvalidAppCredentials
	}

	ok, cmpErr := auth.ComparePassword(req.Password, user.PasswordHash)
	if cmpErr != nil || !ok {
		return nil, ErrInvalidAppCredentials
	}
	if user.Blocked {
		return nil, ErrAppAccountBlocked
	}

	cfg, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.EmailConfirmation && !user.Confirmed {
		return nil, ErrAppAccountUnconfirmed
	}

	now := time.Now().UTC()
	user.LastLoginAt = &now
	user.UpdatedAt = now
	_ = s.repo.UpdateUser(ctx, user)

	hydrated, err := s.repo.GetUserByID(ctx, user.ID)
	if err != nil {
		hydrated = user
	}
	tok, err := GenerateAppAccessToken(hydrated, s.jwtSecret(), time.Duration(cfg.SessionExpirationMinutes)*time.Minute)
	if err != nil {
		return nil, err
	}
	return &AppAuthResult{Token: tok, User: hydrated}, nil
}

// ConfirmEmail validates an email confirmation token, activates the account, and returns an App JWT.
func (s *Service) ConfirmEmail(ctx context.Context, rawToken string) (*AppAuthResult, error) {
	trimmed := strings.TrimSpace(rawToken)
	if trimmed == "" {
		return nil, ErrInvalidTokenOrExpired
	}
	user, err := s.repo.GetUserByConfirmationTokenHash(ctx, auth.HashOpaqueToken(trimmed))
	if err != nil {
		return nil, ErrInvalidTokenOrExpired
	}
	now := time.Now().UTC()
	if user.ConfirmationExpiresAt != nil && now.After(*user.ConfirmationExpiresAt) {
		return nil, ErrInvalidTokenOrExpired
	}
	if user.Blocked {
		return nil, ErrAppAccountBlocked
	}

	user.Confirmed = true
	user.ConfirmationTokenHash = ""
	user.ConfirmationExpiresAt = nil
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	hydrated, err := s.repo.GetUserByID(ctx, user.ID)
	if err != nil {
		hydrated = user
	}
	cfg, _ := s.GetSettings(ctx)
	ttl := 1440 * time.Minute
	if cfg != nil && cfg.SessionExpirationMinutes > 0 {
		ttl = time.Duration(cfg.SessionExpirationMinutes) * time.Minute
	}
	tok, err := GenerateAppAccessToken(hydrated, s.jwtSecret(), ttl)
	if err != nil {
		return nil, err
	}
	return &AppAuthResult{Token: tok, User: hydrated}, nil
}

// SendConfirmation generates and dispatches a fresh email confirmation token if an unconfirmed user exists.
func (s *Service) SendConfirmation(ctx context.Context, email string) error {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	if cleanEmail == "" {
		return errors.New("email is required")
	}
	user, err := s.repo.GetUserByEmail(ctx, cleanEmail)
	if err != nil || user.Confirmed || user.Blocked {
		// Anti-enumeration: always report acceptance cleanly
		return nil
	}
	rawTok, tokHash, err := generateOpaqueHexToken()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	exp := now.Add(24 * time.Hour)
	user.ConfirmationTokenHash = tokHash
	user.ConfirmationExpiresAt = &exp
	user.UpdatedAt = now
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}

	if s.emailSendFn != nil {
		cfg, _ := s.GetSettings(ctx)
		siteName, publicURL := s.siteInfo(ctx)
		confirmBase := publicURL + "/confirm-email"
		if cfg != nil && cfg.EmailConfirmationRedirectionURL != "" {
			confirmBase = cfg.EmailConfirmationRedirectionURL
		}
		_ = s.emailSendFn(ctx, "email_confirmation", user.Email, map[string]string{
			"site_name":   siteName,
			"username":    user.Username,
			"confirm_url": confirmBase + "?token=" + url.QueryEscape(rawTok),
		})
	}
	return nil
}

// ForgotPassword generates a hashed 30-minute reset token and sends the password_reset email template.
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	if cleanEmail == "" {
		return errors.New("email is required")
	}
	user, err := s.repo.GetUserByEmail(ctx, cleanEmail)
	if err != nil || user.Blocked {
		return nil
	}
	rawTok, tokHash, err := generateOpaqueHexToken()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	exp := now.Add(30 * time.Minute)
	user.ResetTokenHash = tokHash
	user.ResetExpiresAt = &exp
	user.UpdatedAt = now
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}

	if s.emailSendFn != nil {
		cfg, _ := s.GetSettings(ctx)
		siteName, publicURL := s.siteInfo(ctx)
		resetBase := publicURL + "/reset-password"
		if cfg != nil && cfg.ResetPasswordURL != "" {
			resetBase = cfg.ResetPasswordURL
		}
		_ = s.emailSendFn(ctx, "password_reset", user.Email, map[string]string{
			"site_name":       siteName,
			"username":        user.Username,
			"reset_url":       resetBase + "?token=" + url.QueryEscape(rawTok),
			"expires_minutes": "30",
		})
	}
	return nil
}

// ResetPassword validates the opaque reset token and sets a new Argon2id password hash.
func (s *Service) ResetPassword(ctx context.Context, req ResetPasswordRequest) error {
	trimmed := strings.TrimSpace(req.Token)
	if trimmed == "" {
		return ErrInvalidTokenOrExpired
	}
	if err := s.validateAppPassword(ctx, req.NewPassword); err != nil {
		return err
	}
	user, err := s.repo.GetUserByResetTokenHash(ctx, auth.HashOpaqueToken(trimmed))
	if err != nil {
		return ErrInvalidTokenOrExpired
	}
	now := time.Now().UTC()
	if user.ResetExpiresAt == nil || now.After(*user.ResetExpiresAt) {
		return ErrInvalidTokenOrExpired
	}
	hash, err := auth.HashPassword(req.NewPassword, nil)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	user.Confirmed = true
	user.ResetTokenHash = ""
	user.ResetExpiresAt = nil
	user.UpdatedAt = now
	return s.repo.UpdateUser(ctx, user)
}

// GetMe fetches the currently authenticated application user by ID.
func (s *Service) GetMe(ctx context.Context, userID string) (*AppUser, error) {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.Blocked {
		return nil, ErrAppAccountBlocked
	}
	return u, nil
}

// --- Admin Management of Roles, Users & Providers ---

func (s *Service) ListRoles(ctx context.Context) ([]AppRole, error) {
	return s.repo.ListRoles(ctx)
}

func (s *Service) GetRole(ctx context.Context, id string) (*AppRole, error) {
	return s.repo.GetRoleByID(ctx, id)
}

func (s *Service) CreateRole(ctx context.Context, req CreateAppRoleRequest, meta settings.AuditMeta) (*AppRole, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("role name is required")
	}
	slug := strings.TrimSpace(strings.ToLower(req.Slug))
	if slug == "" {
		slug = slugCleaner.ReplaceAllString(strings.ToLower(name), "_")
		slug = strings.Trim(slug, "_")
	}
	if slug == "" {
		return nil, errors.New("invalid role slug")
	}
	if req.Permissions == nil {
		req.Permissions = map[string]map[string]bool{}
	}
	now := time.Now().UTC()
	role := &AppRole{
		ID:          uuid.New().String(),
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(req.Description),
		Type:        "custom",
		IsDefault:   req.IsDefault,
		Permissions: req.Permissions,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.CreateRole(ctx, role); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, meta, "app_role.create", "users_permissions:role", role.ID, map[string]any{"slug": role.Slug})
	return s.repo.GetRoleByID(ctx, role.ID)
}

func (s *Service) UpdateRole(ctx context.Context, id string, req UpdateAppRoleRequest, meta settings.AuditMeta) (*AppRole, error) {
	existing, err := s.repo.GetRoleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Name) != "" {
		existing.Name = strings.TrimSpace(req.Name)
	}
	existing.Description = strings.TrimSpace(req.Description)
	if req.Permissions != nil {
		existing.Permissions = req.Permissions
	}
	// Do not allow setting "public" role as default end-user role
	if existing.Slug != "public" {
		existing.IsDefault = req.IsDefault
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateRole(ctx, existing); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, meta, "app_role.update", "users_permissions:role", existing.ID, map[string]any{"slug": existing.Slug})
	return s.repo.GetRoleByID(ctx, existing.ID)
}

func (s *Service) DeleteRole(ctx context.Context, id string, meta settings.AuditMeta) error {
	existing, err := s.repo.GetRoleByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.Type == "public" || existing.Type == "authenticated" || existing.Slug == "public" || existing.Slug == "authenticated" || existing.IsDefault {
		return ErrSystemAppRole
	}
	count, err := s.repo.CountUsersByRole(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrAppRoleInUse
	}
	if err := s.repo.DeleteRole(ctx, id); err != nil {
		return err
	}
	s.recordAudit(ctx, meta, "app_role.delete", "users_permissions:role", id, map[string]any{"slug": existing.Slug})
	return nil
}

func (s *Service) ListUsers(ctx context.Context, filter AppUserListFilter) ([]AppUser, int, error) {
	return s.repo.ListUsers(ctx, filter)
}

func (s *Service) GetUser(ctx context.Context, id string) (*AppUser, error) {
	return s.repo.GetUserByID(ctx, id)
}

func (s *Service) CreateUser(ctx context.Context, req CreateAppUserRequest, meta settings.AuditMeta) (*AppUser, error) {
	username := strings.TrimSpace(req.Username)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !usernamePattern.MatchString(username) {
		return nil, errors.New("username must be 3-64 alphanumeric characters (._- allowed)")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, errors.New("invalid email address")
	}
	if err := s.validateAppPassword(ctx, req.Password); err != nil {
		return nil, err
	}
	roleID := strings.TrimSpace(req.RoleID)
	if roleID == "" {
		if dr, err := s.repo.GetDefaultRole(ctx); err == nil {
			roleID = dr.ID
		}
	} else if _, err := s.repo.GetRoleByID(ctx, roleID); err != nil {
		return nil, err
	}

	hash, err := auth.HashPassword(req.Password, nil)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	u := &AppUser{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		Provider:     "local",
		Confirmed:    req.Confirmed,
		Blocked:      req.Blocked,
		RoleID:       roleID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.CreateUser(ctx, u); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, meta, "app_user.create", "users_permissions:user", u.ID, map[string]any{"email": u.Email})
	return s.repo.GetUserByID(ctx, u.ID)
}

func (s *Service) UpdateUser(ctx context.Context, id string, req UpdateAppUserRequest, meta settings.AuditMeta) (*AppUser, error) {
	u, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Username != nil {
		uname := strings.TrimSpace(*req.Username)
		if !usernamePattern.MatchString(uname) {
			return nil, errors.New("username must be 3-64 alphanumeric characters (._- allowed)")
		}
		u.Username = uname
	}
	if req.Email != nil {
		em := strings.ToLower(strings.TrimSpace(*req.Email))
		if _, err := mail.ParseAddress(em); err != nil {
			return nil, errors.New("invalid email address")
		}
		u.Email = em
	}
	if req.Password != nil && *req.Password != "" {
		if err := s.validateAppPassword(ctx, *req.Password); err != nil {
			return nil, err
		}
		hash, err := auth.HashPassword(*req.Password, nil)
		if err != nil {
			return nil, err
		}
		u.PasswordHash = hash
	}
	if req.RoleID != nil && *req.RoleID != "" {
		if _, err := s.repo.GetRoleByID(ctx, *req.RoleID); err != nil {
			return nil, err
		}
		u.RoleID = *req.RoleID
	}
	if req.Confirmed != nil {
		u.Confirmed = *req.Confirmed
	}
	if req.Blocked != nil {
		u.Blocked = *req.Blocked
	}
	u.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, meta, "app_user.update", "users_permissions:user", u.ID, map[string]any{"blocked": u.Blocked, "confirmed": u.Confirmed})
	return s.repo.GetUserByID(ctx, u.ID)
}

func (s *Service) DeleteUser(ctx context.Context, id string, meta settings.AuditMeta) error {
	if err := s.repo.DeleteUser(ctx, id); err != nil {
		return err
	}
	s.recordAudit(ctx, meta, "app_user.delete", "users_permissions:user", id, nil)
	return nil
}

func sanitizeProvider(p *OAuthProvider) OAuthProvider {
	out := *p
	out.SecretConfigured = strings.TrimSpace(p.ClientSecretEnc) != ""
	out.ClientSecretEnc = ""
	if out.SecretConfigured {
		out.ClientSecret = settings.SecretPreserveSentinel
	} else {
		out.ClientSecret = ""
	}
	return out
}

func (s *Service) ListProviders(ctx context.Context, scope string) ([]OAuthProvider, error) {
	items, err := s.repo.ListProviders(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make([]OAuthProvider, 0, len(items))
	for i := range items {
		out = append(out, sanitizeProvider(&items[i]))
	}
	return out, nil
}

func (s *Service) ListEnabledPublicProviders(ctx context.Context, scope string) ([]PublicProviderInfo, error) {
	items, err := s.repo.ListProviders(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make([]PublicProviderInfo, 0)
	for _, p := range items {
		if p.Enabled && strings.TrimSpace(p.ClientID) != "" {
			out = append(out, PublicProviderInfo{
				Name:         p.Name,
				DisplayName:  p.DisplayName,
				ProviderType: p.ProviderType,
				Scope:        p.Scope,
			})
		}
	}
	return out, nil
}

func (s *Service) UpsertProvider(ctx context.Context, name string, req UpsertOAuthProviderRequest, meta settings.AuditMeta) (*OAuthProvider, error) {
	cleanName := strings.ToLower(strings.TrimSpace(name))
	if cleanName == "" {
		cleanName = strings.ToLower(strings.TrimSpace(req.Name))
	}
	if cleanName == "" {
		return nil, errors.New("provider name is required")
	}

	existing, err := s.repo.GetProviderByName(ctx, cleanName)
	now := time.Now().UTC()
	var target *OAuthProvider
	if err == nil && existing != nil {
		target = existing
	} else {
		pt := strings.TrimSpace(strings.ToLower(req.ProviderType))
		if pt == "" {
			pt = "oidc"
		}
		sc := strings.TrimSpace(strings.ToLower(req.Scope))
		if sc != "admin" {
			sc = "app"
		}
		target = &OAuthProvider{
			ID:           uuid.New().String(),
			Name:         cleanName,
			ProviderType: pt,
			Scope:        sc,
			Scopes:       "openid email profile",
			CreatedAt:    now,
		}
	}

	if strings.TrimSpace(req.DisplayName) != "" {
		target.DisplayName = strings.TrimSpace(req.DisplayName)
	}
	if target.DisplayName == "" {
		target.DisplayName = strings.ToUpper(cleanName[:1]) + cleanName[1:]
	}
	if strings.TrimSpace(req.ProviderType) != "" {
		target.ProviderType = strings.ToLower(strings.TrimSpace(req.ProviderType))
	}
	if strings.TrimSpace(req.Scope) != "" {
		sc := strings.ToLower(strings.TrimSpace(req.Scope))
		if sc == "admin" || sc == "app" {
			target.Scope = sc
		}
	}
	target.Enabled = req.Enabled
	target.ClientID = strings.TrimSpace(req.ClientID)
	target.AuthURL = strings.TrimSpace(req.AuthURL)
	target.TokenURL = strings.TrimSpace(req.TokenURL)
	target.UserinfoURL = strings.TrimSpace(req.UserinfoURL)
	target.IssuerURL = strings.TrimSpace(req.IssuerURL)
	target.RedirectURI = strings.TrimSpace(req.RedirectURI)
	if strings.TrimSpace(req.Scopes) != "" {
		target.Scopes = strings.TrimSpace(req.Scopes)
	}
	if req.Config != nil {
		target.Config = req.Config
	}

	// Validate configured endpoint URLs against SSRF rules
	if !s.allowUnsafeTestURLs {
		for _, u := range []string{target.AuthURL, target.TokenURL, target.UserinfoURL, target.IssuerURL} {
			if u != "" {
				if err := ValidateExternalURL(u); err != nil {
					return nil, err
				}
			}
		}
	}

	if target.Enabled && (target.ClientID == "" || target.AuthURL == "" || target.TokenURL == "") {
		return nil, errors.New("enabled provider requires client_id, auth_url, and token_url")
	}

	// Preserve existing secret when sentinel is sent, or encrypt new secret
	if req.ClientSecret != "" {
		if req.ClientSecret == settings.SecretPreserveSentinel {
			// keep target.ClientSecretEnc untouched
		} else {
			enc, encErr := settings.EncryptSecret(s.encryptionKey(), strings.TrimSpace(req.ClientSecret))
			if encErr != nil {
				return nil, fmt.Errorf("failed to encrypt client secret: %w", encErr)
			}
			target.ClientSecretEnc = enc
		}
	}

	target.UpdatedAt = now
	if err := s.repo.UpsertProvider(ctx, target); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, meta, "oauth_provider.upsert", "users_permissions:provider", target.Name, map[string]any{
		"scope":   target.Scope,
		"enabled": target.Enabled,
	})
	saved, err := s.repo.GetProviderByName(ctx, target.Name)
	if err != nil {
		res := sanitizeProvider(target)
		return &res, nil
	}
	res := sanitizeProvider(saved)
	return &res, nil
}

func (s *Service) DeleteProvider(ctx context.Context, name string, meta settings.AuditMeta) error {
	if err := s.repo.DeleteProvider(ctx, name); err != nil {
		return err
	}
	s.recordAudit(ctx, meta, "oauth_provider.delete", "users_permissions:provider", name, nil)
	return nil
}

// DiscoverOIDC fetches an OpenID Provider's .well-known/openid-configuration document safely.
func (s *Service) DiscoverOIDC(ctx context.Context, issuerURL string) (*OIDCDiscoveryResult, error) {
	return s.discoverOIDCWithClient(ctx, issuerURL)
}

// --- OAuth2 / OIDC / Admin SSO Authorization & Callback ---

func (s *Service) resolveCallbackURI(provider *OAuthProvider, expectedScope, baseURL string) string {
	if strings.TrimSpace(provider.RedirectURI) != "" {
		return strings.TrimSpace(provider.RedirectURI)
	}
	cleanBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if expectedScope == "admin" {
		return cleanBase + "/api/v1/auth/sso/" + url.PathEscape(provider.Name) + "/callback"
	}
	return cleanBase + "/api/v1/app-auth/providers/" + url.PathEscape(provider.Name) + "/callback"
}

// StartOAuthAuthorize constructs a PKCE S256 authorization URL and signed short-lived state token.
func (s *Service) StartOAuthAuthorize(ctx context.Context, providerName, expectedScope, redirectAfter, baseURL string) (*OAuthAuthorizeResult, error) {
	provider, err := s.repo.GetProviderByName(ctx, providerName)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(provider.Scope, expectedScope) {
		return nil, ErrProviderNotFound
	}
	if !provider.Enabled || strings.TrimSpace(provider.ClientID) == "" || strings.TrimSpace(provider.AuthURL) == "" {
		return nil, ErrProviderDisabled
	}
	if expectedScope == "admin" && s.settingsSvc != nil {
		ssoCfg, err := s.settingsSvc.GetSSOSettings(ctx)
		if err != nil || ssoCfg == nil || !ssoCfg.Enabled {
			return nil, ErrProviderDisabled
		}
	}

	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, err
	}
	state, err := signOAuthState(provider.Name, expectedScope, verifier, redirectAfter, s.jwtSecret())
	if err != nil {
		return nil, err
	}

	parsedAuth, err := url.Parse(provider.AuthURL)
	if err != nil {
		return nil, err
	}
	q := parsedAuth.Query()
	q.Set("response_type", "code")
	q.Set("client_id", provider.ClientID)
	q.Set("redirect_uri", s.resolveCallbackURI(provider, expectedScope, baseURL))
	if strings.TrimSpace(provider.Scopes) != "" {
		q.Set("scope", provider.Scopes)
	}
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	parsedAuth.RawQuery = q.Encode()

	return &OAuthAuthorizeResult{
		AuthorizationURL: parsedAuth.String(),
		State:            state,
	}, nil
}

// HandleOAuthCallback validates state, exchanges the authorization code with PKCE verifier, and logs in app or admin user.
func (s *Service) HandleOAuthCallback(ctx context.Context, providerName, expectedScope, code, stateToken, baseURL, clientIP string) (*OAuthCallbackResult, error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("missing authorization code")
	}
	provider, err := s.repo.GetProviderByName(ctx, providerName)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(provider.Scope, expectedScope) || !provider.Enabled {
		return nil, ErrProviderDisabled
	}

	stateClaims, err := verifyOAuthState(stateToken, provider.Name, expectedScope, s.jwtSecret())
	if err != nil {
		return nil, err
	}

	var plainSecret string
	if strings.TrimSpace(provider.ClientSecretEnc) != "" {
		dec, decErr := settings.DecryptSecret(s.encryptionKey(), provider.ClientSecretEnc)
		if decErr != nil {
			return nil, fmt.Errorf("failed to decrypt provider secret: %w", decErr)
		}
		plainSecret = dec
	}

	redirectURI := s.resolveCallbackURI(provider, expectedScope, baseURL)
	profile, err := s.exchangeCodeAndFetchProfile(ctx, provider, plainSecret, code, redirectURI, stateClaims.CodeVerifier)
	if err != nil {
		return nil, err
	}

	if expectedScope == "admin" {
		if s.adminSSOFn == nil {
			return nil, errors.New("admin SSO login handler is not configured")
		}
		var autoProvision bool
		var defaultRoleID string
		if s.settingsSvc != nil {
			ssoCfg, err := s.settingsSvc.GetSSOSettings(ctx)
			if err != nil || ssoCfg == nil || !ssoCfg.Enabled {
				return nil, ErrProviderDisabled
			}
			if len(ssoCfg.AllowedDomains) > 0 {
				parts := strings.SplitN(strings.ToLower(profile.Email), "@", 2)
				domain := ""
				if len(parts) == 2 {
					domain = parts[1]
				}
				allowed := false
				for _, d := range ssoCfg.AllowedDomains {
					if strings.EqualFold(d, domain) {
						allowed = true
						break
					}
				}
				if !allowed {
					return nil, ErrAppForbidden
				}
			}
			autoProvision = ssoCfg.AutoProvisionAdmin
			defaultRoleID = ssoCfg.DefaultRoleID
		}

		res, err := s.adminSSOFn(ctx, auth.SSOLoginRequest{
			Provider:      provider.Name,
			Subject:       profile.Subject,
			Email:         profile.Email,
			FirstName:     profile.FirstName,
			LastName:      profile.LastName,
			AutoProvision: autoProvision,
			DefaultRoleID: defaultRoleID,
			ClientIP:      clientIP,
		})
		if err != nil {
			return nil, err
		}
		return &OAuthCallbackResult{
			Scope:         "admin",
			AdminAuth:     res,
			RedirectAfter: stateClaims.RedirectAfter,
		}, nil
	}

	// App-scope user authentication / auto-registration
	cfg, _ := s.GetSettings(ctx)
	user, err := s.repo.GetUserByEmail(ctx, profile.Email)
	if err != nil {
		if !errors.Is(err, ErrAppUserNotFound) {
			return nil, err
		}
		if cfg != nil && !cfg.AllowRegister {
			return nil, ErrRegistrationDisabled
		}

		randomBytes := make([]byte, 24)
		_, _ = rand.Read(randomBytes)
		pwHash, hashErr := auth.HashPassword("OAuth!"+hex.EncodeToString(randomBytes), nil)
		if hashErr != nil {
			return nil, hashErr
		}

		username := slugCleaner.ReplaceAllString(strings.ToLower(profile.Username), "_")
		username = strings.Trim(username, "_")
		if len(username) < 3 {
			username = "user_" + uuid.NewString()[:6]
		}
		if _, existsErr := s.repo.GetUserByUsername(ctx, username); existsErr == nil {
			username = username + "_" + uuid.NewString()[:5]
		}

		roleID := ""
		if cfg != nil && cfg.DefaultRoleID != "" {
			roleID = cfg.DefaultRoleID
		} else if dr, drErr := s.repo.GetDefaultRole(ctx); drErr == nil {
			roleID = dr.ID
		}

		now := time.Now().UTC()
		user = &AppUser{
			ID:           uuid.New().String(),
			Username:     username,
			Email:        profile.Email,
			PasswordHash: pwHash,
			Provider:     provider.Name,
			Confirmed:    true,
			Blocked:      false,
			RoleID:       roleID,
			LastLoginAt:  &now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := s.repo.CreateUser(ctx, user); err != nil {
			return nil, err
		}
	} else {
		if user.Blocked {
			return nil, ErrAppAccountBlocked
		}
		now := time.Now().UTC()
		user.Confirmed = true
		user.LastLoginAt = &now
		user.UpdatedAt = now
		_ = s.repo.UpdateUser(ctx, user)
	}

	hydrated, err := s.repo.GetUserByID(ctx, user.ID)
	if err != nil {
		hydrated = user
	}
	ttl := 1440 * time.Minute
	if cfg != nil && cfg.SessionExpirationMinutes > 0 {
		ttl = time.Duration(cfg.SessionExpirationMinutes) * time.Minute
	}
	tok, err := GenerateAppAccessToken(hydrated, s.jwtSecret(), ttl)
	if err != nil {
		return nil, err
	}
	return &OAuthCallbackResult{
		Scope:         "app",
		AppAuth:       &AppAuthResult{Token: tok, User: hydrated},
		RedirectAfter: stateClaims.RedirectAfter,
	}, nil
}

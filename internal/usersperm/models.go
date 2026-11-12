package usersperm

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrAppUserNotFound       = errors.New("application user not found")
	ErrAppUserExists         = errors.New("application user with this email or username already exists")
	ErrAppRoleNotFound       = errors.New("application role not found")
	ErrAppRoleExists         = errors.New("application role already exists")
	ErrProviderNotFound      = errors.New("OAuth provider not found")
	ErrProviderDisabled      = errors.New("OAuth provider is disabled")
	ErrSystemAppRole         = errors.New("built-in application role cannot be deleted or have its slug modified")
	ErrAppRoleInUse          = errors.New("application role is currently assigned to one or more users")
	ErrAppUnauthorized       = errors.New("unauthorized application user")
	ErrAppForbidden          = errors.New("forbidden")
	ErrInvalidAppCredentials = errors.New("invalid email/username or password")
	ErrAppAccountBlocked     = errors.New("this account has been blocked")
	ErrAppAccountUnconfirmed = errors.New("email address must be confirmed before logging in")
	ErrInvalidTokenOrExpired = errors.New("token is invalid or has expired")
	ErrRegistrationDisabled  = errors.New("public application registration is currently disabled")
	ErrInvalidOAuthState     = errors.New("invalid or expired OAuth state parameter")
	ErrUnsafeURL             = errors.New("target URL is not permitted by network security policy")
)

// AppRole represents a permission profile assigned to end-user / public API consumers.
type AppRole struct {
	ID          string                     `json:"id"`
	Name        string                     `json:"name"`
	Slug        string                     `json:"slug"`
	Description string                     `json:"description"`
	Type        string                     `json:"type"` // "authenticated", "public", "custom"
	IsDefault   bool                       `json:"is_default"`
	Permissions map[string]map[string]bool `json:"permissions"`
	UserCount   int                        `json:"user_count,omitempty"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
}

// AppUser represents an end-user / application account, strictly isolated from CMS admin accounts.
type AppUser struct {
	ID                    string     `json:"id"`
	Username              string     `json:"username"`
	Email                 string     `json:"email"`
	PasswordHash          string     `json:"-"`
	Provider              string     `json:"provider"`
	Confirmed             bool       `json:"confirmed"`
	Blocked               bool       `json:"blocked"`
	ConfirmationTokenHash string     `json:"-"`
	ConfirmationExpiresAt *time.Time `json:"-"`
	ResetTokenHash        string     `json:"-"`
	ResetExpiresAt        *time.Time `json:"-"`
	RoleID                string     `json:"role_id"`
	Role                  *AppRole   `json:"role,omitempty"`
	LastLoginAt           *time.Time `json:"last_login_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// OAuthProvider configures an external OAuth2 or OIDC provider for app users ("app") or admin SSO ("admin").
type OAuthProvider struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`         // unique identifier e.g. "google", "github", "discord", "oidc", "admin_oidc"
	DisplayName      string         `json:"display_name"`
	ProviderType     string         `json:"provider_type"` // "google", "github", "discord", "oidc", "custom"
	Scope            string         `json:"scope"`         // "app" or "admin"
	Enabled          bool           `json:"enabled"`
	ClientID         string         `json:"client_id"`
	ClientSecretEnc  string         `json:"-"`
	ClientSecret     string         `json:"client_secret,omitempty"` // returns sentinel when configured, never plaintext
	SecretConfigured bool           `json:"secret_configured"`
	AuthURL          string         `json:"auth_url"`
	TokenURL         string         `json:"token_url"`
	UserinfoURL      string         `json:"userinfo_url"`
	IssuerURL        string         `json:"issuer_url"`
	RedirectURI      string         `json:"redirect_uri"`
	Scopes           string         `json:"scopes"`
	Config           map[string]any `json:"config,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// PublicProviderInfo exposes only safe non-secret fields to unauthenticated login pages.
type PublicProviderInfo struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	ProviderType string `json:"provider_type"`
	Scope        string `json:"scope"`
}

// AppAuthSettings configures end-user registration, confirmation, reset URLs, and default role assignment.
type AppAuthSettings struct {
	AllowRegister                    bool      `json:"allow_register"`
	EmailConfirmation                bool      `json:"email_confirmation"`
	ResetPasswordURL                 string    `json:"reset_password_url"`
	EmailConfirmationRedirectionURL  string    `json:"email_confirmation_redirection_url"`
	DefaultRoleID                    string    `json:"default_role_id"`
	SessionExpirationMinutes         int       `json:"session_expiration_minutes"`
	UpdatedAt                        time.Time `json:"updated_at,omitempty"`
}

// DefaultAppAuthSettings returns baseline end-user authentication settings.
func DefaultAppAuthSettings() AppAuthSettings {
	return AppAuthSettings{
		AllowRegister:            true,
		EmailConfirmation:        false,
		SessionExpirationMinutes: 1440,
	}
}

// NormalizeAndValidate bounds and cleans AppAuthSettings.
func (s *AppAuthSettings) NormalizeAndValidate() error {
	s.ResetPasswordURL = strings.TrimSpace(s.ResetPasswordURL)
	s.EmailConfirmationRedirectionURL = strings.TrimSpace(s.EmailConfirmationRedirectionURL)
	s.DefaultRoleID = strings.TrimSpace(s.DefaultRoleID)
	if s.SessionExpirationMinutes == 0 {
		s.SessionExpirationMinutes = 1440
	}
	if s.SessionExpirationMinutes < 15 || s.SessionExpirationMinutes > 43200 {
		return errors.New("session_expiration_minutes must be between 15 and 43200")
	}
	return nil
}

// Request DTOs

type RegisterAppUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginAppUserRequest struct {
	Identifier string `json:"identifier"`
	Username   string `json:"username"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

type ConfirmEmailRequest struct {
	Token string `json:"token"`
}

type SendConfirmationRequest struct {
	Email string `json:"email"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type CreateAppUserRequest struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	RoleID    string `json:"role_id"`
	Confirmed bool   `json:"confirmed"`
	Blocked   bool   `json:"blocked"`
}

type UpdateAppUserRequest struct {
	Username  *string `json:"username,omitempty"`
	Email     *string `json:"email,omitempty"`
	Password  *string `json:"password,omitempty"`
	RoleID    *string `json:"role_id,omitempty"`
	Confirmed *bool   `json:"confirmed,omitempty"`
	Blocked   *bool   `json:"blocked,omitempty"`
}

type CreateAppRoleRequest struct {
	Name        string                     `json:"name"`
	Slug        string                     `json:"slug"`
	Description string                     `json:"description"`
	IsDefault   bool                       `json:"is_default"`
	Permissions map[string]map[string]bool `json:"permissions"`
}

type UpdateAppRoleRequest struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	IsDefault   bool                       `json:"is_default"`
	Permissions map[string]map[string]bool `json:"permissions"`
}

type UpsertOAuthProviderRequest struct {
	Name         string         `json:"name"`
	DisplayName  string         `json:"display_name"`
	ProviderType string         `json:"provider_type"`
	Scope        string         `json:"scope"`
	Enabled      bool           `json:"enabled"`
	ClientID     string         `json:"client_id"`
	ClientSecret string         `json:"client_secret"`
	AuthURL      string         `json:"auth_url"`
	TokenURL     string         `json:"token_url"`
	UserinfoURL  string         `json:"userinfo_url"`
	IssuerURL    string         `json:"issuer_url"`
	RedirectURI  string         `json:"redirect_uri"`
	Scopes       string         `json:"scopes"`
	Config       map[string]any `json:"config,omitempty"`
}

// AppUserListFilter filters paginated application users for admin listings.
type AppUserListFilter struct {
	Search string
	RoleID string
	Limit  int
	Offset int
}

// OIDCDiscoveryResult contains discovered OpenID Connect endpoints.
type OIDCDiscoveryResult struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserinfoEndpoint      string   `json:"userinfo_endpoint"`
	ScopesSupported       []string `json:"scopes_supported,omitempty"`
}


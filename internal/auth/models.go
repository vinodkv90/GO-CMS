package auth

import "time"

// UserStatus represents the account state of a user.
type UserStatus string

const (
	StatusActive    UserStatus = "active"
	StatusInactive  UserStatus = "inactive"
	StatusSuspended UserStatus = "suspended"
)

// Standard role slugs.
const (
	RoleSuperAdmin = "super_admin"
	RoleEditor     = "editor"
	RoleAuthor     = "author"
	RoleViewer     = "viewer"
)

// Permission represents a granular action permitted on a resource.
type Permission struct {
	ID          string    `json:"id" bson:"_id"`
	Action      string    `json:"action" bson:"action"`     // e.g. "create", "read", "update", "delete", "publish", "*"
	Resource    string    `json:"resource" bson:"resource"` // e.g. "content", "media", "users", "roles", "settings", "*"
	Description string    `json:"description" bson:"description"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
}

// Role represents a grouped collection of permissions assigned to users.
type Role struct {
	ID          string       `json:"id" bson:"_id"`
	Name        string       `json:"name" bson:"name"`
	Slug        string       `json:"slug" bson:"slug"`
	Description string       `json:"description" bson:"description"`
	IsSystem    bool         `json:"is_system" bson:"is_system"`
	UserCount   int          `json:"user_count,omitempty" bson:"user_count,omitempty"`
	Permissions []Permission `json:"permissions,omitempty" bson:"permissions,omitempty"`
	CreatedAt   time.Time    `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at" bson:"updated_at"`
}

// User represents an authenticated CMS account.
type User struct {
	ID                  string     `json:"id" bson:"_id"`
	Username            string     `json:"username" bson:"username"`
	Email               string     `json:"email" bson:"email"`
	PasswordHash        string     `json:"-" bson:"password_hash"`
	Status              UserStatus `json:"status" bson:"status"`
	FirstName           string     `json:"first_name" bson:"first_name"`
	LastName            string     `json:"last_name" bson:"last_name"`
	Roles               []Role     `json:"roles,omitempty" bson:"roles,omitempty"`
	LastLoginAt         *time.Time `json:"last_login_at,omitempty" bson:"last_login_at,omitempty"`
	FailedLoginAttempts int        `json:"failed_login_attempts,omitempty" bson:"failed_login_attempts,omitempty"`
	LockedUntil         *time.Time `json:"locked_until,omitempty" bson:"locked_until,omitempty"`
	PasswordChangedAt   *time.Time `json:"password_changed_at,omitempty" bson:"password_changed_at,omitempty"`
	MFAEnabled          bool       `json:"mfa_enabled" bson:"mfa_enabled"`
	MFASecretEnc        string     `json:"-" bson:"mfa_secret_enc,omitempty"`
	MFABackupCodesHash  string     `json:"-" bson:"mfa_backup_codes_hash,omitempty"`
	SSOProvider         string     `json:"sso_provider,omitempty" bson:"sso_provider,omitempty"`
	SSOSubject          string     `json:"sso_subject,omitempty" bson:"sso_subject,omitempty"`
	CreatedAt           time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" bson:"updated_at"`
}

// InvitationStatus represents the lifecycle state of an admin invitation.
type InvitationStatus string

const (
	InvitationPending  InvitationStatus = "pending"
	InvitationAccepted InvitationStatus = "accepted"
	InvitationRevoked  InvitationStatus = "revoked"
	InvitationExpired  InvitationStatus = "expired"
)

// AdminInvitation represents a pending or completed admin onboarding invitation.
type AdminInvitation struct {
	ID         string           `json:"id" bson:"_id"`
	Email      string           `json:"email" bson:"email"`
	Username   string           `json:"username" bson:"username"`
	FirstName  string           `json:"first_name" bson:"first_name"`
	LastName   string           `json:"last_name" bson:"last_name"`
	RoleIDs    []string         `json:"role_ids" bson:"role_ids"`
	Roles      []Role           `json:"roles,omitempty" bson:"roles,omitempty"`
	TokenHash  string           `json:"-" bson:"token_hash"`
	Status     InvitationStatus `json:"status" bson:"status"`
	InvitedBy  string           `json:"invited_by,omitempty" bson:"invited_by,omitempty"`
	ExpiresAt  time.Time        `json:"expires_at" bson:"expires_at"`
	AcceptedAt *time.Time       `json:"accepted_at,omitempty" bson:"accepted_at,omitempty"`
	CreatedAt  time.Time        `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at" bson:"updated_at"`
	EmailSent  *bool            `json:"email_sent,omitempty" bson:"-"`
}

// TokenKind differentiates headless Content API tokens, Admin PATs, and Transfer tokens.
type TokenKind string

const (
	TokenKindAPI      TokenKind = "api"
	TokenKindAdmin    TokenKind = "admin"
	TokenKindTransfer TokenKind = "transfer"
)

// APIToken represents a programmatic token for headless CMS API, admin, or transfer access.
type APIToken struct {
	ID          string     `json:"id" bson:"_id"`
	Name        string     `json:"name" bson:"name"`
	Kind        TokenKind  `json:"kind" bson:"kind"`
	Description string     `json:"description,omitempty" bson:"description,omitempty"`
	TokenType   string     `json:"token_type" bson:"token_type"`
	TokenPrefix string     `json:"token_prefix" bson:"token_prefix"`
	TokenHash   string     `json:"-" bson:"token_hash"`
	UserID      string     `json:"user_id" bson:"user_id"`
	Permissions []string   `json:"permissions" bson:"permissions"` // List of "action:resource" strings
	ExpiresAt   *time.Time `json:"expires_at,omitempty" bson:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty" bson:"last_used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty" bson:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at" bson:"created_at"`
}

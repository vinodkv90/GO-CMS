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
	Permissions []Permission `json:"permissions,omitempty" bson:"permissions,omitempty"`
	CreatedAt   time.Time    `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at" bson:"updated_at"`
}

// User represents an authenticated CMS account.
type User struct {
	ID           string     `json:"id" bson:"_id"`
	Username     string     `json:"username" bson:"username"`
	Email        string     `json:"email" bson:"email"`
	PasswordHash string     `json:"-" bson:"password_hash"`
	Status       UserStatus `json:"status" bson:"status"`
	FirstName    string     `json:"first_name" bson:"first_name"`
	LastName     string     `json:"last_name" bson:"last_name"`
	Roles        []Role     `json:"roles,omitempty" bson:"roles,omitempty"`
	CreatedAt    time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" bson:"updated_at"`
}

// APIToken represents a programmatic Personal Access Token for headless CMS API access.
type APIToken struct {
	ID          string     `json:"id" bson:"_id"`
	Name        string     `json:"name" bson:"name"`
	TokenHash   string     `json:"-" bson:"token_hash"`
	UserID      string     `json:"user_id" bson:"user_id"`
	Permissions []string   `json:"permissions" bson:"permissions"` // List of "action:resource" strings
	ExpiresAt   *time.Time `json:"expires_at,omitempty" bson:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty" bson:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at" bson:"created_at"`
}

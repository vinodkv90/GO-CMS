package auth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user with this username or email already exists")
	ErrRoleNotFound       = errors.New("role not found")
	ErrRoleAlreadyExists  = errors.New("role with this slug already exists")
	ErrPermissionNotFound = errors.New("permission not found")
	ErrTokenNotFound      = errors.New("api token not found")
	ErrInvitationNotFound = errors.New("invitation not found")
	ErrInvitationExpired  = errors.New("invitation has expired")
	ErrInvitationRevoked  = errors.New("invitation has been revoked or already accepted")
)

// UserListFilter configures server-side filtering and pagination of users.
type UserListFilter struct {
	Search string
	Status string
	RoleID string
	Limit  int
	Offset int
}

// UserRepository manages persistence of User entities.
type UserRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, id string) error
	ListUsers(ctx context.Context, limit, offset int) ([]*User, int, error)
	ListUsersFiltered(ctx context.Context, filter UserListFilter) ([]*User, int, error)
	CountUsers(ctx context.Context) (int, error)
}

// RoleRepository manages persistence of Role entities and user-role relations.
type RoleRepository interface {
	CreateRole(ctx context.Context, role *Role) error
	GetRoleByID(ctx context.Context, id string) (*Role, error)
	GetRoleBySlug(ctx context.Context, slug string) (*Role, error)
	ListRoles(ctx context.Context) ([]*Role, error)
	UpdateRole(ctx context.Context, role *Role) error
	DeleteRole(ctx context.Context, id string) error
	AssignRoleToUser(ctx context.Context, userID, roleID string) error
	RemoveRoleFromUser(ctx context.Context, userID, roleID string) error
	ReplaceUserRoles(ctx context.Context, userID string, roleIDs []string) error
	GetUserRoles(ctx context.Context, userID string) ([]Role, error)
	CountUsersByRoleID(ctx context.Context, roleID string) (int, error)
	CountActiveUsersWithRoleSlug(ctx context.Context, slug string) (int, error)
}

// PermissionRepository manages granular system permissions.
type PermissionRepository interface {
	CreatePermission(ctx context.Context, perm *Permission) error
	ListPermissions(ctx context.Context) ([]Permission, error)
	GetByActionAndResource(ctx context.Context, action, resource string) (*Permission, error)
	AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error
	ReplaceRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error
	GetRolePermissions(ctx context.Context, roleID string) ([]Permission, error)
}

// InvitationRepository manages admin invitation persistence.
type InvitationRepository interface {
	CreateInvitation(ctx context.Context, inv *AdminInvitation) error
	GetInvitationByID(ctx context.Context, id string) (*AdminInvitation, error)
	GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*AdminInvitation, error)
	ListInvitations(ctx context.Context) ([]*AdminInvitation, error)
	UpdateInvitation(ctx context.Context, inv *AdminInvitation) error
	DeleteInvitation(ctx context.Context, id string) error
}

// APITokenRepository manages API, Admin PAT, and Transfer Tokens.
type APITokenRepository interface {
	CreateToken(ctx context.Context, token *APIToken) error
	GetByID(ctx context.Context, id string) (*APIToken, error)
	GetByTokenHash(ctx context.Context, hash string) (*APIToken, error)
	ListTokensByUserID(ctx context.Context, userID string) ([]*APIToken, error)
	ListTokensByKind(ctx context.Context, userID string, kind TokenKind, includeAll bool) ([]*APIToken, error)
	UpdateToken(ctx context.Context, token *APIToken) error
	RevokeToken(ctx context.Context, id string, revokedAt time.Time) error
	DeleteToken(ctx context.Context, id string, userID string) error
	UpdateLastUsed(ctx context.Context, id string, lastUsed time.Time) error
}

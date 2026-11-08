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
)

// UserRepository manages persistence of User entities.
type UserRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, id string) error
	ListUsers(ctx context.Context, limit, offset int) ([]*User, int, error)
	CountUsers(ctx context.Context) (int, error)
}

// RoleRepository manages persistence of Role entities and user-role relations.
type RoleRepository interface {
	CreateRole(ctx context.Context, role *Role) error
	GetRoleByID(ctx context.Context, id string) (*Role, error)
	GetRoleBySlug(ctx context.Context, slug string) (*Role, error)
	ListRoles(ctx context.Context) ([]*Role, error)
	AssignRoleToUser(ctx context.Context, userID, roleID string) error
	RemoveRoleFromUser(ctx context.Context, userID, roleID string) error
	GetUserRoles(ctx context.Context, userID string) ([]Role, error)
}

// PermissionRepository manages granular system permissions.
type PermissionRepository interface {
	CreatePermission(ctx context.Context, perm *Permission) error
	ListPermissions(ctx context.Context) ([]Permission, error)
	GetByActionAndResource(ctx context.Context, action, resource string) (*Permission, error)
	AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error
	GetRolePermissions(ctx context.Context, roleID string) ([]Permission, error)
}

// APITokenRepository manages Personal Access Tokens.
type APITokenRepository interface {
	CreateToken(ctx context.Context, token *APIToken) error
	GetByTokenHash(ctx context.Context, hash string) (*APIToken, error)
	ListTokensByUserID(ctx context.Context, userID string) ([]*APIToken, error)
	DeleteToken(ctx context.Context, id string, userID string) error
	UpdateLastUsed(ctx context.Context, id string, lastUsed time.Time) error
}

package auth

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemoryAuthRepository is a thread-safe, in-memory implementation of the auth repositories.
type MemoryAuthRepository struct {
	mu              sync.RWMutex
	users           map[string]*User
	usersByUsername map[string]string
	usersByEmail    map[string]string
	roles           map[string]*Role
	rolesBySlug     map[string]string
	permissions     map[string]*Permission
	userRoles       map[string]map[string]bool // userID -> roleID -> true
	rolePerms       map[string]map[string]bool // roleID -> permID -> true
	apiTokens       map[string]*APIToken
	tokensByHash    map[string]string
}

// NewMemoryAuthRepository initializes an in-memory repository.
func NewMemoryAuthRepository() *MemoryAuthRepository {
	return &MemoryAuthRepository{
		users:           make(map[string]*User),
		usersByUsername: make(map[string]string),
		usersByEmail:    make(map[string]string),
		roles:           make(map[string]*Role),
		rolesBySlug:     make(map[string]string),
		permissions:     make(map[string]*Permission),
		userRoles:       make(map[string]map[string]bool),
		rolePerms:       make(map[string]map[string]bool),
		apiTokens:       make(map[string]*APIToken),
		tokensByHash:    make(map[string]string),
	}
}

// --- Users ---

func (m *MemoryAuthRepository) CreateUser(ctx context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lowerUsername := strings.ToLower(u.Username)
	lowerEmail := strings.ToLower(u.Email)

	if _, exists := m.usersByUsername[lowerUsername]; exists {
		return ErrUserAlreadyExists
	}
	if _, exists := m.usersByEmail[lowerEmail]; exists {
		return ErrUserAlreadyExists
	}

	clone := *u
	m.users[u.ID] = &clone
	m.usersByUsername[lowerUsername] = u.ID
	m.usersByEmail[lowerEmail] = u.ID
	return nil
}

func (m *MemoryAuthRepository) GetUserByID(ctx context.Context, id string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, ok := m.users[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	clone := *u
	clone.Roles = m.getUserRolesLocked(id)
	return &clone, nil
}

func (m *MemoryAuthRepository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.usersByUsername[strings.ToLower(username)]
	if !ok {
		return nil, ErrUserNotFound
	}
	return m.GetUserByID(ctx, id)
}

func (m *MemoryAuthRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.usersByEmail[strings.ToLower(email)]
	if !ok {
		return nil, ErrUserNotFound
	}
	return m.GetUserByID(ctx, id)
}

func (m *MemoryAuthRepository) UpdateUser(ctx context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.users[u.ID]
	if !ok {
		return ErrUserNotFound
	}

	existing.Email = u.Email
	existing.Status = u.Status
	existing.FirstName = u.FirstName
	existing.LastName = u.LastName
	existing.UpdatedAt = u.UpdatedAt
	return nil
}

func (m *MemoryAuthRepository) DeleteUser(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	u, ok := m.users[id]
	if !ok {
		return ErrUserNotFound
	}

	delete(m.usersByUsername, strings.ToLower(u.Username))
	delete(m.usersByEmail, strings.ToLower(u.Email))
	delete(m.users, id)
	delete(m.userRoles, id)
	return nil
}

func (m *MemoryAuthRepository) ListUsers(ctx context.Context, limit, offset int) ([]*User, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []*User
	for _, u := range m.users {
		clone := *u
		clone.Roles = m.getUserRolesLocked(u.ID)
		all = append(all, &clone)
	}

	total := len(all)
	if offset > total {
		return []*User{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return all[offset:end], total, nil
}

func (m *MemoryAuthRepository) CountUsers(ctx context.Context) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.users), nil
}

// --- Roles ---

func (m *MemoryAuthRepository) CreateRole(ctx context.Context, role *Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.rolesBySlug[role.Slug]; exists {
		return ErrRoleAlreadyExists
	}

	clone := *role
	m.roles[role.ID] = &clone
	m.rolesBySlug[role.Slug] = role.ID
	return nil
}

func (m *MemoryAuthRepository) GetRoleByID(ctx context.Context, id string) (*Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	r, ok := m.roles[id]
	if !ok {
		return nil, ErrRoleNotFound
	}
	clone := *r
	clone.Permissions = m.getRolePermissionsLocked(id)
	return &clone, nil
}

func (m *MemoryAuthRepository) GetRoleBySlug(ctx context.Context, slug string) (*Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.rolesBySlug[slug]
	if !ok {
		return nil, ErrRoleNotFound
	}
	r := m.roles[id]
	clone := *r
	clone.Permissions = m.getRolePermissionsLocked(id)
	return &clone, nil
}

func (m *MemoryAuthRepository) ListRoles(ctx context.Context) ([]*Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Role
	for _, r := range m.roles {
		clone := *r
		clone.Permissions = m.getRolePermissionsLocked(r.ID)
		result = append(result, &clone)
	}
	return result, nil
}

func (m *MemoryAuthRepository) AssignRoleToUser(ctx context.Context, userID, roleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.userRoles[userID] == nil {
		m.userRoles[userID] = make(map[string]bool)
	}
	m.userRoles[userID][roleID] = true
	return nil
}

func (m *MemoryAuthRepository) RemoveRoleFromUser(ctx context.Context, userID, roleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.userRoles[userID] != nil {
		delete(m.userRoles[userID], roleID)
	}
	return nil
}

func (m *MemoryAuthRepository) GetUserRoles(ctx context.Context, userID string) ([]Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getUserRolesLocked(userID), nil
}

func (m *MemoryAuthRepository) getUserRolesLocked(userID string) []Role {
	roleIDs := m.userRoles[userID]
	var roles []Role
	for rid := range roleIDs {
		if r, ok := m.roles[rid]; ok {
			clone := *r
			clone.Permissions = m.getRolePermissionsLocked(rid)
			roles = append(roles, clone)
		}
	}
	return roles
}

// --- Permissions ---

func (m *MemoryAuthRepository) CreatePermission(ctx context.Context, perm *Permission) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clone := *perm
	m.permissions[perm.ID] = &clone
	return nil
}

func (m *MemoryAuthRepository) ListPermissions(ctx context.Context) ([]Permission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Permission
	for _, p := range m.permissions {
		result = append(result, *p)
	}
	return result, nil
}

func (m *MemoryAuthRepository) GetByActionAndResource(ctx context.Context, action, resource string) (*Permission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.permissions {
		if p.Action == action && p.Resource == resource {
			clone := *p
			return &clone, nil
		}
	}
	return nil, ErrPermissionNotFound
}

func (m *MemoryAuthRepository) AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.rolePerms[roleID] == nil {
		m.rolePerms[roleID] = make(map[string]bool)
	}
	m.rolePerms[roleID][permissionID] = true
	return nil
}

func (m *MemoryAuthRepository) GetRolePermissions(ctx context.Context, roleID string) ([]Permission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getRolePermissionsLocked(roleID), nil
}

func (m *MemoryAuthRepository) getRolePermissionsLocked(roleID string) []Permission {
	permIDs := m.rolePerms[roleID]
	var perms []Permission
	for pid := range permIDs {
		if p, ok := m.permissions[pid]; ok {
			perms = append(perms, *p)
		}
	}
	return perms
}

// --- APITokens ---

func (m *MemoryAuthRepository) CreateToken(ctx context.Context, token *APIToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clone := *token
	m.apiTokens[token.ID] = &clone
	m.tokensByHash[token.TokenHash] = token.ID
	return nil
}

func (m *MemoryAuthRepository) GetByTokenHash(ctx context.Context, hash string) (*APIToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.tokensByHash[hash]
	if !ok {
		return nil, ErrTokenNotFound
	}
	t := m.apiTokens[id]
	clone := *t
	return &clone, nil
}

func (m *MemoryAuthRepository) ListTokensByUserID(ctx context.Context, userID string) ([]*APIToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*APIToken
	for _, t := range m.apiTokens {
		if t.UserID == userID {
			clone := *t
			result = append(result, &clone)
		}
	}
	return result, nil
}

func (m *MemoryAuthRepository) DeleteToken(ctx context.Context, id, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.apiTokens[id]
	if !ok || t.UserID != userID {
		return ErrTokenNotFound
	}

	delete(m.tokensByHash, t.TokenHash)
	delete(m.apiTokens, id)
	return nil
}

func (m *MemoryAuthRepository) UpdateLastUsed(ctx context.Context, id string, lastUsed time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if t, ok := m.apiTokens[id]; ok {
		t.LastUsedAt = &lastUsed
	}
	return nil
}

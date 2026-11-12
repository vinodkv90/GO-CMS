package auth

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryAuthRepository is a thread-safe, in-memory implementation of the auth repositories.
type MemoryAuthRepository struct {
	mu                 sync.RWMutex
	users              map[string]*User
	usersByUsername    map[string]string
	usersByEmail       map[string]string
	roles              map[string]*Role
	rolesBySlug        map[string]string
	permissions        map[string]*Permission
	userRoles          map[string]map[string]bool // userID -> roleID -> true
	rolePerms          map[string]map[string]bool // roleID -> permID -> true
	apiTokens          map[string]*APIToken
	tokensByHash       map[string]string
	invitations        map[string]*AdminInvitation
	invitationsByToken map[string]string
}

// NewMemoryAuthRepository initializes an in-memory repository.
func NewMemoryAuthRepository() *MemoryAuthRepository {
	return &MemoryAuthRepository{
		users:              make(map[string]*User),
		usersByUsername:    make(map[string]string),
		usersByEmail:       make(map[string]string),
		roles:              make(map[string]*Role),
		rolesBySlug:        make(map[string]string),
		permissions:        make(map[string]*Permission),
		userRoles:          make(map[string]map[string]bool),
		rolePerms:          make(map[string]map[string]bool),
		apiTokens:          make(map[string]*APIToken),
		tokensByHash:       make(map[string]string),
		invitations:        make(map[string]*AdminInvitation),
		invitationsByToken: make(map[string]string),
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

	oldEmail := strings.ToLower(existing.Email)
	newEmail := strings.ToLower(u.Email)
	if oldEmail != newEmail {
		if ownerID, exists := m.usersByEmail[newEmail]; exists && ownerID != u.ID {
			return ErrUserAlreadyExists
		}
		delete(m.usersByEmail, oldEmail)
		m.usersByEmail[newEmail] = u.ID
	}

	existing.Email = u.Email
	existing.PasswordHash = u.PasswordHash
	existing.Status = u.Status
	existing.FirstName = u.FirstName
	existing.LastName = u.LastName
	existing.LastLoginAt = u.LastLoginAt
	existing.FailedLoginAttempts = u.FailedLoginAttempts
	existing.LockedUntil = u.LockedUntil
	existing.PasswordChangedAt = u.PasswordChangedAt
	existing.MFAEnabled = u.MFAEnabled
	existing.MFASecretEnc = u.MFASecretEnc
	existing.MFABackupCodesHash = u.MFABackupCodesHash
	existing.SSOProvider = u.SSOProvider
	existing.SSOSubject = u.SSOSubject
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
	return m.ListUsersFiltered(ctx, UserListFilter{Limit: limit, Offset: offset})
}

func (m *MemoryAuthRepository) ListUsersFiltered(ctx context.Context, filter UserListFilter) ([]*User, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	search := strings.ToLower(strings.TrimSpace(filter.Search))
	status := strings.TrimSpace(filter.Status)
	roleID := strings.TrimSpace(filter.RoleID)

	var all []*User
	for _, u := range m.users {
		if status != "" && string(u.Status) != status {
			continue
		}
		if search != "" {
			matches := strings.Contains(strings.ToLower(u.Username), search) ||
				strings.Contains(strings.ToLower(u.Email), search) ||
				strings.Contains(strings.ToLower(u.FirstName), search) ||
				strings.Contains(strings.ToLower(u.LastName), search)
			if !matches {
				continue
			}
		}
		if roleID != "" && (m.userRoles[u.ID] == nil || !m.userRoles[u.ID][roleID]) {
			continue
		}
		clone := *u
		clone.Roles = m.getUserRolesLocked(u.ID)
		all = append(all, &clone)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	total := len(all)
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
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
	clone.IsSystem = isSystemRoleSlug(r.Slug)
	clone.Permissions = m.getRolePermissionsLocked(id)
	clone.UserCount = m.countUsersByRoleLocked(id)
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
	clone.IsSystem = isSystemRoleSlug(r.Slug)
	clone.Permissions = m.getRolePermissionsLocked(id)
	clone.UserCount = m.countUsersByRoleLocked(id)
	return &clone, nil
}

func (m *MemoryAuthRepository) ListRoles(ctx context.Context) ([]*Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Role
	for _, r := range m.roles {
		clone := *r
		clone.IsSystem = isSystemRoleSlug(r.Slug)
		clone.Permissions = m.getRolePermissionsLocked(r.ID)
		clone.UserCount = m.countUsersByRoleLocked(r.ID)
		result = append(result, &clone)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (m *MemoryAuthRepository) UpdateRole(ctx context.Context, role *Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.roles[role.ID]
	if !ok {
		return ErrRoleNotFound
	}
	if existing.Slug != role.Slug {
		if ownerID, exists := m.rolesBySlug[role.Slug]; exists && ownerID != role.ID {
			return ErrRoleAlreadyExists
		}
		delete(m.rolesBySlug, existing.Slug)
		m.rolesBySlug[role.Slug] = role.ID
	}

	existing.Name = role.Name
	existing.Slug = role.Slug
	existing.Description = role.Description
	existing.UpdatedAt = role.UpdatedAt
	return nil
}

func (m *MemoryAuthRepository) DeleteRole(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.roles[id]
	if !ok {
		return ErrRoleNotFound
	}
	delete(m.rolesBySlug, r.Slug)
	delete(m.roles, id)
	delete(m.rolePerms, id)
	for uid, roles := range m.userRoles {
		delete(roles, id)
		if len(roles) == 0 {
			delete(m.userRoles, uid)
		}
	}
	return nil
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

func (m *MemoryAuthRepository) ReplaceUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	next := make(map[string]bool, len(roleIDs))
	for _, rid := range roleIDs {
		rid = strings.TrimSpace(rid)
		if rid != "" {
			next[rid] = true
		}
	}
	m.userRoles[userID] = next
	return nil
}

func (m *MemoryAuthRepository) GetUserRoles(ctx context.Context, userID string) ([]Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getUserRolesLocked(userID), nil
}

func (m *MemoryAuthRepository) CountUsersByRoleID(ctx context.Context, roleID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.countUsersByRoleLocked(roleID), nil
}

func (m *MemoryAuthRepository) CountActiveUsersWithRoleSlug(ctx context.Context, slug string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	roleID, ok := m.rolesBySlug[slug]
	if !ok {
		return 0, nil
	}
	count := 0
	for uid, u := range m.users {
		if u.Status == StatusActive && m.userRoles[uid] != nil && m.userRoles[uid][roleID] {
			count++
		}
	}
	return count, nil
}

func (m *MemoryAuthRepository) countUsersByRoleLocked(roleID string) int {
	count := 0
	for _, roles := range m.userRoles {
		if roles[roleID] {
			count++
		}
	}
	return count
}

func (m *MemoryAuthRepository) getUserRolesLocked(userID string) []Role {
	roleIDs := m.userRoles[userID]
	var roles []Role
	for rid := range roleIDs {
		if r, ok := m.roles[rid]; ok {
			clone := *r
			clone.IsSystem = isSystemRoleSlug(r.Slug)
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

func (m *MemoryAuthRepository) ReplaceRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	next := make(map[string]bool, len(permissionIDs))
	for _, pid := range permissionIDs {
		pid = strings.TrimSpace(pid)
		if pid != "" {
			next[pid] = true
		}
	}
	m.rolePerms[roleID] = next
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
	if clone.Kind == "" {
		clone.Kind = TokenKindAPI
	}
	if clone.TokenType == "" {
		clone.TokenType = "custom"
	}
	clone.Permissions = append([]string(nil), token.Permissions...)
	m.apiTokens[token.ID] = &clone
	m.tokensByHash[token.TokenHash] = token.ID
	return nil
}

func (m *MemoryAuthRepository) GetByID(ctx context.Context, id string) (*APIToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, ok := m.apiTokens[id]
	if !ok {
		return nil, ErrTokenNotFound
	}
	clone := *t
	clone.Permissions = append([]string(nil), t.Permissions...)
	return &clone, nil
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
	clone.Permissions = append([]string(nil), t.Permissions...)
	return &clone, nil
}

func (m *MemoryAuthRepository) ListTokensByUserID(ctx context.Context, userID string) ([]*APIToken, error) {
	return m.ListTokensByKind(ctx, userID, "", false)
}

func (m *MemoryAuthRepository) ListTokensByKind(ctx context.Context, userID string, kind TokenKind, includeAll bool) ([]*APIToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*APIToken
	for _, t := range m.apiTokens {
		if !includeAll && t.UserID != userID {
			continue
		}
		if kind != "" && t.Kind != kind {
			continue
		}
		clone := *t
		clone.Permissions = append([]string(nil), t.Permissions...)
		result = append(result, &clone)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (m *MemoryAuthRepository) UpdateToken(ctx context.Context, token *APIToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.apiTokens[token.ID]
	if !ok {
		return ErrTokenNotFound
	}
	if existing.TokenHash != token.TokenHash {
		delete(m.tokensByHash, existing.TokenHash)
		m.tokensByHash[token.TokenHash] = token.ID
	}
	clone := *token
	clone.Permissions = append([]string(nil), token.Permissions...)
	m.apiTokens[token.ID] = &clone
	return nil
}

func (m *MemoryAuthRepository) RevokeToken(ctx context.Context, id string, revokedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.apiTokens[id]
	if !ok {
		return ErrTokenNotFound
	}
	t.RevokedAt = &revokedAt
	return nil
}

func (m *MemoryAuthRepository) DeleteToken(ctx context.Context, id, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.apiTokens[id]
	if !ok || (userID != "" && t.UserID != userID) {
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

// --- Invitations ---

func (m *MemoryAuthRepository) CreateInvitation(ctx context.Context, inv *AdminInvitation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clone := *inv
	clone.RoleIDs = append([]string(nil), inv.RoleIDs...)
	m.invitations[inv.ID] = &clone
	m.invitationsByToken[inv.TokenHash] = inv.ID
	return nil
}

func (m *MemoryAuthRepository) GetInvitationByID(ctx context.Context, id string) (*AdminInvitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	inv, ok := m.invitations[id]
	if !ok {
		return nil, ErrInvitationNotFound
	}
	clone := *inv
	clone.RoleIDs = append([]string(nil), inv.RoleIDs...)
	return &clone, nil
}

func (m *MemoryAuthRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*AdminInvitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.invitationsByToken[tokenHash]
	if !ok {
		return nil, ErrInvitationNotFound
	}
	inv, ok := m.invitations[id]
	if !ok {
		return nil, ErrInvitationNotFound
	}
	clone := *inv
	clone.RoleIDs = append([]string(nil), inv.RoleIDs...)
	return &clone, nil
}

func (m *MemoryAuthRepository) ListInvitations(ctx context.Context) ([]*AdminInvitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*AdminInvitation, 0, len(m.invitations))
	for _, inv := range m.invitations {
		clone := *inv
		clone.RoleIDs = append([]string(nil), inv.RoleIDs...)
		list = append(list, &clone)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

func (m *MemoryAuthRepository) UpdateInvitation(ctx context.Context, inv *AdminInvitation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.invitations[inv.ID]
	if !ok {
		return ErrInvitationNotFound
	}
	if existing.TokenHash != inv.TokenHash {
		delete(m.invitationsByToken, existing.TokenHash)
		m.invitationsByToken[inv.TokenHash] = inv.ID
	}
	clone := *inv
	clone.RoleIDs = append([]string(nil), inv.RoleIDs...)
	m.invitations[inv.ID] = &clone
	return nil
}

func (m *MemoryAuthRepository) DeleteInvitation(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.invitations[id]
	if !ok {
		return ErrInvitationNotFound
	}
	delete(m.invitationsByToken, existing.TokenHash)
	delete(m.invitations, id)
	return nil
}

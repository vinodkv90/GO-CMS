package usersperm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-cms/cms/internal/database/interfaces"
)

// Repository manages persistence for end-user roles, application users, and OAuth/SSO providers.
type Repository interface {
	// App Roles
	ListRoles(ctx context.Context) ([]AppRole, error)
	GetRoleByID(ctx context.Context, id string) (*AppRole, error)
	GetRoleBySlug(ctx context.Context, slug string) (*AppRole, error)
	GetDefaultRole(ctx context.Context) (*AppRole, error)
	CreateRole(ctx context.Context, role *AppRole) error
	UpdateRole(ctx context.Context, role *AppRole) error
	DeleteRole(ctx context.Context, id string) error
	CountUsersByRole(ctx context.Context, roleID string) (int, error)

	// App Users
	ListUsers(ctx context.Context, filter AppUserListFilter) ([]AppUser, int, error)
	GetUserByID(ctx context.Context, id string) (*AppUser, error)
	GetUserByEmail(ctx context.Context, email string) (*AppUser, error)
	GetUserByUsername(ctx context.Context, username string) (*AppUser, error)
	GetUserByConfirmationTokenHash(ctx context.Context, hash string) (*AppUser, error)
	GetUserByResetTokenHash(ctx context.Context, hash string) (*AppUser, error)
	CreateUser(ctx context.Context, user *AppUser) error
	UpdateUser(ctx context.Context, user *AppUser) error
	DeleteUser(ctx context.Context, id string) error

	// OAuth / SSO Providers
	ListProviders(ctx context.Context, scope string) ([]OAuthProvider, error)
	GetProviderByName(ctx context.Context, name string) (*OAuthProvider, error)
	UpsertProvider(ctx context.Context, provider *OAuthProvider) error
	DeleteProvider(ctx context.Context, name string) error
}

// MemoryRepository stores isolated app users, roles, and OAuth providers in memory.
type MemoryRepository struct {
	mu        sync.RWMutex
	roles     map[string]*AppRole
	users     map[string]*AppUser
	providers map[string]*OAuthProvider
}

// NewMemoryRepository creates an in-memory Repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		roles:     make(map[string]*AppRole),
		users:     make(map[string]*AppUser),
		providers: make(map[string]*OAuthProvider),
	}
}

func clonePermissions(src map[string]map[string]bool) map[string]map[string]bool {
	out := make(map[string]map[string]bool, len(src))
	for res, actions := range src {
		actCopy := make(map[string]bool, len(actions))
		for act, allowed := range actions {
			actCopy[act] = allowed
		}
		out[res] = actCopy
	}
	return out
}

func cloneAppRole(r *AppRole) *AppRole {
	if r == nil {
		return nil
	}
	c := *r
	c.Permissions = clonePermissions(r.Permissions)
	return &c
}

func cloneAppUser(u *AppUser) *AppUser {
	if u == nil {
		return nil
	}
	c := *u
	if u.Role != nil {
		c.Role = cloneAppRole(u.Role)
	}
	return &c
}

func cloneOAuthProvider(p *OAuthProvider) *OAuthProvider {
	if p == nil {
		return nil
	}
	c := *p
	if p.Config != nil {
		cfgCopy := make(map[string]any, len(p.Config))
		for k, v := range p.Config {
			cfgCopy[k] = v
		}
		c.Config = cfgCopy
	}
	return &c
}

func (m *MemoryRepository) ListRoles(ctx context.Context) ([]AppRole, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	counts := make(map[string]int)
	for _, u := range m.users {
		if u.RoleID != "" {
			counts[u.RoleID]++
		}
	}
	out := make([]AppRole, 0, len(m.roles))
	for _, r := range m.roles {
		c := cloneAppRole(r)
		c.UserCount = counts[r.ID]
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (m *MemoryRepository) GetRoleByID(ctx context.Context, id string) (*AppRole, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.roles[id]
	if !ok {
		return nil, ErrAppRoleNotFound
	}
	return cloneAppRole(r), nil
}

func (m *MemoryRepository) GetRoleBySlug(ctx context.Context, slug string) (*AppRole, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.roles {
		if strings.EqualFold(r.Slug, slug) {
			return cloneAppRole(r), nil
		}
	}
	return nil, ErrAppRoleNotFound
}

func (m *MemoryRepository) GetDefaultRole(ctx context.Context) (*AppRole, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.roles {
		if r.IsDefault {
			return cloneAppRole(r), nil
		}
	}
	for _, r := range m.roles {
		if r.Slug == "authenticated" {
			return cloneAppRole(r), nil
		}
	}
	return nil, ErrAppRoleNotFound
}

func (m *MemoryRepository) CreateRole(ctx context.Context, role *AppRole) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.roles {
		if strings.EqualFold(existing.Slug, role.Slug) {
			return ErrAppRoleExists
		}
	}
	if role.IsDefault {
		for _, existing := range m.roles {
			existing.IsDefault = false
		}
	}
	m.roles[role.ID] = cloneAppRole(role)
	return nil
}

func (m *MemoryRepository) UpdateRole(ctx context.Context, role *AppRole) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.roles[role.ID]; !ok {
		return ErrAppRoleNotFound
	}
	if role.IsDefault {
		for id, existing := range m.roles {
			if id != role.ID {
				existing.IsDefault = false
			}
		}
	}
	m.roles[role.ID] = cloneAppRole(role)
	return nil
}

func (m *MemoryRepository) DeleteRole(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.roles[id]; !ok {
		return ErrAppRoleNotFound
	}
	delete(m.roles, id)
	return nil
}

func (m *MemoryRepository) CountUsersByRole(ctx context.Context, roleID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, u := range m.users {
		if u.RoleID == roleID {
			count++
		}
	}
	return count, nil
}

func (m *MemoryRepository) hydrateUserLocked(u *AppUser) *AppUser {
	c := cloneAppUser(u)
	if c.RoleID != "" {
		if r, ok := m.roles[c.RoleID]; ok {
			c.Role = cloneAppRole(r)
		}
	}
	return c
}

func (m *MemoryRepository) ListUsers(ctx context.Context, filter AppUserListFilter) ([]AppUser, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	search := strings.ToLower(strings.TrimSpace(filter.Search))
	matched := make([]AppUser, 0, len(m.users))
	for _, u := range m.users {
		if filter.RoleID != "" && u.RoleID != filter.RoleID {
			continue
		}
		if search != "" {
			if !strings.Contains(strings.ToLower(u.Username), search) &&
				!strings.Contains(strings.ToLower(u.Email), search) {
				continue
			}
		}
		matched = append(matched, *m.hydrateUserLocked(u))
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})
	total := len(matched)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []AppUser{}, total, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matched[offset:end], total, nil
}

func (m *MemoryRepository) GetUserByID(ctx context.Context, id string) (*AppUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return nil, ErrAppUserNotFound
	}
	return m.hydrateUserLocked(u), nil
}

func (m *MemoryRepository) GetUserByEmail(ctx context.Context, email string) (*AppUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) {
			return m.hydrateUserLocked(u), nil
		}
	}
	return nil, ErrAppUserNotFound
}

func (m *MemoryRepository) GetUserByUsername(ctx context.Context, username string) (*AppUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if strings.EqualFold(u.Username, username) {
			return m.hydrateUserLocked(u), nil
		}
	}
	return nil, ErrAppUserNotFound
}

func (m *MemoryRepository) GetUserByConfirmationTokenHash(ctx context.Context, hash string) (*AppUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.ConfirmationTokenHash != "" && u.ConfirmationTokenHash == hash {
			return m.hydrateUserLocked(u), nil
		}
	}
	return nil, ErrAppUserNotFound
}

func (m *MemoryRepository) GetUserByResetTokenHash(ctx context.Context, hash string) (*AppUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.ResetTokenHash != "" && u.ResetTokenHash == hash {
			return m.hydrateUserLocked(u), nil
		}
	}
	return nil, ErrAppUserNotFound
}

func (m *MemoryRepository) CreateUser(ctx context.Context, user *AppUser) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.users {
		if strings.EqualFold(existing.Email, user.Email) || strings.EqualFold(existing.Username, user.Username) {
			return ErrAppUserExists
		}
	}
	m.users[user.ID] = cloneAppUser(user)
	return nil
}

func (m *MemoryRepository) UpdateUser(ctx context.Context, user *AppUser) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[user.ID]; !ok {
		return ErrAppUserNotFound
	}
	for id, existing := range m.users {
		if id != user.ID && (strings.EqualFold(existing.Email, user.Email) || strings.EqualFold(existing.Username, user.Username)) {
			return ErrAppUserExists
		}
	}
	m.users[user.ID] = cloneAppUser(user)
	return nil
}

func (m *MemoryRepository) DeleteUser(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[id]; !ok {
		return ErrAppUserNotFound
	}
	delete(m.users, id)
	return nil
}

func (m *MemoryRepository) ListProviders(ctx context.Context, scope string) ([]OAuthProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]OAuthProvider, 0, len(m.providers))
	for _, p := range m.providers {
		if scope != "" && !strings.EqualFold(p.Scope, scope) {
			continue
		}
		out = append(out, *cloneOAuthProvider(p))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (m *MemoryRepository) GetProviderByName(ctx context.Context, name string) (*OAuthProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, ErrProviderNotFound
	}
	return cloneOAuthProvider(p), nil
}

func (m *MemoryRepository) UpsertProvider(ctx context.Context, provider *OAuthProvider) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(strings.TrimSpace(provider.Name))
	provider.Name = key
	m.providers[key] = cloneOAuthProvider(provider)
	return nil
}

func (m *MemoryRepository) DeleteProvider(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(strings.TrimSpace(name))
	if _, ok := m.providers[key]; !ok {
		return ErrProviderNotFound
	}
	delete(m.providers, key)
	return nil
}

// SQLRepository implements Repository against PostgreSQL or MySQL cms_app_roles, cms_app_users, and cms_oauth_providers.
type SQLRepository struct {
	db interfaces.SQLDatabase
}

// NewSQLRepository creates a SQL-backed Repository.
func NewSQLRepository(db interfaces.SQLDatabase) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) rebind(query string) string {
	if r.db.DriverName() != "postgres" {
		return query
	}
	var sb strings.Builder
	idx := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			sb.WriteByte('$')
			sb.WriteString(strconv.Itoa(idx))
			idx++
		} else {
			sb.WriteByte(query[i])
		}
	}
	return sb.String()
}

func nullStr(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func nullTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return *t
}

func scanRoleRow(scan func(dest ...any) error) (*AppRole, error) {
	var role AppRole
	var permsRaw []byte
	if err := scan(&role.ID, &role.Name, &role.Slug, &role.Description, &role.Type, &role.IsDefault, &permsRaw, &role.CreatedAt, &role.UpdatedAt); err != nil {
		return nil, err
	}
	if len(permsRaw) > 0 {
		_ = json.Unmarshal(permsRaw, &role.Permissions)
	}
	if role.Permissions == nil {
		role.Permissions = map[string]map[string]bool{}
	}
	return &role, nil
}

func (r *SQLRepository) ListRoles(ctx context.Context) ([]AppRole, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.name, r.slug, r.description, r.type, r.is_default, r.permissions, r.created_at, r.updated_at, COUNT(u.id)
		FROM cms_app_roles r
		LEFT JOIN cms_app_users u ON u.role_id = r.id
		GROUP BY r.id, r.name, r.slug, r.description, r.type, r.is_default, r.permissions, r.created_at, r.updated_at
		ORDER BY r.name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AppRole
	for rows.Next() {
		var role AppRole
		var permsRaw []byte
		if err := rows.Scan(&role.ID, &role.Name, &role.Slug, &role.Description, &role.Type, &role.IsDefault, &permsRaw, &role.CreatedAt, &role.UpdatedAt, &role.UserCount); err != nil {
			return nil, err
		}
		if len(permsRaw) > 0 {
			_ = json.Unmarshal(permsRaw, &role.Permissions)
		}
		if role.Permissions == nil {
			role.Permissions = map[string]map[string]bool{}
		}
		out = append(out, role)
	}
	if out == nil {
		out = []AppRole{}
	}
	return out, rows.Err()
}

func (r *SQLRepository) GetRoleByID(ctx context.Context, id string) (*AppRole, error) {
	q := r.rebind(`SELECT id, name, slug, description, type, is_default, permissions, created_at, updated_at FROM cms_app_roles WHERE id = ? LIMIT 1`)
	role, err := scanRoleRow(r.db.QueryRowContext(ctx, q, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAppRoleNotFound
	}
	return role, err
}

func (r *SQLRepository) GetRoleBySlug(ctx context.Context, slug string) (*AppRole, error) {
	q := r.rebind(`SELECT id, name, slug, description, type, is_default, permissions, created_at, updated_at FROM cms_app_roles WHERE LOWER(slug) = LOWER(?) LIMIT 1`)
	role, err := scanRoleRow(r.db.QueryRowContext(ctx, q, slug).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAppRoleNotFound
	}
	return role, err
}

func (r *SQLRepository) GetDefaultRole(ctx context.Context) (*AppRole, error) {
	q := r.rebind(`SELECT id, name, slug, description, type, is_default, permissions, created_at, updated_at FROM cms_app_roles WHERE is_default = ? LIMIT 1`)
	role, err := scanRoleRow(r.db.QueryRowContext(ctx, q, true).Scan)
	if err == nil {
		return role, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return r.GetRoleBySlug(ctx, "authenticated")
}

func (r *SQLRepository) CreateRole(ctx context.Context, role *AppRole) error {
	if role.IsDefault {
		_, _ = r.db.ExecContext(ctx, r.rebind(`UPDATE cms_app_roles SET is_default = ?`), false)
	}
	permsJSON, err := json.Marshal(role.Permissions)
	if err != nil {
		return err
	}
	q := r.rebind(`INSERT INTO cms_app_roles (id, name, slug, description, type, is_default, permissions, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err = r.db.ExecContext(ctx, q, role.ID, role.Name, role.Slug, role.Description, role.Type, role.IsDefault, string(permsJSON), role.CreatedAt, role.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAppRoleExists
		}
		return fmt.Errorf("failed to create app role: %w", err)
	}
	return nil
}

func (r *SQLRepository) UpdateRole(ctx context.Context, role *AppRole) error {
	if role.IsDefault {
		_, _ = r.db.ExecContext(ctx, r.rebind(`UPDATE cms_app_roles SET is_default = ? WHERE id != ?`), false, role.ID)
	}
	permsJSON, err := json.Marshal(role.Permissions)
	if err != nil {
		return err
	}
	q := r.rebind(`UPDATE cms_app_roles SET name = ?, description = ?, is_default = ?, permissions = ?, updated_at = ? WHERE id = ?`)
	res, err := r.db.ExecContext(ctx, q, role.Name, role.Description, role.IsDefault, string(permsJSON), role.UpdatedAt, role.ID)
	if err != nil {
		return fmt.Errorf("failed to update app role: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrAppRoleNotFound
	}
	return nil
}

func (r *SQLRepository) DeleteRole(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.rebind(`DELETE FROM cms_app_roles WHERE id = ?`), id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrAppRoleNotFound
	}
	return nil
}

func (r *SQLRepository) CountUsersByRole(ctx context.Context, roleID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, r.rebind(`SELECT COUNT(*) FROM cms_app_users WHERE role_id = ?`), roleID).Scan(&count)
	return count, err
}

const appUserSelectCols = `id, username, email, password_hash, provider, confirmed, blocked, confirmation_token_hash, confirmation_expires_at, reset_token_hash, reset_expires_at, role_id, last_login_at, created_at, updated_at`

func (r *SQLRepository) scanAppUser(ctx context.Context, scan func(dest ...any) error) (*AppUser, error) {
	var u AppUser
	var confHash, resetHash, roleID sql.NullString
	var confExp, resetExp, lastLogin sql.NullTime
	err := scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Provider, &u.Confirmed, &u.Blocked,
		&confHash, &confExp, &resetHash, &resetExp, &roleID, &lastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	u.ConfirmationTokenHash = confHash.String
	u.ResetTokenHash = resetHash.String
	u.RoleID = roleID.String
	if confExp.Valid {
		t := confExp.Time
		u.ConfirmationExpiresAt = &t
	}
	if resetExp.Valid {
		t := resetExp.Time
		u.ResetExpiresAt = &t
	}
	if lastLogin.Valid {
		t := lastLogin.Time
		u.LastLoginAt = &t
	}
	if u.RoleID != "" {
		if role, rErr := r.GetRoleByID(ctx, u.RoleID); rErr == nil {
			u.Role = role
		}
	}
	return &u, nil
}

func (r *SQLRepository) ListUsers(ctx context.Context, filter AppUserListFilter) ([]AppUser, int, error) {
	var where []string
	var args []any
	if filter.RoleID != "" {
		where = append(where, "role_id = ?")
		args = append(args, filter.RoleID)
	}
	if s := strings.ToLower(strings.TrimSpace(filter.Search)); s != "" {
		where = append(where, "(LOWER(username) LIKE ? OR LOWER(email) LIKE ?)")
		pat := "%" + s + "%"
		args = append(args, pat, pat)
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	countQ := r.rebind(`SELECT COUNT(*) FROM cms_app_users` + whereClause)
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := r.rebind(`SELECT ` + appUserSelectCols + ` FROM cms_app_users` + whereClause + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`)
	queryArgs := append(append([]any{}, args...), limit, offset)
	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	roleCache := map[string]*AppRole{}
	if roles, rErr := r.ListRoles(ctx); rErr == nil {
		for i := range roles {
			rc := roles[i]
			roleCache[rc.ID] = &rc
		}
	}

	out := make([]AppUser, 0)
	for rows.Next() {
		var u AppUser
		var confHash, resetHash, roleID sql.NullString
		var confExp, resetExp, lastLogin sql.NullTime
		if err := rows.Scan(
			&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Provider, &u.Confirmed, &u.Blocked,
			&confHash, &confExp, &resetHash, &resetExp, &roleID, &lastLogin, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		u.ConfirmationTokenHash = confHash.String
		u.ResetTokenHash = resetHash.String
		u.RoleID = roleID.String
		if confExp.Valid {
			t := confExp.Time
			u.ConfirmationExpiresAt = &t
		}
		if resetExp.Valid {
			t := resetExp.Time
			u.ResetExpiresAt = &t
		}
		if lastLogin.Valid {
			t := lastLogin.Time
			u.LastLoginAt = &t
		}
		if role, ok := roleCache[u.RoleID]; ok {
			u.Role = role
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

func (r *SQLRepository) querySingleUser(ctx context.Context, whereSQL string, arg any) (*AppUser, error) {
	q := r.rebind(`SELECT ` + appUserSelectCols + ` FROM cms_app_users WHERE ` + whereSQL + ` LIMIT 1`)
	u, err := r.scanAppUser(ctx, r.db.QueryRowContext(ctx, q, arg).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAppUserNotFound
	}
	return u, err
}

func (r *SQLRepository) GetUserByID(ctx context.Context, id string) (*AppUser, error) {
	return r.querySingleUser(ctx, "id = ?", id)
}

func (r *SQLRepository) GetUserByEmail(ctx context.Context, email string) (*AppUser, error) {
	return r.querySingleUser(ctx, "LOWER(email) = LOWER(?)", email)
}

func (r *SQLRepository) GetUserByUsername(ctx context.Context, username string) (*AppUser, error) {
	return r.querySingleUser(ctx, "LOWER(username) = LOWER(?)", username)
}

func (r *SQLRepository) GetUserByConfirmationTokenHash(ctx context.Context, hash string) (*AppUser, error) {
	return r.querySingleUser(ctx, "confirmation_token_hash = ?", hash)
}

func (r *SQLRepository) GetUserByResetTokenHash(ctx context.Context, hash string) (*AppUser, error) {
	return r.querySingleUser(ctx, "reset_token_hash = ?", hash)
}

func (r *SQLRepository) CreateUser(ctx context.Context, u *AppUser) error {
	q := r.rebind(`
		INSERT INTO cms_app_users (
			id, username, email, password_hash, provider, confirmed, blocked,
			confirmation_token_hash, confirmation_expires_at, reset_token_hash, reset_expires_at,
			role_id, last_login_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	_, err := r.db.ExecContext(ctx, q,
		u.ID, u.Username, u.Email, u.PasswordHash, u.Provider, u.Confirmed, u.Blocked,
		nullStr(u.ConfirmationTokenHash), nullTimePtr(u.ConfirmationExpiresAt), nullStr(u.ResetTokenHash), nullTimePtr(u.ResetExpiresAt),
		nullStr(u.RoleID), nullTimePtr(u.LastLoginAt), u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAppUserExists
		}
		return fmt.Errorf("failed to create app user: %w", err)
	}
	return nil
}

func (r *SQLRepository) UpdateUser(ctx context.Context, u *AppUser) error {
	q := r.rebind(`
		UPDATE cms_app_users
		SET username = ?, email = ?, password_hash = ?, provider = ?, confirmed = ?, blocked = ?,
		    confirmation_token_hash = ?, confirmation_expires_at = ?, reset_token_hash = ?, reset_expires_at = ?,
		    role_id = ?, last_login_at = ?, updated_at = ?
		WHERE id = ?
	`)
	res, err := r.db.ExecContext(ctx, q,
		u.Username, u.Email, u.PasswordHash, u.Provider, u.Confirmed, u.Blocked,
		nullStr(u.ConfirmationTokenHash), nullTimePtr(u.ConfirmationExpiresAt), nullStr(u.ResetTokenHash), nullTimePtr(u.ResetExpiresAt),
		nullStr(u.RoleID), nullTimePtr(u.LastLoginAt), u.UpdatedAt, u.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAppUserExists
		}
		return fmt.Errorf("failed to update app user: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrAppUserNotFound
	}
	return nil
}

func (r *SQLRepository) DeleteUser(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.rebind(`DELETE FROM cms_app_users WHERE id = ?`), id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrAppUserNotFound
	}
	return nil
}

func scanProviderRow(scan func(dest ...any) error) (*OAuthProvider, error) {
	var p OAuthProvider
	var cfgRaw []byte
	if err := scan(
		&p.ID, &p.Name, &p.DisplayName, &p.ProviderType, &p.Scope, &p.Enabled,
		&p.ClientID, &p.ClientSecretEnc, &p.AuthURL, &p.TokenURL, &p.UserinfoURL,
		&p.IssuerURL, &p.RedirectURI, &p.Scopes, &cfgRaw, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(cfgRaw) > 0 {
		_ = json.Unmarshal(cfgRaw, &p.Config)
	}
	return &p, nil
}

func (r *SQLRepository) ListProviders(ctx context.Context, scope string) ([]OAuthProvider, error) {
	var q string
	var args []any
	const cols = `id, name, display_name, provider_type, scope, enabled, client_id, client_secret_enc, auth_url, token_url, userinfo_url, issuer_url, redirect_uri, scopes, config, created_at, updated_at`
	if scope != "" {
		q = r.rebind(`SELECT ` + cols + ` FROM cms_oauth_providers WHERE LOWER(scope) = LOWER(?) ORDER BY name ASC`)
		args = append(args, scope)
	} else {
		q = `SELECT ` + cols + ` FROM cms_oauth_providers ORDER BY scope ASC, name ASC`
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OAuthProvider
	for rows.Next() {
		p, err := scanProviderRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if out == nil {
		out = []OAuthProvider{}
	}
	return out, rows.Err()
}

func (r *SQLRepository) GetProviderByName(ctx context.Context, name string) (*OAuthProvider, error) {
	q := r.rebind(`SELECT id, name, display_name, provider_type, scope, enabled, client_id, client_secret_enc, auth_url, token_url, userinfo_url, issuer_url, redirect_uri, scopes, config, created_at, updated_at FROM cms_oauth_providers WHERE LOWER(name) = LOWER(?) LIMIT 1`)
	p, err := scanProviderRow(r.db.QueryRowContext(ctx, q, strings.TrimSpace(name)).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProviderNotFound
	}
	return p, err
}

func (r *SQLRepository) UpsertProvider(ctx context.Context, p *OAuthProvider) error {
	cfgJSON, err := json.Marshal(p.Config)
	if err != nil {
		cfgJSON = []byte("{}")
	}
	if len(cfgJSON) == 0 || string(cfgJSON) == "null" {
		cfgJSON = []byte("{}")
	}

	var q string
	if r.db.DriverName() == "mysql" {
		q = `
			INSERT INTO cms_oauth_providers (
				id, name, display_name, provider_type, scope, enabled, client_id, client_secret_enc,
				auth_url, token_url, userinfo_url, issuer_url, redirect_uri, scopes, config, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				display_name = VALUES(display_name), provider_type = VALUES(provider_type), scope = VALUES(scope),
				enabled = VALUES(enabled), client_id = VALUES(client_id), client_secret_enc = VALUES(client_secret_enc),
				auth_url = VALUES(auth_url), token_url = VALUES(token_url), userinfo_url = VALUES(userinfo_url),
				issuer_url = VALUES(issuer_url), redirect_uri = VALUES(redirect_uri), scopes = VALUES(scopes),
				config = VALUES(config), updated_at = VALUES(updated_at)
		`
	} else {
		q = r.rebind(`
			INSERT INTO cms_oauth_providers (
				id, name, display_name, provider_type, scope, enabled, client_id, client_secret_enc,
				auth_url, token_url, userinfo_url, issuer_url, redirect_uri, scopes, config, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (name) DO UPDATE SET
				display_name = EXCLUDED.display_name, provider_type = EXCLUDED.provider_type, scope = EXCLUDED.scope,
				enabled = EXCLUDED.enabled, client_id = EXCLUDED.client_id, client_secret_enc = EXCLUDED.client_secret_enc,
				auth_url = EXCLUDED.auth_url, token_url = EXCLUDED.token_url, userinfo_url = EXCLUDED.userinfo_url,
				issuer_url = EXCLUDED.issuer_url, redirect_uri = EXCLUDED.redirect_uri, scopes = EXCLUDED.scopes,
				config = EXCLUDED.config, updated_at = EXCLUDED.updated_at
		`)
	}
	_, err = r.db.ExecContext(ctx, q,
		p.ID, strings.ToLower(strings.TrimSpace(p.Name)), p.DisplayName, p.ProviderType, p.Scope, p.Enabled,
		p.ClientID, p.ClientSecretEnc, p.AuthURL, p.TokenURL, p.UserinfoURL,
		p.IssuerURL, p.RedirectURI, p.Scopes, string(cfgJSON), p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (r *SQLRepository) DeleteProvider(ctx context.Context, name string) error {
	res, err := r.db.ExecContext(ctx, r.rebind(`DELETE FROM cms_oauth_providers WHERE LOWER(name) = LOWER(?)`), strings.TrimSpace(name))
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrProviderNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}


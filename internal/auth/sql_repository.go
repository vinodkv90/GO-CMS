package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/database/interfaces"
)

// SQLAuthRepository implements UserRepository, RoleRepository, PermissionRepository, and APITokenRepository.
type SQLAuthRepository struct {
	db     interfaces.SQLDatabase
	driver string
}

// NewSQLAuthRepository creates a new SQL-backed auth repository.
func NewSQLAuthRepository(db interfaces.SQLDatabase) *SQLAuthRepository {
	return &SQLAuthRepository{
		db:     db,
		driver: db.DriverName(),
	}
}

func (r *SQLAuthRepository) rebind(query string) string {
	if r.driver != "postgres" {
		return query
	}

	var b strings.Builder
	paramIndex := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			b.WriteString(fmt.Sprintf("$%d", paramIndex))
			paramIndex++
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

// --- UserRepository Implementation ---

const userSelectColumns = `id, username, email, password_hash, status, first_name, last_name, last_login_at, failed_login_attempts, locked_until, password_changed_at, mfa_enabled, mfa_secret_enc, mfa_backup_codes_hash, sso_provider, sso_subject, created_at, updated_at`

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func isSystemRoleSlug(slug string) bool {
	return slug == RoleSuperAdmin || slug == RoleEditor || slug == RoleAuthor || slug == RoleViewer
}

func (r *SQLAuthRepository) CreateUser(ctx context.Context, u *User) error {
	query := r.rebind(`
		INSERT INTO cms_users (
			id, username, email, password_hash, status, first_name, last_name,
			last_login_at, failed_login_attempts, locked_until, password_changed_at,
			mfa_enabled, mfa_secret_enc, mfa_backup_codes_hash, sso_provider, sso_subject,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	_, err := r.db.ExecContext(ctx, query,
		u.ID, u.Username, u.Email, u.PasswordHash, string(u.Status), u.FirstName, u.LastName,
		nullTimePtr(u.LastLoginAt), u.FailedLoginAttempts, nullTimePtr(u.LockedUntil), nullTimePtr(u.PasswordChangedAt),
		u.MFAEnabled, nullStr(u.MFASecretEnc), nullStr(u.MFABackupCodesHash), nullStr(u.SSOProvider), nullStr(u.SSOSubject),
		u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return ErrUserAlreadyExists
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (r *SQLAuthRepository) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := r.rebind(`SELECT ` + userSelectColumns + ` FROM cms_users WHERE id = ?`)
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanUser(ctx, row)
}

func (r *SQLAuthRepository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	query := r.rebind(`SELECT ` + userSelectColumns + ` FROM cms_users WHERE LOWER(username) = LOWER(?)`)
	row := r.db.QueryRowContext(ctx, query, username)
	return r.scanUser(ctx, row)
}

func (r *SQLAuthRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := r.rebind(`SELECT ` + userSelectColumns + ` FROM cms_users WHERE LOWER(email) = LOWER(?)`)
	row := r.db.QueryRowContext(ctx, query, email)
	return r.scanUser(ctx, row)
}

func (r *SQLAuthRepository) UpdateUser(ctx context.Context, u *User) error {
	query := r.rebind(`
		UPDATE cms_users
		SET email = ?, password_hash = ?, status = ?, first_name = ?, last_name = ?,
		    last_login_at = ?, failed_login_attempts = ?, locked_until = ?, password_changed_at = ?,
		    mfa_enabled = ?, mfa_secret_enc = ?, mfa_backup_codes_hash = ?,
		    sso_provider = ?, sso_subject = ?, updated_at = ?
		WHERE id = ?
	`)
	res, err := r.db.ExecContext(ctx, query,
		u.Email, u.PasswordHash, string(u.Status), u.FirstName, u.LastName,
		nullTimePtr(u.LastLoginAt), u.FailedLoginAttempts, nullTimePtr(u.LockedUntil), nullTimePtr(u.PasswordChangedAt),
		u.MFAEnabled, nullStr(u.MFASecretEnc), nullStr(u.MFABackupCodesHash),
		nullStr(u.SSOProvider), nullStr(u.SSOSubject), u.UpdatedAt, u.ID,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return ErrUserAlreadyExists
		}
		return fmt.Errorf("failed to update user: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *SQLAuthRepository) DeleteUser(ctx context.Context, id string) error {
	query := r.rebind(`DELETE FROM cms_users WHERE id = ?`)
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *SQLAuthRepository) ListUsers(ctx context.Context, limit, offset int) ([]*User, int, error) {
	return r.ListUsersFiltered(ctx, UserListFilter{Limit: limit, Offset: offset})
}

func (r *SQLAuthRepository) ListUsersFiltered(ctx context.Context, filter UserListFilter) ([]*User, int, error) {
	var where []string
	var args []any

	if search := strings.TrimSpace(strings.ToLower(filter.Search)); search != "" {
		like := "%" + search + "%"
		where = append(where, `(LOWER(username) LIKE ? OR LOWER(email) LIKE ? OR LOWER(first_name) LIKE ? OR LOWER(last_name) LIKE ?)`)
		args = append(args, like, like, like, like)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		where = append(where, `status = ?`)
		args = append(args, status)
	}
	if roleID := strings.TrimSpace(filter.RoleID); roleID != "" {
		where = append(where, `EXISTS (SELECT 1 FROM cms_user_roles ur WHERE ur.user_id = cms_users.id AND ur.role_id = ?)`)
		args = append(args, roleID)
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	countQuery := r.rebind(`SELECT COUNT(*) FROM cms_users` + whereSQL)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	queryArgs := append(append([]any{}, args...), limit, offset)
	query := r.rebind(`SELECT ` + userSelectColumns + ` FROM cms_users` + whereSQL + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`)
	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	var users []*User
	for rows.Next() {
		u, err := scanUserFields(rows)
		if err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, err
	}
	_ = rows.Close()

	for _, u := range users {
		roles, err := r.GetUserRoles(ctx, u.ID)
		if err == nil {
			u.Roles = roles
		}
	}
	return users, total, nil
}

func (r *SQLAuthRepository) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cms_users").Scan(&count)
	return count, err
}

func scanUserFields(scanner interface{ Scan(dest ...any) error }) (*User, error) {
	var u User
	var status string
	var fn, ln, mfaSec, mfaBackup, ssoProv, ssoSub sql.NullString
	var lastLogin, lockedUntil, pwChanged sql.NullTime
	if err := scanner.Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &status, &fn, &ln,
		&lastLogin, &u.FailedLoginAttempts, &lockedUntil, &pwChanged,
		&u.MFAEnabled, &mfaSec, &mfaBackup, &ssoProv, &ssoSub,
		&u.CreatedAt, &u.UpdatedAt,
	); err != nil {
		return nil, err
	}
	u.Status = UserStatus(status)
	u.FirstName = fn.String
	u.LastName = ln.String
	if lastLogin.Valid {
		t := lastLogin.Time
		u.LastLoginAt = &t
	}
	if lockedUntil.Valid {
		t := lockedUntil.Time
		u.LockedUntil = &t
	}
	if pwChanged.Valid {
		t := pwChanged.Time
		u.PasswordChangedAt = &t
	}
	u.MFASecretEnc = mfaSec.String
	u.MFABackupCodesHash = mfaBackup.String
	u.SSOProvider = ssoProv.String
	u.SSOSubject = ssoSub.String
	return &u, nil
}

func (r *SQLAuthRepository) scanUser(ctx context.Context, row *sql.Row) (*User, error) {
	u, err := scanUserFields(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	roles, err := r.GetUserRoles(ctx, u.ID)
	if err == nil {
		u.Roles = roles
	}

	return u, nil
}

// --- RoleRepository Implementation ---

func (r *SQLAuthRepository) CreateRole(ctx context.Context, role *Role) error {
	query := r.rebind(`
		INSERT INTO cms_roles (id, name, slug, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	_, err := r.db.ExecContext(ctx, query, role.ID, role.Name, role.Slug, role.Description, role.CreatedAt, role.UpdatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return ErrRoleAlreadyExists
		}
		return fmt.Errorf("failed to create role: %w", err)
	}
	return nil
}

func (r *SQLAuthRepository) GetRoleByID(ctx context.Context, id string) (*Role, error) {
	query := r.rebind(`SELECT id, name, slug, description, created_at, updated_at FROM cms_roles WHERE id = ?`)
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanRole(ctx, row)
}

func (r *SQLAuthRepository) GetRoleBySlug(ctx context.Context, slug string) (*Role, error) {
	query := r.rebind(`SELECT id, name, slug, description, created_at, updated_at FROM cms_roles WHERE slug = ?`)
	row := r.db.QueryRowContext(ctx, query, slug)
	return r.scanRole(ctx, row)
}

func (r *SQLAuthRepository) ListRoles(ctx context.Context) ([]*Role, error) {
	counts := make(map[string]int)
	if cntRows, err := r.db.QueryContext(ctx, `SELECT role_id, COUNT(*) FROM cms_user_roles GROUP BY role_id`); err == nil {
		for cntRows.Next() {
			var rid string
			var cnt int
			if scanErr := cntRows.Scan(&rid, &cnt); scanErr == nil {
				counts[rid] = cnt
			}
		}
		_ = cntRows.Close()
	}

	query := `SELECT id, name, slug, description, created_at, updated_at FROM cms_roles ORDER BY name ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query roles: %w", err)
	}

	var roles []*Role
	for rows.Next() {
		var role Role
		var desc sql.NullString
		if err := rows.Scan(&role.ID, &role.Name, &role.Slug, &desc, &role.CreatedAt, &role.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		role.Description = desc.String
		role.IsSystem = isSystemRoleSlug(role.Slug)
		role.UserCount = counts[role.ID]
		roles = append(roles, &role)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	for _, role := range roles {
		perms, _ := r.GetRolePermissions(ctx, role.ID)
		role.Permissions = perms
	}
	return roles, nil
}

func (r *SQLAuthRepository) UpdateRole(ctx context.Context, role *Role) error {
	query := r.rebind(`UPDATE cms_roles SET name = ?, slug = ?, description = ?, updated_at = ? WHERE id = ?`)
	res, err := r.db.ExecContext(ctx, query, role.Name, role.Slug, role.Description, role.UpdatedAt, role.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return ErrRoleAlreadyExists
		}
		return fmt.Errorf("failed to update role: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrRoleNotFound
	}
	return nil
}

func (r *SQLAuthRepository) DeleteRole(ctx context.Context, id string) error {
	query := r.rebind(`DELETE FROM cms_roles WHERE id = ?`)
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrRoleNotFound
	}
	return nil
}

func (r *SQLAuthRepository) AssignRoleToUser(ctx context.Context, userID, roleID string) error {
	query := r.rebind(`INSERT INTO cms_user_roles (user_id, role_id) VALUES (?, ?) ON CONFLICT DO NOTHING`)
	if r.driver == "mysql" {
		query = `INSERT IGNORE INTO cms_user_roles (user_id, role_id) VALUES (?, ?)`
	}
	_, err := r.db.ExecContext(ctx, query, userID, roleID)
	return err
}

func (r *SQLAuthRepository) RemoveRoleFromUser(ctx context.Context, userID, roleID string) error {
	query := r.rebind(`DELETE FROM cms_user_roles WHERE user_id = ? AND role_id = ?`)
	_, err := r.db.ExecContext(ctx, query, userID, roleID)
	return err
}

func (r *SQLAuthRepository) ReplaceUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, r.rebind(`DELETE FROM cms_user_roles WHERE user_id = ?`), userID); err != nil {
		return fmt.Errorf("failed to clear user roles: %w", err)
	}

	insertSQL := r.rebind(`INSERT INTO cms_user_roles (user_id, role_id) VALUES (?, ?) ON CONFLICT DO NOTHING`)
	if r.driver == "mysql" {
		insertSQL = `INSERT IGNORE INTO cms_user_roles (user_id, role_id) VALUES (?, ?)`
	}

	seen := make(map[string]struct{}, len(roleIDs))
	for _, rid := range roleIDs {
		rid = strings.TrimSpace(rid)
		if rid == "" {
			continue
		}
		if _, ok := seen[rid]; ok {
			continue
		}
		seen[rid] = struct{}{}
		if _, err := tx.ExecContext(ctx, insertSQL, userID, rid); err != nil {
			return fmt.Errorf("failed to assign role %s: %w", rid, err)
		}
	}

	return tx.Commit()
}

func (r *SQLAuthRepository) GetUserRoles(ctx context.Context, userID string) ([]Role, error) {
	query := r.rebind(`
		SELECT r.id, r.name, r.slug, r.description, r.created_at, r.updated_at
		FROM cms_roles r
		INNER JOIN cms_user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = ?
	`)
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}

	var roles []Role
	for rows.Next() {
		var role Role
		var desc sql.NullString
		if err := rows.Scan(&role.ID, &role.Name, &role.Slug, &desc, &role.CreatedAt, &role.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		role.Description = desc.String
		role.IsSystem = isSystemRoleSlug(role.Slug)
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	for i := range roles {
		perms, _ := r.GetRolePermissions(ctx, roles[i].ID)
		roles[i].Permissions = perms
	}
	return roles, nil
}

func (r *SQLAuthRepository) CountUsersByRoleID(ctx context.Context, roleID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, r.rebind(`SELECT COUNT(*) FROM cms_user_roles WHERE role_id = ?`), roleID).Scan(&count)
	return count, err
}

func (r *SQLAuthRepository) CountActiveUsersWithRoleSlug(ctx context.Context, slug string) (int, error) {
	query := r.rebind(`
		SELECT COUNT(DISTINCT u.id)
		FROM cms_users u
		INNER JOIN cms_user_roles ur ON ur.user_id = u.id
		INNER JOIN cms_roles r ON r.id = ur.role_id
		WHERE r.slug = ? AND u.status = 'active'
	`)
	var count int
	err := r.db.QueryRowContext(ctx, query, slug).Scan(&count)
	return count, err
}

func (r *SQLAuthRepository) scanRole(ctx context.Context, row *sql.Row) (*Role, error) {
	var role Role
	var desc sql.NullString
	if err := row.Scan(&role.ID, &role.Name, &role.Slug, &desc, &role.CreatedAt, &role.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}
	role.Description = desc.String
	role.IsSystem = isSystemRoleSlug(role.Slug)
	perms, _ := r.GetRolePermissions(ctx, role.ID)
	role.Permissions = perms
	cnt, err := r.CountUsersByRoleID(ctx, role.ID)
	if err == nil {
		role.UserCount = cnt
	}
	return &role, nil
}

// --- PermissionRepository Implementation ---

func (r *SQLAuthRepository) CreatePermission(ctx context.Context, p *Permission) error {
	query := r.rebind(`
		INSERT INTO cms_permissions (id, action, resource, description, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (action, resource) DO NOTHING
	`)
	if r.driver == "mysql" {
		query = `
		INSERT IGNORE INTO cms_permissions (id, action, resource, description, created_at)
		VALUES (?, ?, ?, ?, ?)
		`
	}
	_, err := r.db.ExecContext(ctx, query, p.ID, p.Action, p.Resource, p.Description, p.CreatedAt)
	return err
}

func (r *SQLAuthRepository) ListPermissions(ctx context.Context) ([]Permission, error) {
	query := `SELECT id, action, resource, description, created_at FROM cms_permissions ORDER BY resource, action ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []Permission
	for rows.Next() {
		var p Permission
		var desc sql.NullString
		if err := rows.Scan(&p.ID, &p.Action, &p.Resource, &desc, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Description = desc.String
		perms = append(perms, p)
	}
	return perms, nil
}

func (r *SQLAuthRepository) GetByActionAndResource(ctx context.Context, action, resource string) (*Permission, error) {
	query := r.rebind(`SELECT id, action, resource, description, created_at FROM cms_permissions WHERE action = ? AND resource = ?`)
	row := r.db.QueryRowContext(ctx, query, action, resource)
	var p Permission
	var desc sql.NullString
	if err := row.Scan(&p.ID, &p.Action, &p.Resource, &desc, &p.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPermissionNotFound
		}
		return nil, err
	}
	p.Description = desc.String
	return &p, nil
}

func (r *SQLAuthRepository) AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error {
	query := r.rebind(`INSERT INTO cms_role_permissions (role_id, permission_id) VALUES (?, ?) ON CONFLICT DO NOTHING`)
	if r.driver == "mysql" {
		query = `INSERT IGNORE INTO cms_role_permissions (role_id, permission_id) VALUES (?, ?)`
	}
	_, err := r.db.ExecContext(ctx, query, roleID, permissionID)
	return err
}

func (r *SQLAuthRepository) ReplaceRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, r.rebind(`DELETE FROM cms_role_permissions WHERE role_id = ?`), roleID); err != nil {
		return fmt.Errorf("failed to clear role permissions: %w", err)
	}

	insertSQL := r.rebind(`INSERT INTO cms_role_permissions (role_id, permission_id) VALUES (?, ?) ON CONFLICT DO NOTHING`)
	if r.driver == "mysql" {
		insertSQL = `INSERT IGNORE INTO cms_role_permissions (role_id, permission_id) VALUES (?, ?)`
	}

	seen := make(map[string]struct{}, len(permissionIDs))
	for _, pid := range permissionIDs {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		if _, err := tx.ExecContext(ctx, insertSQL, roleID, pid); err != nil {
			return fmt.Errorf("failed to assign permission %s: %w", pid, err)
		}
	}

	return tx.Commit()
}

func (r *SQLAuthRepository) GetRolePermissions(ctx context.Context, roleID string) ([]Permission, error) {
	query := r.rebind(`
		SELECT p.id, p.action, p.resource, p.description, p.created_at
		FROM cms_permissions p
		INNER JOIN cms_role_permissions rp ON rp.permission_id = p.id
		WHERE rp.role_id = ?
	`)
	rows, err := r.db.QueryContext(ctx, query, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []Permission
	for rows.Next() {
		var p Permission
		var desc sql.NullString
		if err := rows.Scan(&p.ID, &p.Action, &p.Resource, &desc, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Description = desc.String
		perms = append(perms, p)
	}
	return perms, nil
}

// --- APITokenRepository Implementation ---

const tokenSelectColumns = `id, name, kind, description, token_type, token_prefix, token_hash, user_id, permissions, expires_at, last_used_at, revoked_at, created_at`

func scanAPIToken(scanner interface{ Scan(dest ...any) error }) (*APIToken, error) {
	var t APIToken
	var kind, desc, tokType, tokPrefix sql.NullString
	var permsRaw []byte
	var exp, lastUsed, revokedAt sql.NullTime
	if err := scanner.Scan(
		&t.ID, &t.Name, &kind, &desc, &tokType, &tokPrefix, &t.TokenHash,
		&t.UserID, &permsRaw, &exp, &lastUsed, &revokedAt, &t.CreatedAt,
	); err != nil {
		return nil, err
	}
	t.Kind = TokenKind(kind.String)
	if t.Kind == "" {
		t.Kind = TokenKindAPI
	}
	t.Description = desc.String
	t.TokenType = tokType.String
	if t.TokenType == "" {
		t.TokenType = "custom"
	}
	t.TokenPrefix = tokPrefix.String
	if exp.Valid {
		v := exp.Time
		t.ExpiresAt = &v
	}
	if lastUsed.Valid {
		v := lastUsed.Time
		t.LastUsedAt = &v
	}
	if revokedAt.Valid {
		v := revokedAt.Time
		t.RevokedAt = &v
	}
	if len(permsRaw) > 0 {
		_ = json.Unmarshal(permsRaw, &t.Permissions)
	}
	if t.Permissions == nil {
		t.Permissions = []string{}
	}
	return &t, nil
}

func (r *SQLAuthRepository) CreateToken(ctx context.Context, t *APIToken) error {
	permsJSON, err := json.Marshal(t.Permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal token permissions: %w", err)
	}
	kind := string(t.Kind)
	if kind == "" {
		kind = string(TokenKindAPI)
	}
	tokType := t.TokenType
	if tokType == "" {
		tokType = "custom"
	}

	query := r.rebind(`
		INSERT INTO cms_api_tokens (
			id, name, kind, description, token_type, token_prefix,
			token_hash, user_id, permissions, expires_at, last_used_at, revoked_at, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	_, err = r.db.ExecContext(ctx, query,
		t.ID, t.Name, kind, t.Description, tokType, t.TokenPrefix,
		t.TokenHash, t.UserID, string(permsJSON),
		nullTimePtr(t.ExpiresAt), nullTimePtr(t.LastUsedAt), nullTimePtr(t.RevokedAt), t.CreatedAt,
	)
	return err
}

func (r *SQLAuthRepository) GetByID(ctx context.Context, id string) (*APIToken, error) {
	query := r.rebind(`SELECT ` + tokenSelectColumns + ` FROM cms_api_tokens WHERE id = ?`)
	row := r.db.QueryRowContext(ctx, query, id)
	t, err := scanAPIToken(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *SQLAuthRepository) GetByTokenHash(ctx context.Context, hash string) (*APIToken, error) {
	query := r.rebind(`SELECT ` + tokenSelectColumns + ` FROM cms_api_tokens WHERE token_hash = ?`)
	row := r.db.QueryRowContext(ctx, query, hash)
	t, err := scanAPIToken(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *SQLAuthRepository) ListTokensByUserID(ctx context.Context, userID string) ([]*APIToken, error) {
	return r.ListTokensByKind(ctx, userID, "", false)
}

func (r *SQLAuthRepository) ListTokensByKind(ctx context.Context, userID string, kind TokenKind, includeAll bool) ([]*APIToken, error) {
	var where []string
	var args []any
	if !includeAll {
		where = append(where, `user_id = ?`)
		args = append(args, userID)
	}
	if kind != "" {
		where = append(where, `kind = ?`)
		args = append(args, string(kind))
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	query := r.rebind(`SELECT ` + tokenSelectColumns + ` FROM cms_api_tokens` + whereSQL + ` ORDER BY created_at DESC`)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (r *SQLAuthRepository) UpdateToken(ctx context.Context, t *APIToken) error {
	permsJSON, err := json.Marshal(t.Permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal token permissions: %w", err)
	}
	query := r.rebind(`
		UPDATE cms_api_tokens
		SET name = ?, kind = ?, description = ?, token_type = ?, token_prefix = ?,
		    token_hash = ?, permissions = ?, expires_at = ?, revoked_at = ?
		WHERE id = ?
	`)
	res, err := r.db.ExecContext(ctx, query,
		t.Name, string(t.Kind), t.Description, t.TokenType, t.TokenPrefix,
		t.TokenHash, string(permsJSON), nullTimePtr(t.ExpiresAt), nullTimePtr(t.RevokedAt), t.ID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (r *SQLAuthRepository) RevokeToken(ctx context.Context, id string, revokedAt time.Time) error {
	query := r.rebind(`UPDATE cms_api_tokens SET revoked_at = ? WHERE id = ?`)
	res, err := r.db.ExecContext(ctx, query, revokedAt, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (r *SQLAuthRepository) DeleteToken(ctx context.Context, id, userID string) error {
	query := r.rebind(`DELETE FROM cms_api_tokens WHERE id = ?`)
	args := []any{id}
	if userID != "" {
		query = r.rebind(`DELETE FROM cms_api_tokens WHERE id = ? AND user_id = ?`)
		args = append(args, userID)
	}
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (r *SQLAuthRepository) UpdateLastUsed(ctx context.Context, id string, lastUsed time.Time) error {
	query := r.rebind(`UPDATE cms_api_tokens SET last_used_at = ? WHERE id = ?`)
	_, err := r.db.ExecContext(ctx, query, lastUsed, id)
	return err
}

// --- InvitationRepository Implementation ---

const invitationSelectColumns = `id, email, username, first_name, last_name, role_ids, token_hash, status, invited_by, expires_at, accepted_at, created_at, updated_at`

func (r *SQLAuthRepository) CreateInvitation(ctx context.Context, inv *AdminInvitation) error {
	roleIDsJSON, err := json.Marshal(inv.RoleIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal invitation roles: %w", err)
	}
	query := r.rebind(`
		INSERT INTO cms_admin_invitations (
			id, email, username, first_name, last_name, role_ids, token_hash,
			status, invited_by, expires_at, accepted_at, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	_, err = r.db.ExecContext(ctx, query,
		inv.ID, inv.Email, inv.Username, inv.FirstName, inv.LastName, string(roleIDsJSON),
		inv.TokenHash, string(inv.Status), nullStr(inv.InvitedBy), inv.ExpiresAt, nullTimePtr(inv.AcceptedAt),
		inv.CreatedAt, inv.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create invitation: %w", err)
	}
	return nil
}

func (r *SQLAuthRepository) GetInvitationByID(ctx context.Context, id string) (*AdminInvitation, error) {
	row := r.db.QueryRowContext(ctx, r.rebind(`SELECT `+invitationSelectColumns+` FROM cms_admin_invitations WHERE id = ?`), id)
	inv, err := scanInvitationRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvitationNotFound
		}
		return nil, err
	}
	return inv, nil
}

func (r *SQLAuthRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*AdminInvitation, error) {
	row := r.db.QueryRowContext(ctx, r.rebind(`SELECT `+invitationSelectColumns+` FROM cms_admin_invitations WHERE token_hash = ?`), tokenHash)
	inv, err := scanInvitationRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvitationNotFound
		}
		return nil, err
	}
	return inv, nil
}

func (r *SQLAuthRepository) ListInvitations(ctx context.Context) ([]*AdminInvitation, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+invitationSelectColumns+` FROM cms_admin_invitations ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list invitations: %w", err)
	}
	defer rows.Close()

	var list []*AdminInvitation
	for rows.Next() {
		inv, err := scanInvitationRow(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, inv)
	}
	return list, rows.Err()
}

func (r *SQLAuthRepository) UpdateInvitation(ctx context.Context, inv *AdminInvitation) error {
	roleIDsJSON, err := json.Marshal(inv.RoleIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal invitation roles: %w", err)
	}
	query := r.rebind(`
		UPDATE cms_admin_invitations
		SET email = ?, username = ?, first_name = ?, last_name = ?, role_ids = ?,
		    token_hash = ?, status = ?, invited_by = ?, expires_at = ?, accepted_at = ?, updated_at = ?
		WHERE id = ?
	`)
	res, err := r.db.ExecContext(ctx, query,
		inv.Email, inv.Username, inv.FirstName, inv.LastName, string(roleIDsJSON),
		inv.TokenHash, string(inv.Status), nullStr(inv.InvitedBy), inv.ExpiresAt, nullTimePtr(inv.AcceptedAt),
		inv.UpdatedAt, inv.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update invitation: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrInvitationNotFound
	}
	return nil
}

func (r *SQLAuthRepository) DeleteInvitation(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.rebind(`DELETE FROM cms_admin_invitations WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("failed to delete invitation: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrInvitationNotFound
	}
	return nil
}

func scanInvitationRow(scanner interface{ Scan(dest ...any) error }) (*AdminInvitation, error) {
	var inv AdminInvitation
	var status string
	var fn, ln, invitedBy sql.NullString
	var roleIDsRaw []byte
	var acceptedAt sql.NullTime
	if err := scanner.Scan(
		&inv.ID, &inv.Email, &inv.Username, &fn, &ln, &roleIDsRaw,
		&inv.TokenHash, &status, &invitedBy, &inv.ExpiresAt, &acceptedAt,
		&inv.CreatedAt, &inv.UpdatedAt,
	); err != nil {
		return nil, err
	}
	inv.Status = InvitationStatus(status)
	inv.FirstName = fn.String
	inv.LastName = ln.String
	inv.InvitedBy = invitedBy.String
	if acceptedAt.Valid {
		t := acceptedAt.Time
		inv.AcceptedAt = &t
	}
	if len(roleIDsRaw) > 0 {
		_ = json.Unmarshal(roleIDsRaw, &inv.RoleIDs)
	}
	if inv.RoleIDs == nil {
		inv.RoleIDs = []string{}
	}
	return &inv, nil
}

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

func (r *SQLAuthRepository) CreateUser(ctx context.Context, u *User) error {
	query := r.rebind(`
		INSERT INTO cms_users (id, username, email, password_hash, status, first_name, last_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	_, err := r.db.ExecContext(ctx, query,
		u.ID, u.Username, u.Email, u.PasswordHash, string(u.Status), u.FirstName, u.LastName, u.CreatedAt, u.UpdatedAt,
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
	query := r.rebind(`
		SELECT id, username, email, password_hash, status, first_name, last_name, created_at, updated_at
		FROM cms_users WHERE id = ?
	`)
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanUser(ctx, row)
}

func (r *SQLAuthRepository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	query := r.rebind(`
		SELECT id, username, email, password_hash, status, first_name, last_name, created_at, updated_at
		FROM cms_users WHERE LOWER(username) = LOWER(?)
	`)
	row := r.db.QueryRowContext(ctx, query, username)
	return r.scanUser(ctx, row)
}

func (r *SQLAuthRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := r.rebind(`
		SELECT id, username, email, password_hash, status, first_name, last_name, created_at, updated_at
		FROM cms_users WHERE LOWER(email) = LOWER(?)
	`)
	row := r.db.QueryRowContext(ctx, query, email)
	return r.scanUser(ctx, row)
}

func (r *SQLAuthRepository) UpdateUser(ctx context.Context, u *User) error {
	query := r.rebind(`
		UPDATE cms_users
		SET email = ?, status = ?, first_name = ?, last_name = ?, updated_at = ?
		WHERE id = ?
	`)
	res, err := r.db.ExecContext(ctx, query, u.Email, string(u.Status), u.FirstName, u.LastName, u.UpdatedAt, u.ID)
	if err != nil {
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
	countQuery := `SELECT COUNT(*) FROM cms_users`
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	query := r.rebind(`
		SELECT id, username, email, password_hash, status, first_name, last_name, created_at, updated_at
		FROM cms_users
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`)
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		var status string
		var fn, ln sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &status, &fn, &ln, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, err
		}
		u.Status = UserStatus(status)
		u.FirstName = fn.String
		u.LastName = ln.String

		roles, err := r.GetUserRoles(ctx, u.ID)
		if err == nil {
			u.Roles = roles
		}
		users = append(users, &u)
	}
	return users, total, nil
}

func (r *SQLAuthRepository) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cms_users").Scan(&count)
	return count, err
}

func (r *SQLAuthRepository) scanUser(ctx context.Context, row *sql.Row) (*User, error) {
	var u User
	var status string
	var fn, ln sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &status, &fn, &ln, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.Status = UserStatus(status)
	u.FirstName = fn.String
	u.LastName = ln.String

	roles, err := r.GetUserRoles(ctx, u.ID)
	if err == nil {
		u.Roles = roles
	}

	return &u, nil
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
	query := `SELECT id, name, slug, description, created_at, updated_at FROM cms_roles ORDER BY name ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query roles: %w", err)
	}
	defer rows.Close()

	var roles []*Role
	for rows.Next() {
		var role Role
		var desc sql.NullString
		if err := rows.Scan(&role.ID, &role.Name, &role.Slug, &desc, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		role.Description = desc.String
		perms, _ := r.GetRolePermissions(ctx, role.ID)
		role.Permissions = perms
		roles = append(roles, &role)
	}
	return roles, nil
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
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var role Role
		var desc sql.NullString
		if err := rows.Scan(&role.ID, &role.Name, &role.Slug, &desc, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		role.Description = desc.String
		perms, _ := r.GetRolePermissions(ctx, role.ID)
		role.Permissions = perms
		roles = append(roles, role)
	}
	return roles, nil
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
	perms, _ := r.GetRolePermissions(ctx, role.ID)
	role.Permissions = perms
	return &role, nil
}

// --- PermissionRepository Implementation ---

func (r *SQLAuthRepository) CreatePermission(ctx context.Context, p *Permission) error {
	query := r.rebind(`
		INSERT INTO cms_permissions (id, action, resource, description, created_at)
		VALUES (?, ?, ?, ?, ?)
	`)
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

func (r *SQLAuthRepository) CreateToken(ctx context.Context, t *APIToken) error {
	permsJSON, err := json.Marshal(t.Permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal token permissions: %w", err)
	}

	query := r.rebind(`
		INSERT INTO cms_api_tokens (id, name, token_hash, user_id, permissions, expires_at, last_used_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	_, err = r.db.ExecContext(ctx, query, t.ID, t.Name, t.TokenHash, t.UserID, string(permsJSON), t.ExpiresAt, t.LastUsedAt, t.CreatedAt)
	return err
}

func (r *SQLAuthRepository) GetByTokenHash(ctx context.Context, hash string) (*APIToken, error) {
	query := r.rebind(`
		SELECT id, name, token_hash, user_id, permissions, expires_at, last_used_at, created_at
		FROM cms_api_tokens
		WHERE token_hash = ?
	`)
	row := r.db.QueryRowContext(ctx, query, hash)
	var t APIToken
	var permsRaw []byte
	var exp, lastUsed sql.NullTime
	if err := row.Scan(&t.ID, &t.Name, &t.TokenHash, &t.UserID, &permsRaw, &exp, &lastUsed, &t.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	if exp.Valid {
		t.ExpiresAt = &exp.Time
	}
	if lastUsed.Valid {
		t.LastUsedAt = &lastUsed.Time
	}
	if len(permsRaw) > 0 {
		_ = json.Unmarshal(permsRaw, &t.Permissions)
	}
	return &t, nil
}

func (r *SQLAuthRepository) ListTokensByUserID(ctx context.Context, userID string) ([]*APIToken, error) {
	query := r.rebind(`
		SELECT id, name, token_hash, user_id, permissions, expires_at, last_used_at, created_at
		FROM cms_api_tokens
		WHERE user_id = ?
		ORDER BY created_at DESC
	`)
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		var t APIToken
		var permsRaw []byte
		var exp, lastUsed sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.TokenHash, &t.UserID, &permsRaw, &exp, &lastUsed, &t.CreatedAt); err != nil {
			return nil, err
		}
		if exp.Valid {
			t.ExpiresAt = &exp.Time
		}
		if lastUsed.Valid {
			t.LastUsedAt = &lastUsed.Time
		}
		if len(permsRaw) > 0 {
			_ = json.Unmarshal(permsRaw, &t.Permissions)
		}
		tokens = append(tokens, &t)
	}
	return tokens, nil
}

func (r *SQLAuthRepository) DeleteToken(ctx context.Context, id, userID string) error {
	query := r.rebind(`DELETE FROM cms_api_tokens WHERE id = ? AND user_id = ?`)
	res, err := r.db.ExecContext(ctx, query, id, userID)
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

package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/google/uuid"
)

// SQLRepository implements Repository against PostgreSQL and MySQL tables.
type SQLRepository struct {
	db interfaces.SQLDatabase
}

// NewSQLRepository creates a SQL-backed settings repository.
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

func (r *SQLRepository) GetSetting(ctx context.Context, key string) (*Record, error) {
	query := r.rebind(`SELECT id, key, value, is_public, updated_at FROM cms_settings WHERE key = ? LIMIT 1`)
	row := r.db.QueryRowContext(ctx, query, key)

	var rec Record
	var raw []byte
	err := row.Scan(&rec.ID, &rec.Key, &raw, &rec.IsPublic, &rec.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rec.Value = json.RawMessage(raw)
	return &rec, nil
}

func (r *SQLRepository) UpsertSetting(ctx context.Context, key string, value json.RawMessage, isPublic bool) (*Record, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	valStr := string(value)

	var query string
	if r.db.DriverName() == "mysql" {
		query = `INSERT INTO cms_settings (id, key, value, is_public, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE value = VALUES(value), is_public = VALUES(is_public), updated_at = VALUES(updated_at)`
	} else {
		query = `INSERT INTO cms_settings (id, key, value, is_public, updated_at)
			VALUES ($1, $2, $3::jsonb, $4, $5)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, is_public = EXCLUDED.is_public, updated_at = EXCLUDED.updated_at`
	}

	if _, err := r.db.ExecContext(ctx, query, id, key, valStr, isPublic, now); err != nil {
		return nil, err
	}
	return r.GetSetting(ctx, key)
}

func (r *SQLRepository) GetPreferences(ctx context.Context, userID string) (*UserPreferences, error) {
	query := r.rebind(`SELECT user_id, theme, preferred_language, timezone, date_format, time_format, sidebar_collapsed, updated_at FROM cms_user_preferences WHERE user_id = ? LIMIT 1`)
	row := r.db.QueryRowContext(ctx, query, userID)

	var prefs UserPreferences
	err := row.Scan(
		&prefs.UserID,
		&prefs.Theme,
		&prefs.PreferredLanguage,
		&prefs.Timezone,
		&prefs.DateFormat,
		&prefs.TimeFormat,
		&prefs.SidebarCollapsed,
		&prefs.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &prefs, nil
}

func (r *SQLRepository) UpsertPreferences(ctx context.Context, prefs *UserPreferences) (*UserPreferences, error) {
	now := time.Now().UTC()
	var query string
	if r.db.DriverName() == "mysql" {
		query = `INSERT INTO cms_user_preferences (user_id, theme, preferred_language, timezone, date_format, time_format, sidebar_collapsed, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE theme = VALUES(theme), preferred_language = VALUES(preferred_language), timezone = VALUES(timezone), date_format = VALUES(date_format), time_format = VALUES(time_format), sidebar_collapsed = VALUES(sidebar_collapsed), updated_at = VALUES(updated_at)`
	} else {
		query = `INSERT INTO cms_user_preferences (user_id, theme, preferred_language, timezone, date_format, time_format, sidebar_collapsed, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (user_id) DO UPDATE SET theme = EXCLUDED.theme, preferred_language = EXCLUDED.preferred_language, timezone = EXCLUDED.timezone, date_format = EXCLUDED.date_format, time_format = EXCLUDED.time_format, sidebar_collapsed = EXCLUDED.sidebar_collapsed, updated_at = EXCLUDED.updated_at`
	}

	if _, err := r.db.ExecContext(
		ctx,
		query,
		prefs.UserID,
		prefs.Theme,
		prefs.PreferredLanguage,
		prefs.Timezone,
		prefs.DateFormat,
		prefs.TimeFormat,
		prefs.SidebarCollapsed,
		now,
	); err != nil {
		return nil, err
	}
	return r.GetPreferences(ctx, prefs.UserID)
}

package localization

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/google/uuid"
)

var (
	// ErrLocaleNotFound indicates the requested locale code does not exist.
	ErrLocaleNotFound = errors.New("locale not found")
	// ErrLocaleInUse indicates the locale cannot be deleted while content or settings reference it.
	ErrLocaleInUse = errors.New("locale is currently in use")
)

// Repository persists configured locales across restarts.
type Repository interface {
	List(ctx context.Context) ([]Locale, error)
	GetByCode(ctx context.Context, code string) (Locale, error)
	Create(ctx context.Context, l Locale) error
	Update(ctx context.Context, l Locale) error
	SetDefault(ctx context.Context, code string) error
	Delete(ctx context.Context, code string) error
}

// MemoryRepository stores locales in memory when no SQL database is configured.
type MemoryRepository struct {
	mu      sync.RWMutex
	locales map[string]Locale
}

// NewMemoryRepository returns an initialized MemoryRepository seeded with "en".
func NewMemoryRepository() *MemoryRepository {
	now := time.Now().UTC()
	return &MemoryRepository{
		locales: map[string]Locale{
			"en": {
				ID:         "loc-default-en",
				Code:       "en",
				Name:       "English",
				NativeName: "English",
				IsDefault:  true,
				IsEnabled:  true,
				Direction:  DirectionLTR,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		},
	}
}

func (m *MemoryRepository) List(_ context.Context) ([]Locale, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Locale, 0, len(m.locales))
	for _, l := range m.locales {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDefault != out[j].IsDefault {
			return out[i].IsDefault
		}
		return out[i].Code < out[j].Code
	})
	return out, nil
}

func (m *MemoryRepository) GetByCode(_ context.Context, code string) (Locale, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := strings.ToLower(strings.TrimSpace(code))
	for _, l := range m.locales {
		if strings.ToLower(l.Code) == key {
			return l, nil
		}
	}
	return Locale{}, fmt.Errorf("%w: %q", ErrLocaleNotFound, code)
}

func (m *MemoryRepository) Create(_ context.Context, l Locale) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(l.Code)
	for k := range m.locales {
		if strings.ToLower(k) == key {
			return fmt.Errorf("localization: locale %q already exists", l.Code)
		}
	}
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	if l.IsDefault {
		for k, existing := range m.locales {
			existing.IsDefault = false
			m.locales[k] = existing
		}
	}
	m.locales[key] = l
	return nil
}

func (m *MemoryRepository) Update(_ context.Context, l Locale) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(l.Code)
	if _, ok := m.locales[key]; !ok {
		return fmt.Errorf("%w: %q", ErrLocaleNotFound, l.Code)
	}
	if l.IsDefault {
		for k, existing := range m.locales {
			existing.IsDefault = false
			m.locales[k] = existing
		}
	}
	m.locales[key] = l
	return nil
}

func (m *MemoryRepository) SetDefault(_ context.Context, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(strings.TrimSpace(code))
	if _, ok := m.locales[key]; !ok {
		return fmt.Errorf("%w: %q", ErrLocaleNotFound, code)
	}
	for k, existing := range m.locales {
		existing.IsDefault = (k == key)
		if existing.IsDefault {
			existing.IsEnabled = true
		}
		m.locales[k] = existing
	}
	return nil
}

func (m *MemoryRepository) Delete(_ context.Context, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(strings.TrimSpace(code))
	l, ok := m.locales[key]
	if !ok {
		return fmt.Errorf("%w: %q", ErrLocaleNotFound, code)
	}
	if l.IsDefault {
		return errors.New("localization: cannot delete the default locale")
	}
	delete(m.locales, key)
	return nil
}

// SQLRepository stores locales in the cms_locales PostgreSQL/MySQL table.
type SQLRepository struct {
	db interfaces.SQLDatabase
}

// NewSQLRepository wraps an SQL database for cms_locales persistence.
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
			sb.WriteString(fmt.Sprintf("$%d", idx))
			idx++
		} else {
			sb.WriteByte(query[i])
		}
	}
	return sb.String()
}

func scanLocale(scanner interface{ Scan(...any) error }) (Locale, error) {
	var l Locale
	var dirStr string
	if err := scanner.Scan(
		&l.ID, &l.Code, &l.Name, &l.NativeName,
		&l.IsDefault, &l.IsEnabled, &l.FallbackCode, &dirStr,
		&l.CreatedAt, &l.UpdatedAt,
	); err != nil {
		return Locale{}, err
	}
	l.Direction = Direction(dirStr)
	return l, nil
}

func (r *SQLRepository) List(ctx context.Context) ([]Locale, error) {
	query := `SELECT id, code, name, native_name, is_default, is_enabled, fallback_code, direction, created_at, updated_at
		FROM cms_locales ORDER BY is_default DESC, code ASC`
	rows, err := r.db.QueryContext(ctx, r.rebind(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Locale
	for rows.Next() {
		l, err := scanLocale(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *SQLRepository) GetByCode(ctx context.Context, code string) (Locale, error) {
	query := r.rebind(`SELECT id, code, name, native_name, is_default, is_enabled, fallback_code, direction, created_at, updated_at
		FROM cms_locales WHERE LOWER(code) = LOWER(?) LIMIT 1`)
	l, err := scanLocale(r.db.QueryRowContext(ctx, query, strings.TrimSpace(code)))
	if errors.Is(err, sql.ErrNoRows) {
		return Locale{}, fmt.Errorf("%w: %q", ErrLocaleNotFound, code)
	}
	return l, err
}

func (r *SQLRepository) Create(ctx context.Context, l Locale) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	if l.IsDefault {
		if _, err := r.db.ExecContext(ctx, r.rebind(`UPDATE cms_locales SET is_default = false, updated_at = ?`), l.UpdatedAt); err != nil {
			return err
		}
	}
	query := r.rebind(`INSERT INTO cms_locales (id, code, name, native_name, is_default, is_enabled, fallback_code, direction, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := r.db.ExecContext(ctx, query,
		l.ID, l.Code, l.Name, l.NativeName, l.IsDefault, l.IsEnabled, l.FallbackCode, string(l.Direction), l.CreatedAt, l.UpdatedAt,
	)
	return err
}

func (r *SQLRepository) Update(ctx context.Context, l Locale) error {
	if l.IsDefault {
		if _, err := r.db.ExecContext(ctx, r.rebind(`UPDATE cms_locales SET is_default = false, updated_at = ?`), l.UpdatedAt); err != nil {
			return err
		}
	}
	query := r.rebind(`UPDATE cms_locales
		SET name = ?, native_name = ?, is_default = ?, is_enabled = ?, fallback_code = ?, direction = ?, updated_at = ?
		WHERE LOWER(code) = LOWER(?)`)
	res, err := r.db.ExecContext(ctx, query,
		l.Name, l.NativeName, l.IsDefault, l.IsEnabled, l.FallbackCode, string(l.Direction), l.UpdatedAt, l.Code,
	)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: %q", ErrLocaleNotFound, l.Code)
	}
	return nil
}

func (r *SQLRepository) SetDefault(ctx context.Context, code string) error {
	now := time.Now().UTC()
	if _, err := r.GetByCode(ctx, code); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, r.rebind(`UPDATE cms_locales SET is_default = false, updated_at = ?`), now); err != nil {
		return err
	}
	query := r.rebind(`UPDATE cms_locales SET is_default = true, is_enabled = true, updated_at = ? WHERE LOWER(code) = LOWER(?)`)
	_, err := r.db.ExecContext(ctx, query, now, strings.TrimSpace(code))
	return err
}

func (r *SQLRepository) Delete(ctx context.Context, code string) error {
	query := r.rebind(`DELETE FROM cms_locales WHERE LOWER(code) = LOWER(?)`)
	res, err := r.db.ExecContext(ctx, query, strings.TrimSpace(code))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: %q", ErrLocaleNotFound, code)
	}
	return nil
}


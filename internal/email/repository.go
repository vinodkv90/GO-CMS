package email

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/google/uuid"
)

// ErrTemplateNotFound indicates the requested email template key does not exist.
var ErrTemplateNotFound = errors.New("email template not found")

// TemplateRepository persists transactional email templates.
type TemplateRepository interface {
	ListTemplates(ctx context.Context) ([]EmailTemplate, error)
	GetTemplateByKey(ctx context.Context, key string) (*EmailTemplate, error)
	UpsertTemplate(ctx context.Context, tmpl *EmailTemplate) (*EmailTemplate, error)
	SeedMissing(ctx context.Context, defaults []EmailTemplate) error
}

// MemoryTemplateRepository stores templates in memory for tests and embedded mode.
type MemoryTemplateRepository struct {
	mu        sync.RWMutex
	templates map[string]EmailTemplate
}

// NewMemoryTemplateRepository constructs an in-memory TemplateRepository.
func NewMemoryTemplateRepository() *MemoryTemplateRepository {
	return &MemoryTemplateRepository{
		templates: make(map[string]EmailTemplate),
	}
}

func (m *MemoryTemplateRepository) ListTemplates(ctx context.Context) ([]EmailTemplate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]EmailTemplate, 0, len(m.templates))
	for _, t := range m.templates {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].TemplateKey < out[j].TemplateKey
	})
	return out, nil
}

func (m *MemoryTemplateRepository) GetTemplateByKey(ctx context.Context, key string) (*EmailTemplate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.templates[key]
	if !ok {
		return nil, ErrTemplateNotFound
	}
	copyVal := t
	return &copyVal, nil
}

func (m *MemoryTemplateRepository) UpsertTemplate(ctx context.Context, tmpl *EmailTemplate) (*EmailTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copyVal := *tmpl
	if copyVal.ID == "" {
		if prev, ok := m.templates[copyVal.TemplateKey]; ok {
			copyVal.ID = prev.ID
		} else {
			copyVal.ID = uuid.New().String()
		}
	}
	copyVal.UpdatedAt = time.Now().UTC()
	m.templates[copyVal.TemplateKey] = copyVal
	res := copyVal
	return &res, nil
}

func (m *MemoryTemplateRepository) SeedMissing(ctx context.Context, defaults []EmailTemplate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range defaults {
		if _, exists := m.templates[d.TemplateKey]; !exists {
			copyVal := d
			if copyVal.ID == "" {
				copyVal.ID = uuid.New().String()
			}
			m.templates[copyVal.TemplateKey] = copyVal
		}
	}
	return nil
}

// SQLTemplateRepository implements TemplateRepository against PostgreSQL or MySQL cms_email_templates.
type SQLTemplateRepository struct {
	db interfaces.SQLDatabase
}

// NewSQLTemplateRepository constructs a SQL-backed TemplateRepository.
func NewSQLTemplateRepository(db interfaces.SQLDatabase) *SQLTemplateRepository {
	return &SQLTemplateRepository{db: db}
}

func (r *SQLTemplateRepository) rebind(query string) string {
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

func scanTemplateRow(scan func(dest ...any) error) (*EmailTemplate, error) {
	var t EmailTemplate
	var varsRaw []byte
	if err := scan(
		&t.ID,
		&t.TemplateKey,
		&t.Name,
		&t.Subject,
		&t.FromName,
		&t.FromEmail,
		&t.ReplyTo,
		&t.BodyHTML,
		&t.BodyText,
		&varsRaw,
		&t.Enabled,
		&t.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(varsRaw) > 0 {
		_ = json.Unmarshal(varsRaw, &t.Variables)
	}
	if t.Variables == nil {
		t.Variables = []string{}
	}
	return &t, nil
}

func (r *SQLTemplateRepository) ListTemplates(ctx context.Context) ([]EmailTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, template_key, name, subject, from_name, from_email, reply_to, body_html, body_text, variables, enabled, updated_at FROM cms_email_templates ORDER BY template_key ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []EmailTemplate
	for rows.Next() {
		t, err := scanTemplateRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		list = append(list, *t)
	}
	return list, rows.Err()
}

func (r *SQLTemplateRepository) GetTemplateByKey(ctx context.Context, key string) (*EmailTemplate, error) {
	q := r.rebind(`SELECT id, template_key, name, subject, from_name, from_email, reply_to, body_html, body_text, variables, enabled, updated_at FROM cms_email_templates WHERE template_key = ? LIMIT 1`)
	row := r.db.QueryRowContext(ctx, q, key)
	t, err := scanTemplateRow(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *SQLTemplateRepository) UpsertTemplate(ctx context.Context, tmpl *EmailTemplate) (*EmailTemplate, error) {
	id := tmpl.ID
	if id == "" {
		id = uuid.New().String()
	}
	now := time.Now().UTC()
	varsJSON, _ := json.Marshal(tmpl.Variables)

	var q string
	if r.db.DriverName() == "mysql" {
		q = `INSERT INTO cms_email_templates
			(id, template_key, name, subject, from_name, from_email, reply_to, body_html, body_text, variables, enabled, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				name = VALUES(name),
				subject = VALUES(subject),
				from_name = VALUES(from_name),
				from_email = VALUES(from_email),
				reply_to = VALUES(reply_to),
				body_html = VALUES(body_html),
				body_text = VALUES(body_text),
				variables = VALUES(variables),
				enabled = VALUES(enabled),
				updated_at = VALUES(updated_at)`
	} else {
		q = `INSERT INTO cms_email_templates
			(id, template_key, name, subject, from_name, from_email, reply_to, body_html, body_text, variables, enabled, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $12)
			ON CONFLICT (template_key) DO UPDATE SET
				name = EXCLUDED.name,
				subject = EXCLUDED.subject,
				from_name = EXCLUDED.from_name,
				from_email = EXCLUDED.from_email,
				reply_to = EXCLUDED.reply_to,
				body_html = EXCLUDED.body_html,
				body_text = EXCLUDED.body_text,
				variables = EXCLUDED.variables,
				enabled = EXCLUDED.enabled,
				updated_at = EXCLUDED.updated_at`
	}

	if _, err := r.db.ExecContext(ctx, q,
		id,
		tmpl.TemplateKey,
		tmpl.Name,
		tmpl.Subject,
		tmpl.FromName,
		tmpl.FromEmail,
		tmpl.ReplyTo,
		tmpl.BodyHTML,
		tmpl.BodyText,
		string(varsJSON),
		tmpl.Enabled,
		now,
	); err != nil {
		return nil, err
	}
	return r.GetTemplateByKey(ctx, tmpl.TemplateKey)
}

func (r *SQLTemplateRepository) SeedMissing(ctx context.Context, defaults []EmailTemplate) error {
	for _, d := range defaults {
		if _, err := r.GetTemplateByKey(ctx, d.TemplateKey); err == nil {
			continue
		} else if !errors.Is(err, ErrTemplateNotFound) {
			return err
		}
		if d.ID == "" {
			d.ID = uuid.New().String()
		}
		varsJSON, _ := json.Marshal(d.Variables)
		var q string
		if r.db.DriverName() == "mysql" {
			q = `INSERT IGNORE INTO cms_email_templates
				(id, template_key, name, subject, from_name, from_email, reply_to, body_html, body_text, variables, enabled, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		} else {
			q = `INSERT INTO cms_email_templates
				(id, template_key, name, subject, from_name, from_email, reply_to, body_html, body_text, variables, enabled, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $12)
				ON CONFLICT (template_key) DO NOTHING`
		}
		if _, err := r.db.ExecContext(ctx, q,
			d.ID,
			d.TemplateKey,
			d.Name,
			d.Subject,
			d.FromName,
			d.FromEmail,
			d.ReplyTo,
			d.BodyHTML,
			d.BodyText,
			string(varsJSON),
			d.Enabled,
			time.Now().UTC(),
		); err != nil {
			return err
		}
	}
	return nil
}


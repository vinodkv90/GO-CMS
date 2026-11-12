package webhooks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/go-cms/cms/internal/database/interfaces"
)

var (
	// ErrWebhookNotFound indicates the requested webhook ID does not exist.
	ErrWebhookNotFound = errors.New("webhook not found")
)

// Repository persists webhooks and their delivery logs.
type Repository interface {
	ListWebhooks(ctx context.Context) ([]*Webhook, error)
	GetWebhook(ctx context.Context, id string) (*Webhook, error)
	CreateWebhook(ctx context.Context, wh *Webhook) error
	UpdateWebhook(ctx context.Context, wh *Webhook) error
	DeleteWebhook(ctx context.Context, id string) error

	CreateDelivery(ctx context.Context, d *WebhookDelivery) error
	ListDeliveries(ctx context.Context, webhookID string, limit, offset int) ([]*WebhookDelivery, int, error)
}

// MemoryRepository provides an in-memory fallback when an SQL database is not connected.
type MemoryRepository struct {
	mu         sync.RWMutex
	webhooks   map[string]*Webhook
	deliveries map[string][]*WebhookDelivery
}

// NewMemoryRepository returns an initialized MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		webhooks:   make(map[string]*Webhook),
		deliveries: make(map[string][]*WebhookDelivery),
	}
}

func cloneWebhook(w *Webhook) *Webhook {
	if w == nil {
		return nil
	}
	cp := *w
	cp.Headers = make(map[string]string, len(w.Headers))
	for k, v := range w.Headers {
		cp.Headers[k] = v
	}
	cp.Events = append([]string(nil), w.Events...)
	return &cp
}

func (m *MemoryRepository) ListWebhooks(_ context.Context) ([]*Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Webhook, 0, len(m.webhooks))
	for _, w := range m.webhooks {
		out = append(out, cloneWebhook(w))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (m *MemoryRepository) GetWebhook(_ context.Context, id string) (*Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	w, ok := m.webhooks[id]
	if !ok {
		return nil, ErrWebhookNotFound
	}
	return cloneWebhook(w), nil
}

func (m *MemoryRepository) CreateWebhook(_ context.Context, wh *Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhooks[wh.ID] = cloneWebhook(wh)
	return nil
}

func (m *MemoryRepository) UpdateWebhook(_ context.Context, wh *Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.webhooks[wh.ID]; !ok {
		return ErrWebhookNotFound
	}
	m.webhooks[wh.ID] = cloneWebhook(wh)
	return nil
}

func (m *MemoryRepository) DeleteWebhook(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.webhooks[id]; !ok {
		return ErrWebhookNotFound
	}
	delete(m.webhooks, id)
	delete(m.deliveries, id)
	return nil
}

func (m *MemoryRepository) CreateDelivery(_ context.Context, d *WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *d
	m.deliveries[d.WebhookID] = append([]*WebhookDelivery{&cp}, m.deliveries[d.WebhookID]...)
	return nil
}

func (m *MemoryRepository) ListDeliveries(_ context.Context, webhookID string, limit, offset int) ([]*WebhookDelivery, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := m.deliveries[webhookID]
	total := len(items)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []*WebhookDelivery{}, total, nil
	}
	if limit <= 0 {
		limit = 20
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out := make([]*WebhookDelivery, 0, end-offset)
	for _, item := range items[offset:end] {
		cp := *item
		out = append(out, &cp)
	}
	return out, total, nil
}

// SQLRepository persists webhooks and delivery logs in PostgreSQL or MySQL.
type SQLRepository struct {
	db interfaces.SQLDatabase
}

// NewSQLRepository wraps an SQL database for webhook persistence.
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

func scanWebhook(scanner interface{ Scan(...any) error }) (*Webhook, error) {
	var wh Webhook
	var headersRaw, eventsRaw string
	var secretEnc, createdBy sql.NullString
	if err := scanner.Scan(
		&wh.ID,
		&wh.Name,
		&wh.URL,
		&headersRaw,
		&eventsRaw,
		&secretEnc,
		&wh.IsEnabled,
		&wh.MaxRetries,
		&createdBy,
		&wh.CreatedAt,
		&wh.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if secretEnc.Valid {
		wh.SecretEnc = secretEnc.String
	}
	if createdBy.Valid {
		wh.CreatedBy = createdBy.String
	}
	wh.Headers = map[string]string{}
	if strings.TrimSpace(headersRaw) != "" {
		_ = json.Unmarshal([]byte(headersRaw), &wh.Headers)
	}
	wh.Events = []string{}
	if strings.TrimSpace(eventsRaw) != "" {
		_ = json.Unmarshal([]byte(eventsRaw), &wh.Events)
	}
	return &wh, nil
}

func (r *SQLRepository) ListWebhooks(ctx context.Context) ([]*Webhook, error) {
	query := `SELECT id, name, url, headers, events, secret_enc, is_enabled, max_retries, created_by, created_at, updated_at
		FROM cms_webhooks ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, r.rebind(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Webhook
	for rows.Next() {
		wh, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wh)
	}
	return out, rows.Err()
}

func (r *SQLRepository) GetWebhook(ctx context.Context, id string) (*Webhook, error) {
	query := r.rebind(`SELECT id, name, url, headers, events, secret_enc, is_enabled, max_retries, created_by, created_at, updated_at
		FROM cms_webhooks WHERE id = ?`)
	row := r.db.QueryRowContext(ctx, query, id)
	wh, err := scanWebhook(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWebhookNotFound
	}
	if err != nil {
		return nil, err
	}
	return wh, nil
}

func (r *SQLRepository) CreateWebhook(ctx context.Context, wh *Webhook) error {
	headersJSON, _ := json.Marshal(wh.Headers)
	eventsJSON, _ := json.Marshal(wh.Events)
	query := r.rebind(`INSERT INTO cms_webhooks (id, name, url, headers, events, secret_enc, is_enabled, max_retries, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := r.db.ExecContext(ctx, query,
		wh.ID, wh.Name, wh.URL, string(headersJSON), string(eventsJSON),
		wh.SecretEnc, wh.IsEnabled, wh.MaxRetries, wh.CreatedBy, wh.CreatedAt, wh.UpdatedAt,
	)
	return err
}

func (r *SQLRepository) UpdateWebhook(ctx context.Context, wh *Webhook) error {
	headersJSON, _ := json.Marshal(wh.Headers)
	eventsJSON, _ := json.Marshal(wh.Events)
	query := r.rebind(`UPDATE cms_webhooks
		SET name = ?, url = ?, headers = ?, events = ?, secret_enc = ?, is_enabled = ?, max_retries = ?, updated_at = ?
		WHERE id = ?`)
	res, err := r.db.ExecContext(ctx, query,
		wh.Name, wh.URL, string(headersJSON), string(eventsJSON),
		wh.SecretEnc, wh.IsEnabled, wh.MaxRetries, wh.UpdatedAt, wh.ID,
	)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return ErrWebhookNotFound
	}
	return nil
}

func (r *SQLRepository) DeleteWebhook(ctx context.Context, id string) error {
	query := r.rebind(`DELETE FROM cms_webhooks WHERE id = ?`)
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return ErrWebhookNotFound
	}
	return nil
}

func (r *SQLRepository) CreateDelivery(ctx context.Context, d *WebhookDelivery) error {
	query := r.rebind(`INSERT INTO cms_webhook_deliveries (id, webhook_id, event, request_url, status_code, duration_ms, attempt, success, error_message, response_snippet, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := r.db.ExecContext(ctx, query,
		d.ID, d.WebhookID, d.Event, d.RequestURL, d.StatusCode, d.DurationMs,
		d.Attempt, d.Success, d.ErrorMessage, d.ResponseSnippet, d.CreatedAt,
	)
	return err
}

func (r *SQLRepository) ListDeliveries(ctx context.Context, webhookID string, limit, offset int) ([]*WebhookDelivery, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	countQuery := r.rebind(`SELECT COUNT(*) FROM cms_webhook_deliveries WHERE webhook_id = ?`)
	if err := r.db.QueryRowContext(ctx, countQuery, webhookID).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := r.rebind(`SELECT id, webhook_id, event, request_url, status_code, duration_ms, attempt, success, error_message, response_snippet, created_at
		FROM cms_webhook_deliveries
		WHERE webhook_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`)
	rows, err := r.db.QueryContext(ctx, listQuery, webhookID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*WebhookDelivery
	for rows.Next() {
		var d WebhookDelivery
		var errMsg, snippet sql.NullString
		if err := rows.Scan(
			&d.ID, &d.WebhookID, &d.Event, &d.RequestURL,
			&d.StatusCode, &d.DurationMs, &d.Attempt, &d.Success,
			&errMsg, &snippet, &d.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		if errMsg.Valid {
			d.ErrorMessage = errMsg.String
		}
		if snippet.Valid {
			d.ResponseSnippet = snippet.String
		}
		out = append(out, &d)
	}
	if out == nil {
		out = []*WebhookDelivery{}
	}
	return out, total, rows.Err()
}


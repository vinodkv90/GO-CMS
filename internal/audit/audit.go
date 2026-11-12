package audit

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AuditLog represents an immutable system audit event.
type AuditLog struct {
	ID        string         `json:"id"`
	ActorID   string         `json:"actor_id"`
	ActorType string         `json:"actor_type"` // "admin", "user", "api_token", "system"
	Action    string         `json:"action"`     // "create", "update", "delete", "publish", "login"
	Resource  string         `json:"resource"`   // "content:articles", "auth:user", etc.
	TargetID  string         `json:"target_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	IPAddress string         `json:"ip_address,omitempty"`
	UserAgent string         `json:"user_agent,omitempty"`
	Result    string         `json:"result,omitempty"` // "success", "failure"
	Timestamp time.Time      `json:"timestamp"`
}

// Filter specifies query criteria for retrieving audit logs.
type Filter struct {
	ActorID  string
	Action   string
	Resource string
	Result   string
	From     *time.Time
	To       *time.Time
	Limit    int
	Offset   int
}

// Repository defines storage operations for audit logs.
type Repository interface {
	Create(ctx context.Context, log *AuditLog) error
	List(ctx context.Context, filter Filter) ([]*AuditLog, int, error)
}

// MemoryRepository provides a thread-safe in-memory store for audit logs.
type MemoryRepository struct {
	mu   sync.RWMutex
	logs []*AuditLog
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		logs: make([]*AuditLog, 0),
	}
}

func (r *MemoryRepository) Create(_ context.Context, log *AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := *log
	cp.Metadata = RedactMetadata(log.Metadata)
	// Prepend to maintain newest-first order in memory
	r.logs = append([]*AuditLog{&cp}, r.logs...)
	return nil
}

func (r *MemoryRepository) List(_ context.Context, filter Filter) ([]*AuditLog, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filtered := make([]*AuditLog, 0)
	for _, l := range r.logs {
		if filter.ActorID != "" && l.ActorID != filter.ActorID {
			continue
		}
		if filter.Action != "" && !strings.Contains(strings.ToLower(l.Action), strings.ToLower(filter.Action)) {
			continue
		}
		if filter.Resource != "" && !strings.Contains(strings.ToLower(l.Resource), strings.ToLower(filter.Resource)) {
			continue
		}
		if filter.Result != "" && !strings.EqualFold(l.Result, filter.Result) {
			continue
		}
		if filter.From != nil && l.Timestamp.Before(*filter.From) {
			continue
		}
		if filter.To != nil && l.Timestamp.After(*filter.To) {
			continue
		}
		filtered = append(filtered, l)
	}

	total := len(filtered)
	if filter.Offset >= total {
		return []*AuditLog{}, total, nil
	}

	end := filter.Offset + filter.Limit
	if end > total || filter.Limit <= 0 {
		end = total
	}

	return filtered[filter.Offset:end], total, nil
}

// RecordParams defines parameters for logging an audit action.
type RecordParams struct {
	ActorID   string
	ActorType string
	Action    string
	Resource  string
	TargetID  string
	Metadata  map[string]any
	IPAddress string
	UserAgent string
	Result    string
}

// Service provides high-level audit tracking.
type Service struct {
	repo Repository
}

// NewService creates an audit logging Service.
func NewService(repo Repository) *Service {
	if repo == nil {
		repo = NewMemoryRepository()
	}
	return &Service{repo: repo}
}

// Record persists an audit event with automatic sensitive-key redaction.
func (s *Service) Record(ctx context.Context, params RecordParams) (*AuditLog, error) {
	if params.ActorType == "" {
		params.ActorType = "system"
	}
	if params.Result == "" {
		params.Result = "success"
	}

	entry := &AuditLog{
		ID:        uuid.NewString(),
		ActorID:   params.ActorID,
		ActorType: params.ActorType,
		Action:    params.Action,
		Resource:  params.Resource,
		TargetID:  params.TargetID,
		Metadata:  RedactMetadata(params.Metadata),
		IPAddress: params.IPAddress,
		UserAgent: params.UserAgent,
		Result:    params.Result,
		Timestamp: time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, entry); err != nil {
		return nil, err
	}

	return entry, nil
}

// List retrieves historical audit logs matching the supplied filter.
func (s *Service) List(ctx context.Context, filter Filter) ([]*AuditLog, int, error) {
	return s.repo.List(ctx, filter)
}

func isSensitiveAuditKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if lower == "" {
		return false
	}
	switch lower {
	case "password", "new_password", "current_password", "confirm_password",
		"secret", "client_secret", "smtp_password", "secret_access_key",
		"token", "access_token", "refresh_token", "raw_token", "mfa_token",
		"invite_token", "reset_token", "api_key", "apikey",
		"authorization", "cookie", "set-cookie", "private_key", "signing_key",
		"mfa_secret", "mfa_secret_enc", "recovery_codes", "backup_codes", "credential", "credentials":
		return true
	}
	if strings.HasSuffix(lower, "_password") ||
		strings.HasSuffix(lower, "_secret") ||
		strings.HasSuffix(lower, "_secret_enc") ||
		strings.HasSuffix(lower, "_api_key") ||
		strings.HasSuffix(lower, "_private_key") ||
		strings.HasSuffix(lower, "_access_token") ||
		strings.HasSuffix(lower, "_refresh_token") ||
		strings.HasSuffix(lower, "_credential") {
		return true
	}
	return false
}

// RedactMetadata deep-clones an audit metadata map while scrubbing sensitive secret/credential fields.
func RedactMetadata(meta map[string]any) map[string]any {
	if meta == nil {
		return nil
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		if isSensitiveAuditKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = redactAnyValue(v)
	}
	return out
}

func redactAnyValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		return RedactMetadata(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactAnyValue(item)
		}
		return out
	default:
		return v
	}
}

package audit

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AuditLog records a single administrative or system action.
type AuditLog struct {
	ID        string         `json:"id"`
	ActorID   string         `json:"actor_id"`
	ActorType string         `json:"actor_type"` // "user", "api_token", "system"
	Action    string         `json:"action"`     // "create", "update", "delete", "publish", "login"
	Resource  string         `json:"resource"`   // "content:articles", "auth:user", etc.
	TargetID  string         `json:"target_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	IPAddress string         `json:"ip_address,omitempty"`
	UserAgent string         `json:"user_agent,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// Filter specifies query criteria for retrieving audit logs.
type Filter struct {
	ActorID  string
	Action   string
	Resource string
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

// NewMemoryRepository creates a new in-memory audit log repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		logs: make([]*AuditLog, 0),
	}
}

// Create persists an audit log record.
func (r *MemoryRepository) Create(ctx context.Context, log *AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Prepend for newest-first order
	r.logs = append([]*AuditLog{log}, r.logs...)
	return nil
}

// List returns filtered audit log entries with total count.
func (r *MemoryRepository) List(ctx context.Context, filter Filter) ([]*AuditLog, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filtered := make([]*AuditLog, 0)
	for _, l := range r.logs {
		if filter.ActorID != "" && l.ActorID != filter.ActorID {
			continue
		}
		if filter.Action != "" && l.Action != filter.Action {
			continue
		}
		if filter.Resource != "" && l.Resource != filter.Resource {
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
}

// Service provides high-level audit tracking.
type Service struct {
	repo Repository
}

// NewService creates a new audit Service.
func NewService(repo Repository) *Service {
	if repo == nil {
		repo = NewMemoryRepository()
	}
	return &Service{repo: repo}
}

// Record records an event in the audit trail.
func (s *Service) Record(ctx context.Context, params RecordParams) (*AuditLog, error) {
	if params.ActorType == "" {
		params.ActorType = "system"
	}
	if params.ActorID == "" {
		params.ActorID = "system"
	}

	entry := &AuditLog{
		ID:        uuid.New().String(),
		ActorID:   params.ActorID,
		ActorType: params.ActorType,
		Action:    params.Action,
		Resource:  params.Resource,
		TargetID:  params.TargetID,
		Metadata:  params.Metadata,
		IPAddress: params.IPAddress,
		UserAgent: params.UserAgent,
		Timestamp: time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

// List queries audit logs matching the given filter.
func (s *Service) List(ctx context.Context, filter Filter) ([]*AuditLog, int, error) {
	return s.repo.List(ctx, filter)
}


package settings

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryRepository implements Repository in memory for fallback mode and unit tests.
type MemoryRepository struct {
	mu       sync.RWMutex
	settings map[string]*Record
	prefs    map[string]*UserPreferences
}

// NewMemoryRepository initializes an in-memory settings store.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		settings: make(map[string]*Record),
		prefs:    make(map[string]*UserPreferences),
	}
}

func (r *MemoryRepository) GetSetting(_ context.Context, key string) (*Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.settings[key]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *rec
	clone.Value = append(json.RawMessage(nil), rec.Value...)
	return &clone, nil
}

func (r *MemoryRepository) UpsertSetting(_ context.Context, key string, value json.RawMessage, isPublic bool) (*Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	id := uuid.New().String()
	if existing, ok := r.settings[key]; ok {
		id = existing.ID
	}
	rec := &Record{
		ID:        id,
		Key:       key,
		Value:     append(json.RawMessage(nil), value...),
		IsPublic:  isPublic,
		UpdatedAt: now,
	}
	r.settings[key] = rec
	clone := *rec
	clone.Value = append(json.RawMessage(nil), rec.Value...)
	return &clone, nil
}

func (r *MemoryRepository) GetPreferences(_ context.Context, userID string) (*UserPreferences, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.prefs[userID]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *p
	return &clone, nil
}

func (r *MemoryRepository) UpsertPreferences(_ context.Context, prefs *UserPreferences) (*UserPreferences, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	clone := *prefs
	clone.UpdatedAt = time.Now().UTC()
	r.prefs[prefs.UserID] = &clone

	res := clone
	return &res, nil
}


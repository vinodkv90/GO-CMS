package media

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemoryRepository provides an in-memory implementation of media.Repository for unit tests.
type MemoryRepository struct {
	mu     sync.RWMutex
	assets map[string]*MediaAsset
}

// NewMemoryRepository initializes a new in-memory media repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		assets: make(map[string]*MediaAsset),
	}
}

// Create persists an asset in memory.
func (m *MemoryRepository) Create(ctx context.Context, asset *MediaAsset) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = now
	}
	if asset.UpdatedAt.IsZero() {
		asset.UpdatedAt = now
	}

	clone := *asset
	m.assets[asset.ID] = &clone
	return nil
}

// GetByID retrieves an asset by ID.
func (m *MemoryRepository) GetByID(ctx context.Context, id string) (*MediaAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	a, ok := m.assets[id]
	if !ok {
		return nil, ErrAssetNotFound
	}

	clone := *a
	return &clone, nil
}

// GetByKey retrieves an asset by storage key.
func (m *MemoryRepository) GetByKey(ctx context.Context, storageKey string) (*MediaAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, a := range m.assets {
		if a.StorageKey == storageKey {
			clone := *a
			return &clone, nil
		}
	}

	return nil, ErrAssetNotFound
}

// List returns filtered assets from memory.
func (m *MemoryRepository) List(ctx context.Context, filter MediaFilter) ([]*MediaAsset, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matched []*MediaAsset
	for _, a := range m.assets {
		if filter.Search != "" {
			term := strings.ToLower(filter.Search)
			if !strings.Contains(strings.ToLower(a.Filename), term) && !strings.Contains(strings.ToLower(a.OriginalName), term) {
				continue
			}
		}

		if filter.MIMEType != "" {
			if strings.HasSuffix(filter.MIMEType, "/*") || strings.HasSuffix(filter.MIMEType, "/") {
				prefix := strings.TrimSuffix(strings.TrimSuffix(filter.MIMEType, "*"), "/")
				if !strings.HasPrefix(a.MIMEType, prefix+"/") {
					continue
				}
			} else if a.MIMEType != filter.MIMEType {
				continue
			}
		}

		clone := *a
		matched = append(matched, &clone)
	}

	total := len(matched)

	// Apply pagination
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
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

// Update modifies an asset in memory.
func (m *MemoryRepository) Update(ctx context.Context, asset *MediaAsset) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.assets[asset.ID]; !ok {
		return ErrAssetNotFound
	}

	asset.UpdatedAt = time.Now().UTC()
	clone := *asset
	m.assets[asset.ID] = &clone
	return nil
}

// Delete removes an asset from memory.
func (m *MemoryRepository) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.assets[id]; !ok {
		return ErrAssetNotFound
	}

	delete(m.assets, id)
	return nil
}

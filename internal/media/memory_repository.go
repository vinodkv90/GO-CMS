package media

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryRepository provides an in-memory implementation of media.Repository for unit tests.
type MemoryRepository struct {
	mu      sync.RWMutex
	assets  map[string]*MediaAsset
	folders map[string]*MediaFolder
}

// NewMemoryRepository initializes a new in-memory media repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		assets:  make(map[string]*MediaAsset),
		folders: make(map[string]*MediaFolder),
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
		// Filter by FolderID
		if filter.FolderID != nil {
			if *filter.FolderID == "" {
				if a.FolderID != nil {
					continue
				}
			} else {
				if a.FolderID == nil || *a.FolderID != *filter.FolderID {
					continue
				}
			}
		}

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

	// Sort matched assets newest first
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

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

// UpdateAssetFolder moves an asset to another folder.
func (m *MemoryRepository) UpdateAssetFolder(ctx context.Context, assetID string, targetFolderID *string, newStorageKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, ok := m.assets[assetID]
	if !ok {
		return ErrAssetNotFound
	}

	a.FolderID = targetFolderID
	a.StorageKey = newStorageKey
	a.Filename = filepath.Base(newStorageKey)
	a.UpdatedAt = time.Now().UTC()
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

// CreateFolder creates a folder in memory.
func (m *MemoryRepository) CreateFolder(ctx context.Context, folder *MediaFolder) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check duplicates under parent
	for _, f := range m.folders {
		sameParent := (f.ParentID == nil && folder.ParentID == nil) ||
			(f.ParentID != nil && folder.ParentID != nil && *f.ParentID == *folder.ParentID)
		if sameParent && (strings.EqualFold(f.Name, folder.Name) || f.Slug == folder.Slug) {
			return ErrDuplicateFolder
		}
	}

	now := time.Now().UTC()
	if folder.CreatedAt.IsZero() {
		folder.CreatedAt = now
	}
	if folder.UpdatedAt.IsZero() {
		folder.UpdatedAt = now
	}

	clone := *folder
	m.folders[folder.ID] = &clone
	return nil
}

// GetFolderByID retrieves a folder by ID.
func (m *MemoryRepository) GetFolderByID(ctx context.Context, id string) (*MediaFolder, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	f, ok := m.folders[id]
	if !ok {
		return nil, ErrFolderNotFound
	}

	clone := *f
	return &clone, nil
}

// GetFolderByPath retrieves a folder by canonical path.
func (m *MemoryRepository) GetFolderByPath(ctx context.Context, path string) (*MediaFolder, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, f := range m.folders {
		if f.Path == path {
			clone := *f
			return &clone, nil
		}
	}

	return nil, ErrFolderNotFound
}

// ListFolders lists folders under a parent (or all if parentID is nil).
func (m *MemoryRepository) ListFolders(ctx context.Context, parentID *string) ([]*MediaFolder, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*MediaFolder
	for _, f := range m.folders {
		if parentID == nil {
			clone := *f
			result = append(result, &clone)
		} else if *parentID == "" {
			if f.ParentID == nil {
				clone := *f
				result = append(result, &clone)
			}
		} else {
			if f.ParentID != nil && *f.ParentID == *parentID {
				clone := *f
				result = append(result, &clone)
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// GetAllFolders returns all folders sorted by path.
func (m *MemoryRepository) GetAllFolders(ctx context.Context) ([]*MediaFolder, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*MediaFolder
	for _, f := range m.folders {
		clone := *f
		result = append(result, &clone)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})

	return result, nil
}

// UpdateFolder modifies a folder in memory.
func (m *MemoryRepository) UpdateFolder(ctx context.Context, folder *MediaFolder) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, ok := m.folders[folder.ID]
	if !ok {
		return ErrFolderNotFound
	}

	folder.UpdatedAt = time.Now().UTC()
	*f = *folder
	return nil
}

// DeleteFolder removes a folder from memory.
func (m *MemoryRepository) DeleteFolder(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.folders[id]; !ok {
		return ErrFolderNotFound
	}

	delete(m.folders, id)
	return nil
}

// GetAssetsByFolderID returns all assets in the given folder.
func (m *MemoryRepository) GetAssetsByFolderID(ctx context.Context, folderID string) ([]*MediaAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*MediaAsset
	for _, a := range m.assets {
		if a.FolderID != nil && *a.FolderID == folderID {
			clone := *a
			result = append(result, &clone)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

// GetSubfolders returns subfolders directly under parentID.
func (m *MemoryRepository) GetSubfolders(ctx context.Context, parentID string) ([]*MediaFolder, error) {
	p := &parentID
	return m.ListFolders(ctx, p)
}

package content

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MemoryContentRepository provides a thread-safe in-memory implementation of content repositories.
type MemoryContentRepository struct {
	mu            sync.RWMutex
	contentTypes  map[string]*ContentType
	typesByUID    map[string]string // uid -> ID
	fields        map[string]*ContentField
	fieldsByType  map[string][]string // typeID -> []fieldID
	entries       map[string]*ContentEntry
	entriesByType map[string][]string // typeID -> []entryID
	versions      map[string]*ContentVersion
	versionsByEnt map[string][]string // entryID -> []versionID
}

// NewMemoryContentRepository creates an in-memory repository.
func NewMemoryContentRepository() *MemoryContentRepository {
	return &MemoryContentRepository{
		contentTypes:  make(map[string]*ContentType),
		typesByUID:    make(map[string]string),
		fields:        make(map[string]*ContentField),
		fieldsByType:  make(map[string][]string),
		entries:       make(map[string]*ContentEntry),
		entriesByType: make(map[string][]string),
		versions:      make(map[string]*ContentVersion),
		versionsByEnt: make(map[string][]string),
	}
}

// --- ContentTypeRepository ---

func (m *MemoryContentRepository) CreateType(ctx context.Context, ct *ContentType) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lowerUID := strings.ToLower(ct.UID)
	if _, exists := m.typesByUID[lowerUID]; exists {
		return ErrContentTypeExists
	}

	clone := *ct
	m.contentTypes[ct.ID] = &clone
	m.typesByUID[lowerUID] = ct.ID
	return nil
}

func (m *MemoryContentRepository) GetTypeByID(ctx context.Context, id string) (*ContentType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ct, ok := m.contentTypes[id]
	if !ok {
		return nil, ErrContentTypeNotFound
	}
	clone := *ct
	clone.Fields = m.listFieldsLocked(id)
	return &clone, nil
}

func (m *MemoryContentRepository) GetTypeByUID(ctx context.Context, uid string) (*ContentType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.typesByUID[strings.ToLower(uid)]
	if !ok {
		return nil, ErrContentTypeNotFound
	}
	ct := m.contentTypes[id]
	clone := *ct
	clone.Fields = m.listFieldsLocked(id)
	return &clone, nil
}

func (m *MemoryContentRepository) UpdateType(ctx context.Context, ct *ContentType) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.contentTypes[ct.ID]
	if !ok {
		return ErrContentTypeNotFound
	}
	existing.Name = ct.Name
	existing.Settings = ct.Settings
	existing.UpdatedAt = ct.UpdatedAt
	return nil
}

func (m *MemoryContentRepository) DeleteType(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ct, ok := m.contentTypes[id]
	if !ok {
		return ErrContentTypeNotFound
	}

	delete(m.typesByUID, strings.ToLower(ct.UID))
	delete(m.contentTypes, id)
	delete(m.fieldsByType, id)
	return nil
}

func (m *MemoryContentRepository) ListTypes(ctx context.Context) ([]*ContentType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ContentType
	for _, ct := range m.contentTypes {
		clone := *ct
		clone.Fields = m.listFieldsLocked(ct.ID)
		result = append(result, &clone)
	}
	return result, nil
}

func (m *MemoryContentRepository) CreateField(ctx context.Context, f *ContentField) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if field_key already exists in this content type
	for _, fid := range m.fieldsByType[f.ContentTypeID] {
		if existing, ok := m.fields[fid]; ok {
			if strings.EqualFold(existing.FieldKey, f.FieldKey) {
				return ErrFieldExists
			}
		}
	}

	clone := *f
	m.fields[f.ID] = &clone
	m.fieldsByType[f.ContentTypeID] = append(m.fieldsByType[f.ContentTypeID], f.ID)
	return nil
}

func (m *MemoryContentRepository) ListFields(ctx context.Context, contentTypeID string) ([]ContentField, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listFieldsLocked(contentTypeID), nil
}

func (m *MemoryContentRepository) listFieldsLocked(contentTypeID string) []ContentField {
	var result []ContentField
	for _, fid := range m.fieldsByType[contentTypeID] {
		if f, ok := m.fields[fid]; ok {
			result = append(result, *f)
		}
	}
	return result
}

func (m *MemoryContentRepository) DeleteField(ctx context.Context, fieldID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, ok := m.fields[fieldID]
	if !ok {
		return ErrFieldNotFound
	}

	delete(m.fields, fieldID)

	// Remove from fieldsByType
	typeFields := m.fieldsByType[f.ContentTypeID]
	var updated []string
	for _, fid := range typeFields {
		if fid != fieldID {
			updated = append(updated, fid)
		}
	}
	m.fieldsByType[f.ContentTypeID] = updated
	return nil
}

// --- ContentEntryRepository ---

func (m *MemoryContentRepository) CreateEntry(ctx context.Context, e *ContentEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e.SingletonKey != nil {
		for _, existing := range m.entries {
			if existing.SingletonKey != nil && *existing.SingletonKey == *e.SingletonKey && existing.DeletedAt == nil {
				return ErrSingletonExists
			}
		}
	}

	clone := *e
	m.entries[e.ID] = &clone
	m.entriesByType[e.ContentTypeID] = append(m.entriesByType[e.ContentTypeID], e.ID)
	return nil
}

func (m *MemoryContentRepository) GetEntryByID(ctx context.Context, id string) (*ContentEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.entries[id]
	if !ok || e.DeletedAt != nil {
		return nil, ErrEntryNotFound
	}
	clone := *e
	return &clone, nil
}

func (m *MemoryContentRepository) GetEntryBySlug(ctx context.Context, contentTypeID, slug string) (*ContentEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, eid := range m.entriesByType[contentTypeID] {
		if e, ok := m.entries[eid]; ok && e.DeletedAt == nil && e.Slug == slug {
			clone := *e
			return &clone, nil
		}
	}
	return nil, ErrEntryNotFound
}

func (m *MemoryContentRepository) GetSingleEntry(ctx context.Context, contentTypeID string) (*ContentEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, eid := range m.entriesByType[contentTypeID] {
		if e, ok := m.entries[eid]; ok && e.DeletedAt == nil {
			clone := *e
			return &clone, nil
		}
	}
	return nil, ErrEntryNotFound
}

func (m *MemoryContentRepository) UpdateEntry(ctx context.Context, e *ContentEntry, expectedDraftVersion int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.entries[e.ID]
	if !ok || existing.DeletedAt != nil {
		return ErrEntryNotFound
	}

	// Optimistic locking verification
	if existing.DraftVersion != expectedDraftVersion {
		return ErrVersionMismatch
	}

	existing.DraftData = e.DraftData
	existing.PublishedData = e.PublishedData
	existing.Status = e.Status
	existing.DraftVersion = expectedDraftVersion + 1
	existing.PublishedVersion = e.PublishedVersion
	existing.UpdatedBy = e.UpdatedBy
	existing.PublishedAt = e.PublishedAt
	existing.UpdatedAt = e.UpdatedAt

	e.DraftVersion = existing.DraftVersion
	return nil
}

func (m *MemoryContentRepository) PublishEntry(ctx context.Context, id string, publishedData map[string]any, publishedVersion int, publishedAt time.Time, updatedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[id]
	if !ok || e.DeletedAt != nil {
		return ErrEntryNotFound
	}

	clone := make(map[string]any, len(publishedData))
	for k, v := range publishedData {
		clone[k] = v
	}

	e.PublishedData = clone
	e.Status = StatusPublished
	e.PublishedVersion = publishedVersion
	e.PublishedAt = &publishedAt
	e.UpdatedBy = updatedBy
	e.UpdatedAt = publishedAt
	return nil
}

func (m *MemoryContentRepository) UnpublishEntry(ctx context.Context, id string, updatedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[id]
	if !ok || e.DeletedAt != nil {
		return ErrEntryNotFound
	}

	now := time.Now().UTC()
	e.PublishedData = nil
	e.Status = StatusDraft
	e.PublishedAt = nil
	e.UpdatedBy = updatedBy
	e.UpdatedAt = now
	return nil
}

func (m *MemoryContentRepository) DeleteEntry(ctx context.Context, id string, hardDelete bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[id]
	if !ok || (!hardDelete && e.DeletedAt != nil) {
		return ErrEntryNotFound
	}

	if hardDelete {
		delete(m.entries, id)
	} else {
		now := time.Now().UTC()
		e.DeletedAt = &now
	}
	return nil
}

func (m *MemoryContentRepository) ListEntries(ctx context.Context, contentTypeID string, filter EntryFilter) ([]*ContentEntry, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matched []*ContentEntry
	for _, eid := range m.entriesByType[contentTypeID] {
		if e, ok := m.entries[eid]; ok && e.DeletedAt == nil {
			if filter.Status != "" && string(e.Status) != filter.Status {
				continue
			}
			if filter.Search != "" && !strings.Contains(strings.ToLower(e.Slug+" "+fmt.Sprint(e.DraftData)), strings.ToLower(filter.Search)) {
				continue
			}
			clone := *e
			matched = append(matched, &clone)
		}
	}

	total := len(matched)
	if filter.Offset > total {
		return []*ContentEntry{}, total, nil
	}

	end := filter.Offset + filter.Limit
	if filter.Limit <= 0 || end > total {
		end = total
	}

	return matched[filter.Offset:end], total, nil
}

// --- ContentVersionRepository ---

func (m *MemoryContentRepository) CreateVersion(ctx context.Context, v *ContentVersion) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clone := *v
	m.versions[v.ID] = &clone
	m.versionsByEnt[v.EntryID] = append(m.versionsByEnt[v.EntryID], v.ID)
	return nil
}

func (m *MemoryContentRepository) ListVersions(ctx context.Context, entryID string) ([]*ContentVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ContentVersion
	vids := m.versionsByEnt[entryID]
	// Return in reverse chronological order
	for i := len(vids) - 1; i >= 0; i-- {
		vid := vids[i]
		if v, ok := m.versions[vid]; ok {
			clone := *v
			result = append(result, &clone)
		}
	}
	return result, nil
}

func (m *MemoryContentRepository) GetVersion(ctx context.Context, entryID string, versionNumber int) (*ContentVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, vid := range m.versionsByEnt[entryID] {
		if v, ok := m.versions[vid]; ok && v.VersionNumber == versionNumber {
			clone := *v
			return &clone, nil
		}
	}
	return nil, ErrVersionNotFound
}

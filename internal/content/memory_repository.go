package content

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
	return m.createTypeLocked(ct, false)
}

func (m *MemoryContentRepository) CreateTypeWithFields(ctx context.Context, ct *ContentType) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createTypeLocked(ct, true)
}

func (m *MemoryContentRepository) createTypeLocked(ct *ContentType, withFields bool) error {
	if _, err := json.Marshal(ct); err != nil {
		return err
	}
	if m.contentTypes[ct.ID] != nil || m.typesByUID[strings.ToLower(ct.UID)] != "" {
		return ErrContentTypeExists
	}
	seen := map[string]bool{}
	if withFields {
		for _, f := range ct.Fields {
			if f.ContentTypeID != ct.ID {
				return fmt.Errorf("field content type mismatch")
			}
			if seen[strings.ToLower(f.FieldKey)] || m.fields[f.ID] != nil {
				return ErrFieldExists
			}
			seen[strings.ToLower(f.FieldKey)] = true
			for _, other := range ct.Fields {
				if other.ID == f.ID && other.FieldKey != f.FieldKey {
					return ErrFieldExists
				}
			}
		}
	}
	clone := persistenceClone(ct)
	clone.Fields = nil
	m.contentTypes[ct.ID] = clone
	m.typesByUID[strings.ToLower(ct.UID)] = ct.ID
	if withFields {
		for _, f := range ct.Fields {
			m.fields[f.ID] = persistenceClone(&f)
			m.fieldsByType[ct.ID] = append(m.fieldsByType[ct.ID], f.ID)
		}
	}
	return nil
}

func (m *MemoryContentRepository) GetTypeByID(ctx context.Context, id string) (*ContentType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ct, ok := m.contentTypes[id]
	if !ok {
		return nil, ErrContentTypeNotFound
	}
	clone := persistenceClone(*ct)
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
	clone := persistenceClone(*ct)
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
	if _, err := json.Marshal(ct.Settings); err != nil {
		return err
	}
	existing.Name = ct.Name
	existing.Settings = persistenceClone(ct.Settings)
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
	for _, fid := range m.fieldsByType[id] {
		delete(m.fields, fid)
	}
	delete(m.fieldsByType, id)
	for _, eid := range m.entriesByType[id] {
		delete(m.entries, eid)
		for _, vid := range m.versionsByEnt[eid] {
			delete(m.versions, vid)
		}
		delete(m.versionsByEnt, eid)
	}
	delete(m.entriesByType, id)
	return nil
}

func (m *MemoryContentRepository) ListTypes(ctx context.Context) ([]*ContentType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ContentType
	for _, ct := range m.contentTypes {
		clone := persistenceClone(*ct)
		clone.Fields = m.listFieldsLocked(ct.ID)
		result = append(result, &clone)
	}
	return result, nil
}

func (m *MemoryContentRepository) CreateField(ctx context.Context, f *ContentField) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.contentTypes[f.ContentTypeID] == nil {
		return ErrContentTypeNotFound
	}
	if _, err := json.Marshal(f); err != nil {
		return err
	}
	if m.fields[f.ID] != nil {
		return ErrFieldExists
	}
	schema := m.listFieldsLocked(f.ContentTypeID)
	for _, existing := range schema {
		if strings.EqualFold(existing.FieldKey, f.FieldKey) {
			return ErrFieldExists
		}
	}
	if err := validatePersistenceUnique(append(schema, *f), m.entriesLocked(f.ContentTypeID)); err != nil {
		return err
	}
	m.fields[f.ID] = persistenceClone(f)
	m.fieldsByType[f.ContentTypeID] = append(m.fieldsByType[f.ContentTypeID], f.ID)
	return nil
}

func (m *MemoryContentRepository) ListFields(ctx context.Context, contentTypeID string) ([]ContentField, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listFieldsLocked(contentTypeID), nil
}

func (m *MemoryContentRepository) UpdateField(ctx context.Context, f *ContentField) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := m.fields[f.ID]
	if existing == nil {
		return ErrFieldNotFound
	}
	if f.ContentTypeID != existing.ContentTypeID || f.FieldKey != existing.FieldKey {
		return fmt.Errorf("field identity cannot change")
	}
	if _, err := json.Marshal(f); err != nil {
		return err
	}
	schema := m.listFieldsLocked(f.ContentTypeID)
	for i := range schema {
		if schema[i].ID == f.ID {
			schema[i] = *f
		}
	}
	if err := validatePersistenceUnique(schema, m.entriesLocked(f.ContentTypeID)); err != nil {
		return err
	}
	clone := persistenceClone(f)
	clone.CreatedAt = existing.CreatedAt
	m.fields[f.ID] = clone
	return nil
}

func (m *MemoryContentRepository) listFieldsLocked(contentTypeID string) []ContentField {
	var result []ContentField
	for _, fid := range m.fieldsByType[contentTypeID] {
		if f, ok := m.fields[fid]; ok {
			result = append(result, persistenceClone(*f))
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
	if _, err := json.Marshal(e); err != nil {
		return err
	}
	if m.entries[e.ID] != nil {
		return ErrVersionMismatch
	}
	if err := m.validateEntryLocked(e); err != nil {
		return err
	}
	m.entries[e.ID] = persistenceClone(e)
	m.entriesByType[e.ContentTypeID] = append(m.entriesByType[e.ContentTypeID], e.ID)
	return nil
}

func (m *MemoryContentRepository) entriesLocked(typeID string) []*ContentEntry {
	var entries []*ContentEntry
	for _, e := range m.entries {
		if e.ContentTypeID == typeID {
			entries = append(entries, e)
		}
	}
	return entries
}

func (m *MemoryContentRepository) validateEntryLocked(e *ContentEntry) error {
	ct := m.contentTypes[e.ContentTypeID]
	if ct == nil {
		return ErrContentTypeNotFound
	}
	entries := []*ContentEntry{e}
	for _, existing := range m.entries {
		if existing.ID == e.ID {
			continue
		}
		if existing.DeletedAt == nil && (ct.Kind == KindSingle && existing.ContentTypeID == e.ContentTypeID ||
			e.SingletonKey != nil && existing.SingletonKey != nil && *e.SingletonKey == *existing.SingletonKey) {
			return ErrSingletonExists
		}
		if existing.ContentTypeID == e.ContentTypeID {
			entries = append(entries, existing)
		}
	}
	return validatePersistenceUnique(m.listFieldsLocked(e.ContentTypeID), entries)
}

func (m *MemoryContentRepository) CommitEntry(ctx context.Context, e *ContentEntry, expected int, version *ContentVersion) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("nil entry")
	}
	next, history, err := prepareEntryCommit(m.entries[e.ID], e, expected, version)
	if err != nil {
		return err
	}
	if m.versions[history.ID] != nil {
		return fmt.Errorf("version ID already exists")
	}
	if err := m.validateEntryLocked(next); err != nil {
		return err
	}
	if m.entries[next.ID] == nil {
		m.entriesByType[next.ContentTypeID] = append(m.entriesByType[next.ContentTypeID], next.ID)
	}
	m.entries[next.ID] = next
	m.versions[history.ID] = history
	m.versionsByEnt[next.ID] = append(m.versionsByEnt[next.ID], history.ID)
	*e = persistenceClone(*next)
	*version = persistenceClone(*history)
	return nil
}

func (m *MemoryContentRepository) GetEntryByID(ctx context.Context, id string) (*ContentEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.entries[id]
	if !ok || e.DeletedAt != nil {
		return nil, ErrEntryNotFound
	}
	clone := persistenceClone(*e)
	return &clone, nil
}

func (m *MemoryContentRepository) GetEntryBySlug(ctx context.Context, contentTypeID, slug string) (*ContentEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, eid := range m.entriesByType[contentTypeID] {
		if e, ok := m.entries[eid]; ok && e.DeletedAt == nil && e.Slug == slug {
			clone := persistenceClone(*e)
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
			clone := persistenceClone(*e)
			return &clone, nil
		}
	}
	return nil, ErrEntryNotFound
}

func (m *MemoryContentRepository) UpdateEntry(ctx context.Context, e *ContentEntry, expectedDraftVersion int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next, _, err := prepareEntryCommit(m.entries[e.ID], e, expectedDraftVersion, &ContentVersion{ID: "legacy", EntryID: e.ID, Action: "update_draft"})
	if err != nil {
		return err
	}
	if err := m.validateEntryLocked(next); err != nil {
		return err
	}
	m.entries[e.ID] = next
	*e = persistenceClone(*next)
	return nil
}

func (m *MemoryContentRepository) PublishEntry(ctx context.Context, id string, publishedData map[string]any, publishedVersion int, publishedAt time.Time, updatedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.entries[id]
	if current == nil || current.DeletedAt != nil {
		return ErrEntryNotFound
	}
	proposed := persistenceClone(current)
	proposed.UpdatedBy = updatedBy
	proposed.UpdatedAt = publishedAt
	proposed.PublishedAt = &publishedAt
	next, _, err := prepareEntryCommit(current, proposed, publishedVersion, &ContentVersion{ID: "legacy", EntryID: id, Action: "publish"})
	if err != nil {
		return err
	}
	if err := m.validateEntryLocked(next); err != nil {
		return err
	}
	m.entries[id] = next
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
	e.PublishedVersion = 0
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
		for _, vid := range m.versionsByEnt[id] {
			delete(m.versions, vid)
		}
		delete(m.versionsByEnt, id)
		ids := m.entriesByType[e.ContentTypeID]
		kept := ids[:0]
		for _, eid := range ids {
			if eid != id {
				kept = append(kept, eid)
			}
		}
		m.entriesByType[e.ContentTypeID] = kept
	} else {
		now := time.Now().UTC()
		e.DeletedAt = &now
		e.SingletonKey = nil
	}
	return nil
}

func (m *MemoryContentRepository) ListEntries(ctx context.Context, contentTypeID string, filter EntryFilter) ([]*ContentEntry, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	matched := []*ContentEntry{}
	for _, e := range m.entriesLocked(contentTypeID) {
		if e.DeletedAt != nil || filter.Status != "" && string(e.Status) != filter.Status {
			continue
		}
		data, err := json.Marshal(e.DraftData)
		if err != nil {
			return nil, 0, err
		}
		if filter.Search != "" && !strings.Contains(strings.ToLower(e.Slug+" "+string(data)), strings.ToLower(filter.Search)) {
			continue
		}
		matched = append(matched, persistenceClone(e))
	}
	sortPersistenceEntries(matched, filter)
	total := len(matched)
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
	end := total
	if limit < total-offset {
		end = offset + limit
	}
	return matched[offset:end], total, nil
}

// --- ContentVersionRepository ---

func (m *MemoryContentRepository) CreateVersion(ctx context.Context, v *ContentVersion) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := json.Marshal(v); err != nil {
		return err
	}
	if m.entries[v.EntryID] == nil {
		return ErrEntryNotFound
	}
	if m.versions[v.ID] != nil {
		return fmt.Errorf("version ID already exists")
	}
	m.versions[v.ID] = persistenceClone(v)
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
			clone := persistenceClone(*v)
			result = append(result, &clone)
		}
	}
	sort.Slice(result, func(i, j int) bool { return persistenceVersionLess(result[i], result[j]) })
	return result, nil
}

func (m *MemoryContentRepository) GetVersion(ctx context.Context, entryID string, versionNumber int) (*ContentVersion, error) {
	versions, err := m.ListVersions(ctx, entryID)
	if err != nil {
		return nil, err
	}
	for _, v := range versions {
		if v.VersionNumber == versionNumber {
			return v, nil
		}
	}
	return nil, ErrVersionNotFound
}

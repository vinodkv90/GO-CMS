package content

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/fields"
	"github.com/google/uuid"
)

var (
	ErrNotPublished   = errors.New("content is not published")
	ErrSingleReadOnly = errors.New("cannot create multiple entries for a single content type")
)

var validUIDRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var nonAlphaNumSlugRegex = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	clean := nonAlphaNumSlugRegex.ReplaceAllString(lower, "-")
	return strings.Trim(clean, "-")
}

// CreateTypeRequest holds input for creating a new content type.
type CreateTypeRequest struct {
	UID      string         `json:"uid"`
	Name     string         `json:"name"`
	Kind     ContentKind    `json:"kind"` // "collection" or "single"
	Settings map[string]any `json:"settings,omitempty"`
	Fields   []ContentField `json:"fields,omitempty"`
}

// UpdateTypeRequest holds input for updating an existing content type.
type UpdateTypeRequest struct {
	Name     string         `json:"name"`
	Settings map[string]any `json:"settings,omitempty"`
}

// Service orchestrates content type building, entries management, draft/publish, and versioning.
type Service struct {
	typeRepo    ContentTypeRepository
	entryRepo   ContentEntryRepository
	versionRepo ContentVersionRepository
	fieldReg    *fields.Registry
	eventBus    *EventBus
	logger      *slog.Logger
}

// NewService creates a new content service instance.
func NewService(
	typeRepo ContentTypeRepository,
	entryRepo ContentEntryRepository,
	versionRepo ContentVersionRepository,
	fieldReg *fields.Registry,
	eventBus *EventBus,
	logger *slog.Logger,
) *Service {
	if fieldReg == nil {
		fieldReg = fields.NewRegistry()
	}
	if eventBus == nil {
		eventBus = NewEventBus()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		typeRepo:    typeRepo,
		entryRepo:   entryRepo,
		versionRepo: versionRepo,
		fieldReg:    fieldReg,
		eventBus:    eventBus,
		logger:      logger,
	}
}

// EventBus returns the content lifecycle event bus.
func (s *Service) EventBus() *EventBus {
	return s.eventBus
}

// --- Content Type Management ---

// CreateContentType creates a new collection or single content type.
func (s *Service) CreateContentType(ctx context.Context, req CreateTypeRequest) (*ContentType, error) {
	req.UID = strings.TrimSpace(strings.ToLower(req.UID))
	if req.UID == "" {
		return nil, errors.New("content type uid is required")
	}
	if !validUIDRegex.MatchString(req.UID) {
		return nil, errors.New("content type uid must be lower-case alphanumeric with hyphens (e.g. 'article', 'product-category')")
	}
	if req.Name == "" {
		return nil, errors.New("content type name is required")
	}
	if req.Kind != KindCollection && req.Kind != KindSingle {
		req.Kind = KindCollection
	}

	now := time.Now().UTC()
	typeID := uuid.New().String()

	ct := &ContentType{
		ID:        typeID,
		UID:       req.UID,
		Name:      req.Name,
		Kind:      req.Kind,
		Settings:  req.Settings,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.typeRepo.CreateType(ctx, ct); err != nil {
		return nil, err
	}

	// Register initial fields if provided
	for _, f := range req.Fields {
		f.ID = uuid.New().String()
		f.ContentTypeID = typeID
		f.CreatedAt = now
		f.FieldKey = strings.TrimSpace(strings.ToLower(f.FieldKey))

		if !s.fieldReg.IsValid(f.FieldType) {
			return nil, fmt.Errorf("invalid field type %q for field %q", f.FieldType, f.FieldKey)
		}

		if err := s.typeRepo.CreateField(ctx, &f); err != nil {
			return nil, fmt.Errorf("failed to add field %q: %w", f.FieldKey, err)
		}
	}

	return s.typeRepo.GetTypeByUID(ctx, req.UID)
}

// GetContentType retrieves a content type definition by its UID.
func (s *Service) GetContentType(ctx context.Context, uid string) (*ContentType, error) {
	return s.typeRepo.GetTypeByUID(ctx, uid)
}

// ListContentTypes returns all registered content types.
func (s *Service) ListContentTypes(ctx context.Context) ([]*ContentType, error) {
	return s.typeRepo.ListTypes(ctx)
}

// UpdateContentType updates content type metadata.
func (s *Service) UpdateContentType(ctx context.Context, uid string, req UpdateTypeRequest) (*ContentType, error) {
	ct, err := s.typeRepo.GetTypeByUID(ctx, uid)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		ct.Name = req.Name
	}
	if req.Settings != nil {
		ct.Settings = req.Settings
	}
	ct.UpdatedAt = time.Now().UTC()

	if err := s.typeRepo.UpdateType(ctx, ct); err != nil {
		return nil, err
	}

	return s.typeRepo.GetTypeByUID(ctx, uid)
}

// DeleteContentType deletes a content type and all associated fields.
func (s *Service) DeleteContentType(ctx context.Context, uid string) error {
	ct, err := s.typeRepo.GetTypeByUID(ctx, uid)
	if err != nil {
		return err
	}
	return s.typeRepo.DeleteType(ctx, ct.ID)
}

// AddField adds a new field to an existing content type.
func (s *Service) AddField(ctx context.Context, typeUID string, f ContentField) error {
	ct, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return err
	}

	f.FieldKey = strings.TrimSpace(strings.ToLower(f.FieldKey))
	if f.FieldKey == "" || f.Name == "" {
		return errors.New("field key and name are required")
	}

	if !s.fieldReg.IsValid(f.FieldType) {
		return fmt.Errorf("unsupported field type: %s", f.FieldType)
	}

	f.ID = uuid.New().String()
	f.ContentTypeID = ct.ID
	f.CreatedAt = time.Now().UTC()

	return s.typeRepo.CreateField(ctx, &f)
}

// DeleteField removes a field definition from a content type without dropping entry data.
func (s *Service) DeleteField(ctx context.Context, typeUID, fieldKey string) error {
	ct, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return err
	}

	for _, f := range ct.Fields {
		if strings.EqualFold(f.FieldKey, fieldKey) {
			return s.typeRepo.DeleteField(ctx, f.ID)
		}
	}
	return ErrFieldNotFound
}

// --- Content Entries & Manager ---

// validateEntryData validates a data payload against the content type's field rules.
func (s *Service) validateEntryData(ct *ContentType, data map[string]any) error {
	for _, f := range ct.Fields {
		val := data[f.FieldKey]
		def := fields.FieldDefinition{
			Name:  f.FieldKey,
			Label: f.Name,
			Type:  f.FieldType,
			Rules: f.Configuration,
		}
		if err := fields.ValidateValue(def, val); err != nil {
			return err
		}
	}
	return nil
}

// CreateEntry creates a new content entry in draft status.
func (s *Service) CreateEntry(ctx context.Context, typeUID string, data map[string]any, userID string) (*ContentEntry, error) {
	ct, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return nil, err
	}

	// Validate data against content type schema
	if err := s.validateEntryData(ct, data); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	entryID := uuid.New().String()

	var singletonKey *string
	if ct.Kind == KindSingle {
		// Single types enforce strictly one entry
		key := ct.ID
		singletonKey = &key
	}

	// Extract optional slug if present or derive from title
	slug := ""
	if sVal, ok := data["slug"].(string); ok && sVal != "" {
		slug = sVal
	} else if tVal, ok := data["title"].(string); ok && tVal != "" {
		slug = slugify(tVal)
	}

	entry := &ContentEntry{
		ID:               entryID,
		ContentTypeID:    ct.ID,
		ContentTypeUID:   ct.UID,
		Slug:             slug,
		DraftData:        data,
		Status:           StatusDraft,
		DraftVersion:     1,
		PublishedVersion: 0,
		SingletonKey:     singletonKey,
		CreatedBy:        userID,
		UpdatedBy:        userID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// 1. Emit BeforeCreate lifecycle hook
	if err := s.eventBus.Emit(ctx, EventBeforeCreate, entry); err != nil {
		return nil, err
	}

	// 2. Persist Entry
	if err := s.entryRepo.CreateEntry(ctx, entry); err != nil {
		return nil, err
	}

	// 3. Record Initial Version Snapshot
	version := &ContentVersion{
		ID:            uuid.New().String(),
		EntryID:       entryID,
		VersionNumber: 1,
		Snapshot:      data,
		Action:        "create_draft",
		ChangedBy:     userID,
		CreatedAt:     now,
	}
	_ = s.versionRepo.CreateVersion(ctx, version)

	// 4. Emit AfterCreate lifecycle hook
	_ = s.eventBus.Emit(ctx, EventAfterCreate, entry)

	return entry, nil
}

// UpdateEntry updates an entry's draft data with optimistic concurrency control.
func (s *Service) UpdateEntry(ctx context.Context, typeUID, entryID string, data map[string]any, expectedDraftVersion int, userID string) (*ContentEntry, error) {
	ct, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return nil, err
	}

	existing, err := s.entryRepo.GetEntryByID(ctx, entryID)
	if err != nil {
		return nil, err
	}

	// Validate new data
	if err := s.validateEntryData(ct, data); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	existing.DraftData = data
	existing.UpdatedBy = userID
	existing.UpdatedAt = now

	if sVal, ok := data["slug"].(string); ok && sVal != "" {
		existing.Slug = sVal
	} else if existing.Slug == "" {
		if tVal, ok := data["title"].(string); ok && tVal != "" {
			existing.Slug = slugify(tVal)
		}
	}

	// 1. Emit BeforeUpdate lifecycle hook
	if err := s.eventBus.Emit(ctx, EventBeforeUpdate, existing); err != nil {
		return nil, err
	}

	// 2. Persist update with version verification
	if err := s.entryRepo.UpdateEntry(ctx, existing, expectedDraftVersion); err != nil {
		return nil, err
	}

	// 3. Create historical version snapshot
	version := &ContentVersion{
		ID:            uuid.New().String(),
		EntryID:       entryID,
		VersionNumber: existing.DraftVersion,
		Snapshot:      data,
		Action:        "update_draft",
		ChangedBy:     userID,
		CreatedAt:     now,
	}
	_ = s.versionRepo.CreateVersion(ctx, version)

	// 4. Emit AfterUpdate hook
	_ = s.eventBus.Emit(ctx, EventAfterUpdate, existing)

	return existing, nil
}

// PublishEntry promotes the current draft_data to published_data.
func (s *Service) PublishEntry(ctx context.Context, typeUID, entryID string, userID string) (*ContentEntry, error) {
	_, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return nil, err
	}

	entry, err := s.entryRepo.GetEntryByID(ctx, entryID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	// Clone draft data to published data
	pubData := make(map[string]any, len(entry.DraftData))
	for k, v := range entry.DraftData {
		pubData[k] = v
	}

	entry.PublishedData = pubData
	entry.Status = StatusPublished
	entry.PublishedVersion = entry.DraftVersion
	entry.PublishedAt = &now
	entry.UpdatedBy = userID
	entry.UpdatedAt = now

	// 1. Emit BeforePublish hook
	if err := s.eventBus.Emit(ctx, EventBeforePublish, entry); err != nil {
		return nil, err
	}

	// 2. Persist Entry using dedicated publish operation
	if err := s.entryRepo.PublishEntry(ctx, entry.ID, pubData, entry.DraftVersion, now, userID); err != nil {
		return nil, err
	}

	// 3. Record Version
	version := &ContentVersion{
		ID:            uuid.New().String(),
		EntryID:       entryID,
		VersionNumber: entry.DraftVersion,
		Snapshot:      pubData,
		Action:        "publish",
		ChangedBy:     userID,
		CreatedAt:     now,
	}
	_ = s.versionRepo.CreateVersion(ctx, version)

	// 4. Emit AfterPublish hook
	_ = s.eventBus.Emit(ctx, EventAfterPublish, entry)

	return entry, nil
}

// UnpublishEntry sets the status to draft and removes the live published_data.
func (s *Service) UnpublishEntry(ctx context.Context, typeUID, entryID string, userID string) (*ContentEntry, error) {
	_, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return nil, err
	}

	entry, err := s.entryRepo.GetEntryByID(ctx, entryID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	entry.PublishedData = nil
	entry.Status = StatusDraft
	entry.PublishedAt = nil
	entry.UpdatedBy = userID
	entry.UpdatedAt = now

	if err := s.entryRepo.UnpublishEntry(ctx, entry.ID, userID); err != nil {
		return nil, err
	}

	version := &ContentVersion{
		ID:            uuid.New().String(),
		EntryID:       entryID,
		VersionNumber: entry.DraftVersion,
		Snapshot:      entry.DraftData,
		Action:        "unpublish",
		ChangedBy:     userID,
		CreatedAt:     now,
	}
	_ = s.versionRepo.CreateVersion(ctx, version)

	return entry, nil
}

// GetEntry retrieves a single entry by ID. If previewDraft is false, returns published view only.
func (s *Service) GetEntry(ctx context.Context, typeUID, entryID string, previewDraft bool) (*ContentEntry, error) {
	_, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return nil, err
	}

	entry, err := s.entryRepo.GetEntryByID(ctx, entryID)
	if err != nil {
		return nil, err
	}

	if !previewDraft {
		if entry.Status != StatusPublished || entry.PublishedData == nil {
			return nil, ErrNotPublished
		}
	}

	return entry, nil
}

// GetEntryBySlug retrieves a single entry by its slug.
func (s *Service) GetEntryBySlug(ctx context.Context, typeUID, slug string, previewDraft bool) (*ContentEntry, error) {
	ct, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return nil, err
	}

	entry, err := s.entryRepo.GetEntryBySlug(ctx, ct.ID, slug)
	if err != nil {
		return nil, err
	}

	if !previewDraft {
		if entry.Status != StatusPublished || entry.PublishedData == nil {
			return nil, ErrNotPublished
		}
	}

	return entry, nil
}

// GetSingleEntry retrieves the singleton entry for a single content type.
func (s *Service) GetSingleEntry(ctx context.Context, typeUID string, previewDraft bool) (*ContentEntry, error) {
	ct, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return nil, err
	}

	entry, err := s.entryRepo.GetSingleEntry(ctx, ct.ID)
	if err != nil {
		return nil, err
	}

	if !previewDraft {
		if entry.Status != StatusPublished || entry.PublishedData == nil {
			return nil, ErrNotPublished
		}
	}

	return entry, nil
}

// ListEntries queries entries for a collection content type.
func (s *Service) ListEntries(ctx context.Context, typeUID string, filter EntryFilter, previewDraft bool) ([]*ContentEntry, int, error) {
	ct, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return nil, 0, err
	}

	if !previewDraft {
		// Non-preview/public queries are strictly locked to published content
		filter.Status = string(StatusPublished)
	}

	return s.entryRepo.ListEntries(ctx, ct.ID, filter)
}

// DeleteEntry deletes an entry (soft delete by default).
func (s *Service) DeleteEntry(ctx context.Context, typeUID, entryID string, hardDelete bool, userID string) error {
	entry, err := s.entryRepo.GetEntryByID(ctx, entryID)
	if err != nil {
		return err
	}

	// 1. Emit BeforeDelete hook
	if err := s.eventBus.Emit(ctx, EventBeforeDelete, entry); err != nil {
		return err
	}

	// 2. Perform deletion
	if err := s.entryRepo.DeleteEntry(ctx, entryID, hardDelete); err != nil {
		return err
	}

	// 3. Emit AfterDelete hook
	_ = s.eventBus.Emit(ctx, EventAfterDelete, entry)

	return nil
}

// ListVersions returns the version history of an entry.
func (s *Service) ListVersions(ctx context.Context, typeUID, entryID string) ([]*ContentVersion, error) {
	_, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return nil, err
	}
	return s.versionRepo.ListVersions(ctx, entryID)
}

// RestoreVersion restores an entry's draft_data from an earlier snapshot version.
func (s *Service) RestoreVersion(ctx context.Context, typeUID, entryID string, versionNumber int, userID string) (*ContentEntry, error) {
	entry, err := s.entryRepo.GetEntryByID(ctx, entryID)
	if err != nil {
		return nil, err
	}

	ver, err := s.versionRepo.GetVersion(ctx, entryID, versionNumber)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	entry.DraftData = ver.Snapshot
	entry.UpdatedBy = userID
	entry.UpdatedAt = now

	// Optimistic update
	if err := s.entryRepo.UpdateEntry(ctx, entry, entry.DraftVersion); err != nil {
		return nil, err
	}

	// Record restore as a new version snapshot
	newVer := &ContentVersion{
		ID:            uuid.New().String(),
		EntryID:       entryID,
		VersionNumber: entry.DraftVersion,
		Snapshot:      ver.Snapshot,
		Action:        fmt.Sprintf("restore_from_v%d", versionNumber),
		ChangedBy:     userID,
		CreatedAt:     now,
	}
	_ = s.versionRepo.CreateVersion(ctx, newVer)

	return entry, nil
}

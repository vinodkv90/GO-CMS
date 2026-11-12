package content

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
	"unicode"

	"github.com/go-cms/cms/internal/fields"
	"github.com/google/uuid"
)

var (
	ErrNotPublished   = errors.New("content is not published")
	ErrSingleReadOnly = errors.New("cannot create multiple entries for a single content type")
)

var validUIDRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var nonAlphaNumSlugRegex = regexp.MustCompile(`[^a-z0-9]+`)

var errUniqueField = errors.New("unique field value already exists")

func slugify(s string) string {
	var ascii strings.Builder
	for _, r := range norm.NFKD.String(strings.ToLower(strings.TrimSpace(s))) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			ascii.WriteRune(r)
		} else {
			ascii.WriteByte('-')
		}
	}
	return strings.Trim(nonAlphaNumSlugRegex.ReplaceAllString(ascii.String(), "-"), "-")
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
	mediaLookup func(context.Context, string) (string, error)
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

// FieldCapabilities returns supported configuration keys for built-in and active custom field types.
func (s *Service) FieldCapabilities() map[fields.FieldType]fields.FieldCapabilities {
	if s.fieldReg == nil {
		return fields.Capabilities()
	}
	return s.fieldReg.Capabilities()
}

// CustomFields returns metadata descriptors for active plugin-registered custom field types.
func (s *Service) CustomFields() []fields.CustomFieldDescriptor {
	if s.fieldReg == nil {
		return []fields.CustomFieldDescriptor{}
	}
	return s.fieldReg.CustomFields()
}

func wrapFieldError(fieldPath string, err error) error {
	if err == nil {
		return nil
	}
	var fe fields.FieldError
	if errors.As(err, &fe) {
		msg := fe.Message
		if msg == "" {
			msg = err.Error()
		}
		return fields.FieldError{Field: fieldPath, Message: msg}
	}
	return fields.FieldError{Field: fieldPath, Message: err.Error()}
}

func asFieldError(err error) (fields.FieldError, bool) {
	var fe fields.FieldError
	if errors.As(err, &fe) {
		return fe, true
	}
	return fields.FieldError{}, false
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
	if (req.Kind == KindComponent && !validComponentUID.MatchString(req.UID)) || (req.Kind != KindComponent && !validUIDRegex.MatchString(req.UID)) {
		return nil, errors.New("content type uid must be lower-case alphanumeric with hyphens (e.g. 'article', 'product-category')")
	}
	if req.Name == "" {
		return nil, errors.New("content type name is required")
	}
	if req.Kind == "" {
		req.Kind = KindCollection
	}
	if req.Kind != KindCollection && req.Kind != KindSingle && req.Kind != KindComponent {
		return nil, errors.New("invalid content type kind")
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

	// Register initial fields if provided
	for _, f := range req.Fields {
		f.ID = uuid.New().String()
		f.ContentTypeID = typeID
		f.CreatedAt = now
		f.FieldKey = strings.TrimSpace(strings.ToLower(f.FieldKey))

		ct.Fields = append(ct.Fields, f)
	}
	if err := s.validateSchema(ctx, ct); err != nil {
		return nil, err
	}
	if err := s.createTypeWithFields(ctx, ct); err != nil {
		return nil, err
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
	if err := s.ensureTypeUnreferenced(ctx, ct); err != nil {
		return err
	}
	entries, total, err := s.entryRepo.ListEntries(ctx, ct.ID, EntryFilter{Limit: 1})
	if err != nil { return err }
	if total > 0 || len(entries) > 0 { return fmt.Errorf("%w: content type %s still has entries", ErrReferenced, ct.UID) }
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
	proposed := *ct
	proposed.Fields = append(append([]ContentField(nil), ct.Fields...), f)
	if err := s.validateSchema(ctx, &proposed); err != nil {
		return err
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
			proposed := *ct
			proposed.Fields = nil
			for _, sibling := range ct.Fields {
				if sibling.ID != f.ID {
					proposed.Fields = append(proposed.Fields, sibling)
				}
			}
			if err := s.validateSchema(ctx, &proposed); err != nil {
				return err
			}
			return s.typeRepo.DeleteField(ctx, f.ID)
		}
	}
	return ErrFieldNotFound
}

// UpdateField updates a field definition without changing its key or position.
func (s *Service) UpdateField(ctx context.Context, typeUID, fieldKey string, updated ContentField) error {
	ct, err := s.typeRepo.GetTypeByUID(ctx, typeUID)
	if err != nil {
		return err
	}
	for _, field := range ct.Fields {
		if strings.EqualFold(field.FieldKey, fieldKey) {
			updated.ID = field.ID
			updated.ContentTypeID = field.ContentTypeID
			updated.FieldKey = field.FieldKey
			updated.CreatedAt = field.CreatedAt
			if !s.fieldReg.IsValid(updated.FieldType) {
				return fmt.Errorf("unsupported field type: %s", updated.FieldType)
			}
			proposed := *ct
			proposed.Fields = append([]ContentField(nil), ct.Fields...)
			for i := range proposed.Fields {
				if proposed.Fields[i].ID == field.ID {
					proposed.Fields[i] = updated
				}
			}
			if err := s.validateSchema(ctx, &proposed); err != nil {
				return err
			}
			return s.typeRepo.UpdateField(ctx, &updated)
		}
	}
	return ErrFieldNotFound
}

// --- Content Entries & Manager ---

// validateEntryData validates a data payload against the content type's field rules.
func (s *Service) validateEntryData(ctx context.Context, ct *ContentType, data map[string]any) error {
	for _, f := range ct.Fields {
		val := data[f.FieldKey]
		def := fields.FieldDefinition{
			Name:  f.FieldKey,
			Label: f.Name,
			Type:  f.FieldType,
			Rules: f.Configuration,
		}
		if err := s.fieldReg.ValidateField(def, val); err != nil {
			return wrapFieldError(f.FieldKey, err)
		}
		if f.FieldType == fields.TypeSingleRelation || f.FieldType == fields.TypeMultipleRelation {
			if err := s.validateRelationValue(ctx, f, val); err != nil {
				return wrapFieldError(f.FieldKey, err)
			}
		}
	}
	return nil
}

func (s *Service) validateRelationValue(ctx context.Context, field ContentField, value any) error {
	if value == nil || value == "" {
		return nil
	}
	target, err := s.typeRepo.GetTypeByUID(ctx, field.Configuration.TargetType)
	if err != nil {
		return fmt.Errorf("relation %q target content type %q does not exist", field.FieldKey, field.Configuration.TargetType)
	}
	ids := []string{}
	if field.FieldType == fields.TypeSingleRelation {
		if id, ok := value.(string); ok {
			ids = append(ids, id)
		}
	} else {
		switch values := value.(type) {
		case []string:
			ids = append(ids, values...)
		case []any:
			for _, item := range values {
				if id, ok := item.(string); ok {
					ids = append(ids, id)
				}
			}
		}
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("relation %q contains duplicate entry %q", field.FieldKey, id)
		}
		seen[id] = struct{}{}
		entry, lookupErr := s.entryRepo.GetEntryByID(ctx, id)
		if lookupErr != nil || entry.ContentTypeID != target.ID {
			return fmt.Errorf("relation %q references an invalid target entry", field.FieldKey)
		}
	}
	return nil
}

func validateFieldConfiguration(f ContentField, siblingFields []ContentField) error {
	c := f.Configuration
	if f.FieldType == fields.TypeUID {
		// Legacy schemas may have a UID without a source field; retain their
		// existing title-based fallback while new schemas can opt into binding.
		if c.TargetField == "" {
			return nil
		}
		if strings.EqualFold(c.TargetField, f.FieldKey) {
			return fmt.Errorf("UID field %q cannot reference itself", f.FieldKey)
		}
		found := false
		for _, sibling := range siblingFields {
			if strings.EqualFold(sibling.FieldKey, c.TargetField) {
				found = true
				if sibling.FieldType != fields.TypeShortText && sibling.FieldType != fields.TypeLongText && sibling.FieldType != fields.TypeRichText {
					return fmt.Errorf("UID source field %q must be text-compatible", c.TargetField)
				}
				break
			}
		}
		if !found {
			return fmt.Errorf("UID source field %q does not exist", c.TargetField)
		}
	}
	if (f.FieldType == fields.TypeSingleRelation || f.FieldType == fields.TypeMultipleRelation) && c.TargetType == "" {
		return fmt.Errorf("relation field %q requires target_type configuration", f.FieldKey)
	}
	if f.FieldType == fields.TypeMedia && !c.Multiple && c.MaxItems != nil && *c.MaxItems > 1 {
		return fmt.Errorf("single media field %q cannot allow more than one item", f.FieldKey)
	}
	if (f.FieldType == fields.TypeComponent || f.FieldType == fields.TypeRepeatableComponent) && c.Component == "" {
		return fmt.Errorf("component field %q requires component configuration", f.FieldKey)
	}
	if f.FieldType == fields.TypeDynamicZone && len(c.Components) == 0 {
		return fmt.Errorf("dynamic zone field %q requires at least one component", f.FieldKey)
	}
	return nil
}

func (s *Service) validateUniqueFields(ctx context.Context, ct *ContentType, data map[string]any, currentID string) error {
	entries, _, err := s.entryRepo.ListEntries(ctx, ct.ID, EntryFilter{Limit: 10000})
	if err != nil {
		return err
	}
	for _, f := range ct.Fields {
		if !f.Configuration.Unique {
			continue
		}
		value, exists := data[f.FieldKey]
		if !exists || value == nil || value == "" {
			continue
		}
		for _, entry := range entries {
			if entry.ID != currentID && fmt.Sprint(entry.DraftData[f.FieldKey]) == fmt.Sprint(value) {
				return fmt.Errorf("%w: %s", errUniqueField, f.FieldKey)
			}
		}
	}
	return nil
}

// applyUIDFields materializes generated UID values into the JSON document so
// REST responses, snapshots, and uniqueness checks all observe the same value.
func applyUIDFields(ct *ContentType, data map[string]any) {
	for _, f := range ct.Fields {
		if f.FieldType != fields.TypeUID || f.Configuration.TargetField == "" {
			continue
		}
		if current, ok := data[f.FieldKey].(string); ok && strings.TrimSpace(current) != "" {
			continue
		}
		if source, ok := data[f.Configuration.TargetField].(string); ok && strings.TrimSpace(source) != "" {
			data[f.FieldKey] = slugify(source)
		}
	}
}

// CreateEntry creates a new content entry in draft status.
func (s *Service) CreateEntry(ctx context.Context, typeUID string, data map[string]any, userID string) (*ContentEntry, error) {
	ct, err := s.entryType(ctx, typeUID)
	if err != nil {
		return nil, err
	}
	data, err = s.prepareData(ctx, ct, data, nil, 0, "", false, true)
	if err != nil {
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
	} else {
		for _, f := range ct.Fields {
			if f.FieldType == fields.TypeUID {
				if value, ok := data[f.FieldKey].(string); ok {
					slug = value
					break
				}
			}
		}
		if slug == "" {
			if tVal, ok := data["title"].(string); ok && tVal != "" {
				slug = slugify(tVal)
			}
		}
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
	version := &ContentVersion{
		ID:            uuid.New().String(),
		EntryID:       entryID,
		VersionNumber: 1,
		Snapshot:      cloneData(data),
		Action:        "create_draft",
		ChangedBy:     userID,
		CreatedAt:     now,
	}
	if err := s.commitEntry(ctx, entry, 0, version); err != nil {
		return nil, err
	}

	// 4. Emit AfterCreate lifecycle hook
	_ = s.eventBus.Emit(ctx, EventAfterCreate, entry)

	return entry, nil
}

// UpdateEntry updates an entry's draft data with optimistic concurrency control.
func (s *Service) UpdateEntry(ctx context.Context, typeUID, entryID string, data map[string]any, expectedDraftVersion int, userID string) (*ContentEntry, error) {
	ct, existing, err := s.ownedEntry(ctx, typeUID, entryID)
	if err != nil {
		return nil, err
	}
	data, err = s.prepareData(ctx, ct, data, existing.DraftData, 0, "", false, true)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	existing.DraftData = data
	existing.UpdatedBy = userID
	existing.UpdatedAt = now

	if sVal, ok := data["slug"].(string); ok && sVal != "" {
		existing.Slug = sVal
	} else {
		for _, f := range ct.Fields {
			if f.FieldType == fields.TypeUID {
				if value, ok := data[f.FieldKey].(string); ok {
					existing.Slug = value
					break
				}
			}
		}
		if existing.Slug == "" {
			if tVal, ok := data["title"].(string); ok && tVal != "" {
				existing.Slug = slugify(tVal)
			}
		}
	}

	// 1. Emit BeforeUpdate lifecycle hook
	if err := s.eventBus.Emit(ctx, EventBeforeUpdate, existing); err != nil {
		return nil, err
	}

	// 2. Persist update with version verification
	version := &ContentVersion{
		ID:            uuid.New().String(),
		EntryID:       entryID,
		VersionNumber: existing.DraftVersion,
		Snapshot:      cloneData(data),
		Action:        "update_draft",
		ChangedBy:     userID,
		CreatedAt:     now,
	}
	if err := s.commitEntry(ctx, existing, expectedDraftVersion, version); err != nil {
		return nil, err
	}

	// 4. Emit AfterUpdate hook
	_ = s.eventBus.Emit(ctx, EventAfterUpdate, existing)

	return existing, nil
}

// PublishEntry promotes the current draft_data to published_data.
func (s *Service) PublishEntry(ctx context.Context, typeUID, entryID string, userID string) (*ContentEntry, error) {
	ct, entry, err := s.ownedEntry(ctx, typeUID, entryID)
	if err != nil {
		return nil, err
	}
	if err := s.validateSchema(ctx, ct); err != nil {
		return nil, err
	}
	entry.DraftData, err = s.prepareData(ctx, ct, entry.DraftData, nil, 0, "", true, false)
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

	// Persist the publication and its immutable snapshot atomically when supported.
	version := &ContentVersion{
		ID:            uuid.New().String(),
		EntryID:       entryID,
		VersionNumber: entry.DraftVersion,
		Snapshot:      pubData,
		Action:        "publish",
		ChangedBy:     userID,
		CreatedAt:     now,
	}
	if err := s.commitEntry(ctx, entry, entry.DraftVersion, version); err != nil {
		return nil, err
	}

	// 4. Emit AfterPublish hook
	_ = s.eventBus.Emit(ctx, EventAfterPublish, entry)

	return entry, nil
}

// UnpublishEntry sets the status to draft and removes the live published_data.
func (s *Service) UnpublishEntry(ctx context.Context, typeUID, entryID string, userID string) (*ContentEntry, error) {
	_, entry, err := s.ownedEntry(ctx, typeUID, entryID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	entry.PublishedData = nil
	entry.Status = StatusDraft
	entry.PublishedAt = nil
	entry.UpdatedBy = userID
	entry.UpdatedAt = now

	version := &ContentVersion{
		ID:            uuid.New().String(),
		EntryID:       entryID,
		VersionNumber: entry.DraftVersion,
		Snapshot:      entry.DraftData,
		Action:        "unpublish",
		ChangedBy:     userID,
		CreatedAt:     now,
	}
	if err := s.commitEntry(ctx, entry, entry.DraftVersion, version); err != nil {
		return nil, err
	}

	return entry, nil
}

// GetEntry retrieves a single entry by ID. If previewDraft is false, returns published view only.
func (s *Service) GetEntry(ctx context.Context, typeUID, entryID string, previewDraft bool) (*ContentEntry, error) {
	_, entry, err := s.ownedEntry(ctx, typeUID, entryID)
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
	ct, err := s.entryType(ctx, typeUID)
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
	ct, err := s.entryType(ctx, typeUID)
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
	ct, err := s.entryType(ctx, typeUID)
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
	_, entry, err := s.ownedEntry(ctx, typeUID, entryID)
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
	_, _, err := s.ownedEntry(ctx, typeUID, entryID)
	if err != nil {
		return nil, err
	}
	return s.versionRepo.ListVersions(ctx, entryID)
}

// RestoreVersion restores an entry's draft_data from an earlier snapshot version.
func (s *Service) RestoreVersion(ctx context.Context, typeUID, entryID string, versionNumber int, userID string) (*ContentEntry, error) {
	ct, entry, err := s.ownedEntry(ctx, typeUID, entryID)
	if err != nil {
		return nil, err
	}

	ver, err := s.versionRepo.GetVersion(ctx, entryID, versionNumber)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	prepared, err := s.prepareData(ctx, ct, ver.Snapshot, entry.DraftData, 0, "", true, false)
	if err != nil {
		return nil, err
	}
	entry.DraftData = prepared
	entry.UpdatedBy = userID
	entry.UpdatedAt = now

	newVer := &ContentVersion{
		ID:            uuid.New().String(),
		EntryID:       entryID,
		VersionNumber: entry.DraftVersion,
		Snapshot:      ver.Snapshot,
		Action:        "restore",
		ChangedBy:     userID,
		CreatedAt:     now,
	}
	if err := s.commitEntry(ctx, entry, entry.DraftVersion, newVer); err != nil {
		return nil, err
	}

	return entry, nil
}

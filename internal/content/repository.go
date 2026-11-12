package content

import (
	"context"
	"errors"
	"time"
)

// AtomicContentTypeRepository optionally creates a schema and all its fields as
// one operation. Implementations must leave no partial schema on failure.
type AtomicContentTypeRepository interface {
	CreateTypeWithFields(context.Context, *ContentType) error
}

// AtomicContentEntryRepository commits an entry mutation and its history record
// together. expectedDraftVersion is zero for create_draft and positive otherwise.
// update_draft/restore increment the draft version and preserve publication state;
// publish/unpublish check the draft version but do not increment it. On success
// entry contains the stored state and version contains the stored version number.
// A non-nil version with the matching EntryID and supported Action is required.
type AtomicContentEntryRepository interface {
	CommitEntry(context.Context, *ContentEntry, int, *ContentVersion) error
}

var (
	ErrContentTypeNotFound = errors.New("content type not found")
	ErrContentTypeExists   = errors.New("content type with this UID already exists")
	ErrFieldNotFound       = errors.New("content field not found")
	ErrFieldExists         = errors.New("field with this key already exists in content type")
	ErrEntryNotFound       = errors.New("content entry not found")
	ErrSingletonExists     = errors.New("singleton entry already exists for this single content type")
	ErrVersionMismatch     = errors.New("optimistic concurrency conflict: content entry was modified by another user")
	ErrVersionNotFound     = errors.New("content version not found")
)

// ContentTypeRepository manages content type definitions and fields.
type ContentTypeRepository interface {
	CreateType(ctx context.Context, ct *ContentType) error
	GetTypeByID(ctx context.Context, id string) (*ContentType, error)
	GetTypeByUID(ctx context.Context, uid string) (*ContentType, error)
	UpdateType(ctx context.Context, ct *ContentType) error
	DeleteType(ctx context.Context, id string) error
	ListTypes(ctx context.Context) ([]*ContentType, error)

	CreateField(ctx context.Context, field *ContentField) error
	UpdateField(ctx context.Context, field *ContentField) error
	ListFields(ctx context.Context, contentTypeID string) ([]ContentField, error)
	DeleteField(ctx context.Context, fieldID string) error
}

// ContentEntryRepository manages content entry data and queries.
type ContentEntryRepository interface {
	CreateEntry(ctx context.Context, entry *ContentEntry) error
	GetEntryByID(ctx context.Context, id string) (*ContentEntry, error)
	GetEntryBySlug(ctx context.Context, contentTypeID, slug string) (*ContentEntry, error)
	GetSingleEntry(ctx context.Context, contentTypeID string) (*ContentEntry, error)
	UpdateEntry(ctx context.Context, entry *ContentEntry, expectedDraftVersion int) error
	PublishEntry(ctx context.Context, id string, publishedData map[string]any, publishedVersion int, publishedAt time.Time, updatedBy string) error
	UnpublishEntry(ctx context.Context, id string, updatedBy string) error
	DeleteEntry(ctx context.Context, id string, hardDelete bool) error
	ListEntries(ctx context.Context, contentTypeID string, filter EntryFilter) ([]*ContentEntry, int, error)
}

// ContentVersionRepository manages historical versions of entries.
type ContentVersionRepository interface {
	CreateVersion(ctx context.Context, v *ContentVersion) error
	ListVersions(ctx context.Context, entryID string) ([]*ContentVersion, error)
	GetVersion(ctx context.Context, entryID string, versionNumber int) (*ContentVersion, error)
}

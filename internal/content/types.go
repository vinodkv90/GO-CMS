package content

import (
	"time"

	"github.com/go-cms/cms/internal/fields"
)

// ContentKind indicates whether a content type holds multiple records or a single singleton record.
type ContentKind string

const (
	KindCollection ContentKind = "collection"
	KindSingle     ContentKind = "single"
)

// ContentStatus represents the publication state of a content entry.
type ContentStatus string

const (
	StatusDraft     ContentStatus = "draft"
	StatusPublished ContentStatus = "published"
	StatusArchived  ContentStatus = "archived"
)

// ContentField represents a field defined within a content type.
type ContentField struct {
	ID            string                 `json:"id" bson:"_id"`
	ContentTypeID string                 `json:"content_type_id" bson:"content_type_id"`
	Name          string                 `json:"name" bson:"name"`
	FieldKey      string                 `json:"field_key" bson:"field_key"`
	FieldType     fields.FieldType       `json:"field_type" bson:"field_type"`
	Configuration fields.ValidationRules `json:"configuration" bson:"configuration"`
	CreatedAt     time.Time              `json:"created_at" bson:"created_at"`
}

// ContentType represents a dynamic schema definition.
type ContentType struct {
	ID        string         `json:"id" bson:"_id"`
	UID       string         `json:"uid" bson:"uid"`   // e.g. "article", "product", "homepage"
	Name      string         `json:"name" bson:"name"` // e.g. "Articles", "Homepage"
	Kind      ContentKind    `json:"kind" bson:"kind"` // "collection" or "single"
	Settings  map[string]any `json:"settings,omitempty" bson:"settings,omitempty"`
	Fields    []ContentField `json:"fields,omitempty" bson:"fields,omitempty"`
	CreatedAt time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" bson:"updated_at"`
}

// ContentEntry represents an instance of content data.
// In accordance with Strapi-like draft/publish architecture, draft_data and published_data are
// maintained separately so editing a published entry does not prematurely alter the live public view.
type ContentEntry struct {
	ID               string         `json:"id" bson:"_id"`
	ContentTypeID    string         `json:"content_type_id" bson:"content_type_id"`
	Slug             string         `json:"slug,omitempty" bson:"slug,omitempty"`
	DraftData        map[string]any `json:"draft_data" bson:"draft_data"`
	PublishedData    map[string]any `json:"published_data,omitempty" bson:"published_data,omitempty"`
	Status           ContentStatus  `json:"status" bson:"status"`
	DraftVersion     int            `json:"draft_version" bson:"draft_version"`
	PublishedVersion int            `json:"published_version" bson:"published_version"`
	SingletonKey     *string        `json:"-" bson:"singleton_key,omitempty"` // For single types: set to ContentTypeID
	CreatedBy        string         `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy        string         `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	PublishedAt      *time.Time     `json:"published_at,omitempty" bson:"published_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at" bson:"updated_at"`
	DeletedAt        *time.Time     `json:"deleted_at,omitempty" bson:"deleted_at,omitempty"`
}

// ContentVersion represents an immutable historical snapshot of an entry's data.
type ContentVersion struct {
	ID            string         `json:"id" bson:"_id"`
	EntryID       string         `json:"entry_id" bson:"entry_id"`
	VersionNumber int            `json:"version_number" bson:"version_number"`
	Snapshot      map[string]any `json:"snapshot" bson:"snapshot"`
	Action        string         `json:"action" bson:"action"` // "create_draft", "update_draft", "publish", "unpublish", "restore"
	ChangedBy     string         `json:"changed_by,omitempty" bson:"changed_by,omitempty"`
	CreatedAt     time.Time      `json:"created_at" bson:"created_at"`
}

// ContentRelation tracks links between content entries.
type ContentRelation struct {
	ID            string `json:"id" bson:"_id"`
	SourceEntryID string `json:"source_entry_id" bson:"source_entry_id"`
	TargetEntryID string `json:"target_entry_id" bson:"target_entry_id"`
	RelationKey   string `json:"relation_key" bson:"relation_key"`
}

// EntryFilter defines options for querying content collections.
type EntryFilter struct {
	Status    string
	Search    string
	SortBy    string
	SortOrder string
	Limit     int
	Offset    int
}

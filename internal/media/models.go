package media

import (
	"encoding/json"
	"time"
)

// MediaMetadata holds descriptive and dimensional metadata about a media file.
type MediaMetadata struct {
	Width      int               `json:"width,omitempty"`
	Height     int               `json:"height,omitempty"`
	AltText    string            `json:"alt_text,omitempty"`
	Caption    string            `json:"caption,omitempty"`
	Format     string            `json:"format,omitempty"`
	Thumbnails map[string]string `json:"thumbnails,omitempty"` // format name -> storage_key
	Custom     map[string]any    `json:"custom,omitempty"`
}

// MediaFolder represents a folder for organizing media assets in CMS and storage.
type MediaFolder struct {
	ID          string    `json:"id" bson:"_id"`
	Name        string    `json:"name" bson:"name"`
	Slug        string    `json:"slug" bson:"slug"`
	ParentID    *string   `json:"parent_id,omitempty" bson:"parent_id,omitempty"`
	Path        string    `json:"path" bson:"path"`                 // e.g. "blog/articles"
	StoragePath string    `json:"storage_path" bson:"storage_path"` // e.g. "blog/articles/"
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" bson:"updated_at"`
	CreatedBy   *string   `json:"created_by,omitempty" bson:"created_by,omitempty"`
}

// MediaFolderTreeNode represents a folder node with nested subfolders and statistics.
type MediaFolderTreeNode struct {
	MediaFolder
	Children   []*MediaFolderTreeNode `json:"children"`
	AssetCount int                    `json:"asset_count"`
}

// MediaAsset represents a file managed by the CMS media library.
type MediaAsset struct {
	ID              string        `json:"id" bson:"_id"`
	FolderID        *string       `json:"folder_id,omitempty" bson:"folder_id,omitempty"`
	Filename        string        `json:"filename" bson:"filename"`
	OriginalName    string        `json:"original_name" bson:"original_name"`
	MIMEType        string        `json:"mime_type" bson:"mime_type"`
	FileSize        int64         `json:"file_size" bson:"file_size"`
	StorageProvider string        `json:"storage_provider" bson:"storage_provider"`
	StorageKey      string        `json:"storage_key" bson:"storage_key"`
	URL             string        `json:"url,omitempty" bson:"-"`
	Metadata        MediaMetadata `json:"metadata" bson:"metadata"`
	CreatedAt       time.Time     `json:"created_at" bson:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at" bson:"updated_at"`
}

// MediaFilter specifies criteria for querying media assets.
type MediaFilter struct {
	FolderID *string `json:"folder_id,omitempty"` // Filter by folder (nil = all or unscoped, empty string = root only)
	Search   string  `json:"search"`
	MIMEType string  `json:"mime_type"` // Filters by prefix (e.g. "image/") or exact match
	Limit    int     `json:"limit"`
	Offset   int     `json:"offset"`
}

// MetadataJSON returns the JSON string representation of metadata for database storage.
func (m MediaMetadata) MetadataJSON() string {
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

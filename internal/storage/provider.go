package storage

import (
	"context"
	"errors"
	"io"
)

var (
	// ErrFileNotFound indicates the requested asset does not exist in storage.
	ErrFileNotFound = errors.New("file not found in storage")

	// ErrInvalidPath indicates the given storage key is malformed.
	ErrInvalidPath = errors.New("invalid storage path")

	// ErrPathTraversal indicates an attempt to access paths outside the storage root.
	ErrPathTraversal = errors.New("path traversal detected: access denied")

	// ErrStorageOperation indicates a generic storage provider error.
	ErrStorageOperation = errors.New("storage operation failed")
)

// StorageProvider defines the contract for persisting and retrieving media files.
type StorageProvider interface {
	// Name returns the identifier of this storage provider (e.g. "local", "s3", "minio").
	Name() string

	// Save persists data from reader r into the storage location identified by key.
	Save(ctx context.Context, key string, r io.Reader, size int64, mimeType string) error

	// Open retrieves a stream for reading the object identified by key.
	Open(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object identified by key from storage.
	Delete(ctx context.Context, key string) error

	// Exists checks whether the object identified by key exists in storage.
	Exists(ctx context.Context, key string) (bool, error)

	// URL returns the public or relative access URL for the given key.
	URL(ctx context.Context, key string) (string, error)

	// Move renames or moves an object or directory from srcKey to dstKey.
	Move(ctx context.Context, srcKey, dstKey string) error

	// DeletePrefix deletes all objects matching the given key prefix or folder path.
	DeletePrefix(ctx context.Context, prefix string) error
}

// HealthChecker is an optional interface implemented by storage providers supporting active connection checks.
type HealthChecker interface {
	CheckHealth(ctx context.Context) error
}

// ProviderCapabilities describes optional storage features supported by a provider.
type ProviderCapabilities struct {
	Provider              string `json:"provider"`
	SupportsHLS           bool   `json:"supports_hls"`
	SupportsSignedURLs    bool   `json:"supports_signed_urls"`
	SupportsTransforms    bool   `json:"supports_transforms"`
	SupportsFolderMapping bool   `json:"supports_folder_mapping"`
}

// CapabilityReporter is an optional interface providers can implement to advertise feature support.
type CapabilityReporter interface {
	Capabilities() ProviderCapabilities
}

// ProviderResolver resolves named storage providers so existing assets remain accessible after switching active drivers.
type ProviderResolver interface {
	ResolveProvider(name string) (StorageProvider, bool)
	RegisterProvider(p StorageProvider)
}

// UploadOptions carries optional provider-specific upload hints such as folder and resource type.
type UploadOptions struct {
	Filename     string
	Folder       string
	ResourceType string
	DeliveryType string
	UploadPreset string
	Tags         []string
	Context      map[string]string
	Overwrite    bool
}

// UploadResult returns normalized remote metadata produced during an upload.
type UploadResult struct {
	StorageKey   string            `json:"storage_key,omitempty"`
	PublicID     string            `json:"public_id,omitempty"`
	AssetID      string            `json:"asset_id,omitempty"`
	ResourceType string            `json:"resource_type,omitempty"`
	DeliveryType string            `json:"delivery_type,omitempty"`
	Version      int64             `json:"version,omitempty"`
	Format       string            `json:"format,omitempty"`
	Width        int               `json:"width,omitempty"`
	Height       int               `json:"height,omitempty"`
	Bytes        int64             `json:"bytes,omitempty"`
	Duration     float64           `json:"duration,omitempty"`
	SecureURL    string            `json:"secure_url,omitempty"`
	URL          string            `json:"url,omitempty"`
	Folder       string            `json:"folder,omitempty"`
	Variants     map[string]string `json:"variants,omitempty"`
}

// AssetRef identifies an existing stored object along with optional provider metadata.
type AssetRef struct {
	Key          string
	MIMEType     string
	PublicID     string
	ResourceType string
	DeliveryType string
	Version      int64
	Format       string
	SecureURL    string
}

// ImageTransformOptions describes safe, validated delivery transformation parameters.
type ImageTransformOptions struct {
	Width   int
	Height  int
	Crop    string
	Format  string
	Quality string
	Gravity string
	DPR     string
}

// RichUploader is an optional interface for providers returning rich remote asset metadata.
type RichUploader interface {
	SaveWithOptions(ctx context.Context, key string, r io.Reader, size int64, mimeType string, opts UploadOptions) (*UploadResult, error)
}

// AssetRefOperator is an optional interface for providers that benefit from asset-level metadata during URL, open, delete, or move operations.
type AssetRefOperator interface {
	OpenAsset(ctx context.Context, ref AssetRef) (io.ReadCloser, error)
	DeleteAsset(ctx context.Context, ref AssetRef) error
	MoveAsset(ctx context.Context, src AssetRef, dstKey string) (*UploadResult, error)
	AssetURL(ctx context.Context, ref AssetRef) (string, error)
}

// TransformURLGenerator is an optional interface for providers supporting safe URL transformations.
type TransformURLGenerator interface {
	TransformURL(ctx context.Context, ref AssetRef, opts ImageTransformOptions) (string, error)
}

// SignedAssetURLGenerator is an optional interface for providers that natively sign delivery URLs.
type SignedAssetURLGenerator interface {
	SignedURL(ctx context.Context, ref AssetRef, expiresIn int64) (string, error)
}

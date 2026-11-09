package media

import (
	"context"
	"errors"
)

var (
	// ErrAssetNotFound indicates the media asset does not exist in the database.
	ErrAssetNotFound = errors.New("media asset not found")
)

// Repository defines database operations for media asset records.
type Repository interface {
	// Create persists a new media record.
	Create(ctx context.Context, asset *MediaAsset) error

	// GetByID retrieves a media record by its unique ID.
	GetByID(ctx context.Context, id string) (*MediaAsset, error)

	// GetByKey retrieves a media record by its storage key.
	GetByKey(ctx context.Context, storageKey string) (*MediaAsset, error)

	// List queries media assets matching the provided filter, returning records and total count.
	List(ctx context.Context, filter MediaFilter) ([]*MediaAsset, int, error)

	// Update modifies an existing media asset's metadata.
	Update(ctx context.Context, asset *MediaAsset) error

	// Delete removes a media record from the database.
	Delete(ctx context.Context, id string) error
}

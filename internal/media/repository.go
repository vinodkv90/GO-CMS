package media

import (
	"context"
	"errors"
)

var (
	// ErrPathTraversal indicates an attempted path traversal.
	ErrPathTraversal = errors.New("path traversal detected: access denied")

	// ErrAssetNotFound indicates the media asset does not exist in the database.
	ErrAssetNotFound = errors.New("media asset not found")

	// ErrFolderNotFound indicates the media folder does not exist.
	ErrFolderNotFound = errors.New("media folder not found")

	// ErrDuplicateFolder indicates a folder with the same name or slug exists under the parent.
	ErrDuplicateFolder = errors.New("a folder with this name or slug already exists in this directory")

	// ErrCircularFolder indicates an attempt to move a folder into itself or a descendant.
	ErrCircularFolder = errors.New("cannot move a folder into itself or one of its descendants")

	// ErrFolderNotEmpty indicates an attempt to delete a folder containing items without recursive authorization.
	ErrFolderNotEmpty = errors.New("folder contains items; recursive deletion must be explicitly requested")
)

// Repository defines database operations for media asset records and folder hierarchies.
type Repository interface {
	// Asset operations
	Create(ctx context.Context, asset *MediaAsset) error
	GetByID(ctx context.Context, id string) (*MediaAsset, error)
	GetByKey(ctx context.Context, storageKey string) (*MediaAsset, error)
	List(ctx context.Context, filter MediaFilter) ([]*MediaAsset, int, error)
	Update(ctx context.Context, asset *MediaAsset) error
	Delete(ctx context.Context, id string) error
	UpdateAssetFolder(ctx context.Context, assetID string, targetFolderID *string, newStorageKey string) error

	// Folder operations
	CreateFolder(ctx context.Context, folder *MediaFolder) error
	GetFolderByID(ctx context.Context, id string) (*MediaFolder, error)
	GetFolderByPath(ctx context.Context, path string) (*MediaFolder, error)
	ListFolders(ctx context.Context, parentID *string) ([]*MediaFolder, error)
	GetAllFolders(ctx context.Context) ([]*MediaFolder, error)
	UpdateFolder(ctx context.Context, folder *MediaFolder) error
	DeleteFolder(ctx context.Context, id string) error
	GetAssetsByFolderID(ctx context.Context, folderID string) ([]*MediaAsset, error)
	GetSubfolders(ctx context.Context, parentID string) ([]*MediaFolder, error)
}

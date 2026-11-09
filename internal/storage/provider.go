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

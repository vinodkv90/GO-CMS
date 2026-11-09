package local_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-cms/cms/internal/storage"
	"github.com/go-cms/cms/internal/storage/local"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalProvider_CRUD(t *testing.T) {
	tempDir := t.TempDir()
	p, err := local.New(tempDir, "/uploads")
	require.NoError(t, err)
	assert.Equal(t, "local", p.Name())

	ctx := context.Background()
	key := "2026/11/test-image.png"
	content := []byte("fake png binary data")

	// 1. File does not exist initially
	exists, err := p.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)

	// 2. Save file
	err = p.Save(ctx, key, bytes.NewReader(content), int64(len(content)), "image/png")
	require.NoError(t, err)

	// 3. File exists now
	exists, err = p.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	// 4. Open and read file
	rc, err := p.Open(ctx, key)
	require.NoError(t, err)

	readData, err := io.ReadAll(rc)
	require.NoError(t, err)
	_ = rc.Close()
	assert.Equal(t, content, readData)

	// 5. URL generation
	url, err := p.URL(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, "/uploads/2026/11/test-image.png", url)

	// 6. Delete file
	err = p.Delete(ctx, key)
	require.NoError(t, err)

	exists, err = p.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)

	// 7. Deleting non-existent file returns ErrFileNotFound
	err = p.Delete(ctx, key)
	assert.ErrorIs(t, err, storage.ErrFileNotFound)
}

func TestLocalProvider_PathTraversalDefenses(t *testing.T) {
	tempDir := t.TempDir()
	p, err := local.New(tempDir, "/uploads")
	require.NoError(t, err)

	ctx := context.Background()
	secretFile := filepath.Join(tempDir, "secret.txt")
	require.NoError(t, os.WriteFile(secretFile, []byte("super secret"), 0644))

	attackKeys := []string{
		"../secret.txt",
		"../../secret.txt",
		"sub/../../secret.txt",
		"sub/../../../etc/passwd",
		"/etc/passwd",
		"\\windows\\system32",
		"test\x00file.png",
	}

	for _, key := range attackKeys {
		t.Run("traversal_"+key, func(t *testing.T) {
			// Save
			err := p.Save(ctx, key, strings.NewReader("bad"), 3, "text/plain")
			assert.ErrorIs(t, err, storage.ErrPathTraversal)

			// Open
			_, err = p.Open(ctx, key)
			assert.ErrorIs(t, err, storage.ErrPathTraversal)

			// Delete
			err = p.Delete(ctx, key)
			assert.ErrorIs(t, err, storage.ErrPathTraversal)

			// Exists
			_, err = p.Exists(ctx, key)
			assert.ErrorIs(t, err, storage.ErrPathTraversal)
		})
	}
}

func TestLocalProvider_Move_And_DeletePrefix(t *testing.T) {
	tempDir := t.TempDir()
	p, err := local.New(tempDir, "/uploads")
	require.NoError(t, err)

	ctx := context.Background()

	// 1. Move file
	srcKey := "blog/articles/old-photo.jpg"
	dstKey := "blog/archive/new-photo.jpg"
	content := []byte("image binary payload")

	require.NoError(t, p.Save(ctx, srcKey, bytes.NewReader(content), int64(len(content)), "image/jpeg"))

	// Verify src exists before move
	exists, err := p.Exists(ctx, srcKey)
	require.NoError(t, err)
	assert.True(t, exists)

	// Move
	require.NoError(t, p.Move(ctx, srcKey, dstKey))

	// Verify src no longer exists, dst exists with content
	exists, err = p.Exists(ctx, srcKey)
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = p.Exists(ctx, dstKey)
	require.NoError(t, err)
	assert.True(t, exists)

	rc, err := p.Open(ctx, dstKey)
	require.NoError(t, err)
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	_ = rc.Close()
	assert.Equal(t, content, data)

	// 2. Move directory
	dirSrcKey := "blog/archive"
	dirDstKey := "blog/historical"
	require.NoError(t, p.Move(ctx, dirSrcKey, dirDstKey))

	// Old file under moved dir doesn't exist, new file does
	exists, err = p.Exists(ctx, "blog/archive/new-photo.jpg")
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = p.Exists(ctx, "blog/historical/new-photo.jpg")
	require.NoError(t, err)
	assert.True(t, exists)

	// 3. DeletePrefix
	require.NoError(t, p.Save(ctx, "blog/historical/extra.jpg", bytes.NewReader(content), int64(len(content)), "image/jpeg"))
	require.NoError(t, p.DeletePrefix(ctx, "blog/historical"))

	exists, err = p.Exists(ctx, "blog/historical/new-photo.jpg")
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = p.Exists(ctx, "blog/historical/extra.jpg")
	require.NoError(t, err)
	assert.False(t, exists)

	// 4. Moving non-existent source returns ErrFileNotFound
	err = p.Move(ctx, "nonexistent/file.png", "destination/file.png")
	assert.ErrorIs(t, err, storage.ErrFileNotFound)
}

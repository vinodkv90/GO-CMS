package media_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/media"
	"github.com/go-cms/cms/internal/storage/local"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestMediaService(t *testing.T, enableSignedURLs bool) (*media.Service, *media.MemoryRepository, *local.Provider, *config.Config) {
	tempDir := t.TempDir()
	storageProvider, err := local.New(tempDir, "/uploads")
	require.NoError(t, err)

	repo := media.NewMemoryRepository()
	cfg := config.DefaultConfig()
	cfg.Media.SignedURLs.Enabled = enableSignedURLs
	cfg.Media.SignedURLs.SecretKey = "media-test-hmac-secret-key"
	cfg.Media.SignedURLs.Expiration = 1 * time.Hour

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := media.NewService(repo, storageProvider, cfg, logger)

	return svc, repo, storageProvider, cfg
}

func TestMediaService_UploadAndRetrieve(t *testing.T) {
	svc, _, storageProvider, _ := setupTestMediaService(t, false)
	ctx := context.Background()

	pngData := createTestPNG(600, 400)

	// 1. Upload Image
	asset, err := svc.Upload(
		ctx,
		bytes.NewReader(pngData),
		"Hero Banner.png",
		int64(len(pngData)),
		"A vibrant hero banner",
		"Homepage hero section",
	)
	require.NoError(t, err)
	assert.NotEmpty(t, asset.ID)
	assert.Equal(t, "hero-banner.png", asset.Filename[stringsLastIndex(asset.Filename, "_")+1:])
	assert.Equal(t, "image/png", asset.MIMEType)
	assert.Equal(t, int64(len(pngData)), asset.FileSize)
	assert.Equal(t, 600, asset.Metadata.Width)
	assert.Equal(t, 400, asset.Metadata.Height)
	assert.Equal(t, "A vibrant hero banner", asset.Metadata.AltText)
	assert.NotEmpty(t, asset.Metadata.Thumbnails["small"])
	assert.Contains(t, asset.URL, "/uploads/")

	// Verify main file exists in storage
	mainExists, err := storageProvider.Exists(ctx, asset.StorageKey)
	require.NoError(t, err)
	assert.True(t, mainExists)

	// Verify thumbnail exists in storage
	thumbExists, err := storageProvider.Exists(ctx, asset.Metadata.Thumbnails["small"])
	require.NoError(t, err)
	assert.True(t, thumbExists)

	// 2. Get Asset
	fetched, err := svc.GetAsset(ctx, asset.ID)
	require.NoError(t, err)
	assert.Equal(t, asset.ID, fetched.ID)
	assert.Equal(t, asset.StorageKey, fetched.StorageKey)

	// 3. Update Asset
	updated, err := svc.UpdateAsset(ctx, asset.ID, "Updated Alt", "Updated Caption")
	require.NoError(t, err)
	assert.Equal(t, "Updated Alt", updated.Metadata.AltText)
	assert.Equal(t, "Updated Caption", updated.Metadata.Caption)

	// 4. Delete Asset (should delete file + thumbnail + db)
	err = svc.DeleteAsset(ctx, asset.ID)
	require.NoError(t, err)

	_, err = svc.GetAsset(ctx, asset.ID)
	assert.ErrorIs(t, err, media.ErrAssetNotFound)

	mainExists, _ = storageProvider.Exists(ctx, asset.StorageKey)
	assert.False(t, mainExists)

	thumbExists, _ = storageProvider.Exists(ctx, asset.Metadata.Thumbnails["small"])
	assert.False(t, thumbExists)
}

func TestMediaService_ListAndFilter(t *testing.T) {
	svc, _, _, _ := setupTestMediaService(t, false)
	ctx := context.Background()

	// Upload 3 items
	img1 := createTestPNG(100, 100)
	img2 := createTestJPEG(200, 200)
	pdfData := []byte("%PDF-1.4 test document content")

	_, err := svc.Upload(ctx, bytes.NewReader(img1), "Nature landscape.png", int64(len(img1)), "", "")
	require.NoError(t, err)

	_, err = svc.Upload(ctx, bytes.NewReader(img2), "City skyline.jpg", int64(len(img2)), "", "")
	require.NoError(t, err)

	_, err = svc.Upload(ctx, bytes.NewReader(pdfData), "Annual Report.pdf", int64(len(pdfData)), "", "")
	require.NoError(t, err)

	// 1. List all
	all, total, err := svc.ListAssets(ctx, media.MediaFilter{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, all, 3)

	// 2. Filter by search "city"
	results, count, err := svc.ListAssets(ctx, media.MediaFilter{Search: "city"})
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Contains(t, results[0].OriginalName, "City")

	// 3. Filter by MIME type "image/"
	imgResults, imgCount, err := svc.ListAssets(ctx, media.MediaFilter{MIMEType: "image/"})
	require.NoError(t, err)
	assert.Equal(t, 2, imgCount)
	assert.Len(t, imgResults, 2)
}

func TestMediaService_SignedURLs(t *testing.T) {
	svc, _, _, _ := setupTestMediaService(t, true)
	ctx := context.Background()

	pngData := createTestPNG(50, 50)
	asset, err := svc.Upload(ctx, bytes.NewReader(pngData), "private.png", int64(len(pngData)), "", "")
	require.NoError(t, err)

	// URL automatically contains signature when signed URLs are enabled
	assert.Contains(t, asset.URL, "signature=")
	assert.Contains(t, asset.URL, "expires=")

	// Explicit GenerateSignedURL with custom TTL
	customSignedURL, err := svc.GenerateSignedURL(ctx, asset.ID, 15*time.Minute)
	require.NoError(t, err)
	assert.Contains(t, customSignedURL, "signature=")
	assert.Contains(t, customSignedURL, "expires=")
}

func stringsLastIndex(s, sep string) int {
	for i := len(s) - len(sep); i >= 0; i-- {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}


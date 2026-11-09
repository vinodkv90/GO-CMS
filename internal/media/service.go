package media

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/storage"
	"github.com/go-cms/cms/internal/tenancy"
)

const maxSignedURLTTL = 24 * time.Hour

// Service orchestrates media upload processing, metadata inspection, storage, and retrieval.
type Service struct {
	repo    Repository
	storage storage.StorageProvider
	signer  *storage.URLSigner
	cfg     *config.Config
	logger  *slog.Logger
}

// NewService creates a new media service instance.
func NewService(
	repo Repository,
	storageProvider storage.StorageProvider,
	cfg *config.Config,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}

	var signer *storage.URLSigner
	if cfg != nil && cfg.Media.SignedURLs.Enabled {
		signer = storage.NewURLSigner(cfg.Media.SignedURLs.SecretKey, cfg.Media.SignedURLs.Expiration)
	}

	return &Service{
		repo:    repo,
		storage: storageProvider,
		signer:  signer,
		cfg:     cfg,
		logger:  logger,
	}
}

// Storage returns the underlying storage provider.
func (s *Service) Storage() storage.StorageProvider {
	return s.storage
}

// Signer returns the URLSigner if configured.
func (s *Service) Signer() *storage.URLSigner {
	return s.signer
}

// Upload handles receiving a file, sniffing MIME type, extracting image metadata,
// generating thumbnails, persisting to storage, and storing the media record in database.
func (s *Service) Upload(
	ctx context.Context,
	r io.Reader,
	originalName string,
	size int64,
	altText string,
	caption string,
) (*MediaAsset, error) {
	// Read file into memory (or buffer) to allow MIME sniffing and multiple passes
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload payload: %w", err)
	}

	actualSize := int64(len(data))
	if actualSize == 0 {
		return nil, fmt.Errorf("uploaded file is empty")
	}

	// 1. Detect MIME type
	mimeType, _, err := SniffMIME(bytes.NewReader(data))
	if err != nil || mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// 2. Sanitize filename and generate storage key
	sanitizedName := SanitizeFilename(originalName)
	assetID, storageKey := GenerateStorageKey(sanitizedName)

	metadata := MediaMetadata{
		AltText:    altText,
		Caption:    caption,
		Thumbnails: make(map[string]string),
	}

	// 3. Image processing (if media is an image)
	if strings.HasPrefix(mimeType, "image/") && mimeType != "image/svg+xml" {
		w, h, format, inspectErr := InspectImage(data)
		if inspectErr == nil {
			metadata.Width = w
			metadata.Height = h
			metadata.Format = format
		}

		// Generate thumbnail for raster images
		thumbBytes, tw, th, thumbErr := GenerateThumbnail(bytes.NewReader(data), 250)
		if thumbErr == nil && len(thumbBytes) > 0 {
			ext := filepath.Ext(storageKey)
			thumbKey := strings.TrimSuffix(storageKey, ext) + "_thumb" + ext
			if saveThumbErr := s.storage.Save(ctx, thumbKey, bytes.NewReader(thumbBytes), int64(len(thumbBytes)), mimeType); saveThumbErr == nil {
				metadata.Thumbnails["small"] = thumbKey
				s.logger.Debug("Generated thumbnail for media asset", "asset_id", assetID, "width", tw, "height", th)
			}
		}
	}

	// 4. Save primary file to storage provider
	if err := s.storage.Save(ctx, storageKey, bytes.NewReader(data), actualSize, mimeType); err != nil {
		return nil, fmt.Errorf("failed to save file to storage: %w", err)
	}

	// 5. Persist media record to database
	now := time.Now().UTC()
	asset := &MediaAsset{
		ID:              assetID,
		Filename:        filepath.Base(storageKey),
		OriginalName:    originalName,
		MIMEType:        mimeType,
		FileSize:        actualSize,
		StorageProvider: s.storage.Name(),
		StorageKey:      storageKey,
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.repo.Create(ctx, asset); err != nil {
		// Cleanup saved file on database failure
		_ = s.storage.Delete(ctx, storageKey)
		return nil, fmt.Errorf("failed to save media record in database: %w", err)
	}

	// Resolve asset URL
	asset.URL, _ = s.ResolveURL(ctx, asset.StorageKey)
	return asset, nil
}

// GetAsset retrieves a media asset by ID with its resolved access URL.
func (s *Service) GetAsset(ctx context.Context, id string) (*MediaAsset, error) {
	asset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	asset.URL, _ = s.ResolveURL(ctx, asset.StorageKey)
	return asset, nil
}

// ListAssets returns paginated media assets.
func (s *Service) ListAssets(ctx context.Context, filter MediaFilter) ([]*MediaAsset, int, error) {
	assets, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	for _, a := range assets {
		a.URL, _ = s.ResolveURL(ctx, a.StorageKey)
	}

	return assets, total, nil
}

// UpdateAsset modifies user-editable metadata (alt text, caption).
func (s *Service) UpdateAsset(ctx context.Context, id string, altText, caption string) (*MediaAsset, error) {
	asset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	asset.Metadata.AltText = altText
	asset.Metadata.Caption = caption

	if err := s.repo.Update(ctx, asset); err != nil {
		return nil, err
	}

	asset.URL, _ = s.ResolveURL(ctx, asset.StorageKey)
	return asset, nil
}

// DeleteAsset removes the file and any thumbnails from storage and deletes the database record.
func (s *Service) DeleteAsset(ctx context.Context, id string) error {
	asset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Delete primary file
	_ = s.storage.Delete(ctx, asset.StorageKey)

	// Delete thumbnails
	for _, thumbKey := range asset.Metadata.Thumbnails {
		_ = s.storage.Delete(ctx, thumbKey)
	}

	return s.repo.Delete(ctx, id)
}

// GenerateSignedURL generates a temporary HMAC-SHA256 signed access URL for the asset.
func (s *Service) GenerateSignedURL(ctx context.Context, id string, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > maxSignedURLTTL {
		return "", fmt.Errorf("signed URL TTL must be between 1 second and 24 hours")
	}
	asset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}

	rawURL, err := s.storage.URL(ctx, asset.StorageKey)
	if err != nil {
		return "", err
	}

	if s.signer == nil {
		return rawURL, nil
	}

	tenantID := ""
	if tenant := tenancy.GetTenant(ctx); tenant != nil {
		tenantID = tenant.ID
	}
	return s.signer.SignForTenant(rawURL, ttl, tenantID)
}

// ResolveURL generates the standard access URL for an asset key, applying signatures if required.
func (s *Service) ResolveURL(ctx context.Context, storageKey string) (string, error) {
	rawURL, err := s.storage.URL(ctx, storageKey)
	if err != nil {
		return "", err
	}

	if s.cfg != nil && s.cfg.Media.SignedURLs.Enabled && s.signer != nil {
		tenantID := ""
		if tenant := tenancy.GetTenant(ctx); tenant != nil {
			tenantID = tenant.ID
		}
		return s.signer.SignForTenant(rawURL, s.cfg.Media.SignedURLs.Expiration, tenantID)
	}

	return rawURL, nil
}

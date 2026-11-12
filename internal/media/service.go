package media

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/storage"
	"github.com/go-cms/cms/internal/tenancy"
	"github.com/google/uuid"
)

const maxSignedURLTTL = 7 * 24 * time.Hour

// RuntimeConfig holds dynamically enforced media upload, thumbnail, and URL settings.
type RuntimeConfig struct {
	MaxFileSizeBytes       int64
	AllowedMIMETypes       []string
	ResponsiveSizesEnabled bool
	GenerateSmall          bool
	GenerateMedium         bool
	GenerateLarge          bool
	RequireAltText         bool
	DefaultFolderID        *string
	DefaultAccess          string // "public" or "signed"
	SignedURLTTLSeconds    int
}

// Service orchestrates media upload processing, metadata inspection, storage, and retrieval.
type Service struct {
	repo             Repository
	storage          storage.StorageProvider
	signer           *storage.URLSigner
	cfg              *config.Config
	logger           *slog.Logger
	mu               sync.RWMutex
	settingsProvider func(context.Context) (RuntimeConfig, error)
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

	var signingSecret string
	var defaultTTL time.Duration
	if cfg != nil {
		signingSecret = cfg.Media.SignedURLs.SecretKey
		if signingSecret == "" {
			signingSecret = cfg.Auth.SessionSecret
		}
		if signingSecret == "" {
			signingSecret = cfg.Auth.JWTSecret
		}
		defaultTTL = cfg.Media.SignedURLs.Expiration
	}
	if signingSecret == "" {
		signingSecret = "cms-media-signing-fallback"
	}
	if defaultTTL <= 0 {
		defaultTTL = time.Hour
	}
	signer := storage.NewURLSigner(signingSecret, defaultTTL)

	return &Service{
		repo:    repo,
		storage: storageProvider,
		signer:  signer,
		cfg:     cfg,
		logger:  logger,
	}
}

// SetMediaSettingsProvider registers a callback returning live media policies from Settings.
func (s *Service) SetMediaSettingsProvider(fn func(context.Context) (RuntimeConfig, error)) {
	s.mu.Lock()
	s.settingsProvider = fn
	s.mu.Unlock()
}

func (s *Service) currentRuntimeConfig(ctx context.Context) RuntimeConfig {
	defaultAccess := "public"
	ttlSec := 3600
	if s.cfg != nil {
		if s.cfg.Media.SignedURLs.Enabled {
			defaultAccess = "signed"
		}
		if s.cfg.Media.SignedURLs.Expiration > 0 {
			ttlSec = int(s.cfg.Media.SignedURLs.Expiration.Seconds())
		}
	}
	def := RuntimeConfig{
		MaxFileSizeBytes:       50 << 20,
		ResponsiveSizesEnabled: true,
		GenerateSmall:          true,
		DefaultAccess:          defaultAccess,
		SignedURLTTLSeconds:    ttlSec,
	}
	s.mu.RLock()
	fn := s.settingsProvider
	s.mu.RUnlock()
	if fn == nil {
		return def
	}
	rt, err := fn(ctx)
	if err != nil {
		return def
	}
	if rt.MaxFileSizeBytes <= 0 {
		rt.MaxFileSizeBytes = def.MaxFileSizeBytes
	}
	if rt.DefaultAccess == "" {
		rt.DefaultAccess = def.DefaultAccess
	}
	if rt.SignedURLTTLSeconds <= 0 {
		rt.SignedURLTTLSeconds = def.SignedURLTTLSeconds
	}
	return rt
}

// MaxUploadRequestBodyBytes returns the configured max upload byte size plus 2MB multipart overhead.
func (s *Service) MaxUploadRequestBodyBytes(ctx context.Context) int64 {
	rt := s.currentRuntimeConfig(ctx)
	limit := rt.MaxFileSizeBytes + (2 << 20)
	if limit < (32 << 20) {
		return 32 << 20
	}
	return limit
}

// ShouldRequireSignedURLs reports whether signed URLs are enforced for asset URLs and file serving.
func (s *Service) ShouldRequireSignedURLs(ctx context.Context) bool {
	if s.signer == nil {
		return false
	}
	return strings.EqualFold(s.currentRuntimeConfig(ctx).DefaultAccess, "signed")
}

// DefaultSignedURLTTL returns the configured default signed URL lifetime bounded by maxSignedURLTTL.
func (s *Service) DefaultSignedURLTTL(ctx context.Context) time.Duration {
	sec := s.currentRuntimeConfig(ctx).SignedURLTTLSeconds
	if sec <= 0 {
		return time.Hour
	}
	ttl := time.Duration(sec) * time.Second
	if ttl > maxSignedURLTTL {
		return maxSignedURLTTL
	}
	return ttl
}

func matchesAllowedMIME(actual string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	base := strings.ToLower(strings.TrimSpace(strings.Split(actual, ";")[0]))
	if base == "" {
		return false
	}
	actualParts := strings.SplitN(base, "/", 2)
	for _, pat := range allowed {
		p := strings.ToLower(strings.TrimSpace(pat))
		if p == "" {
			continue
		}
		if p == "*/*" || p == base {
			return true
		}
		if strings.HasSuffix(p, "/*") && len(actualParts) == 2 {
			if strings.TrimSuffix(p, "/*") == actualParts[0] {
				return true
			}
		}
	}
	return false
}

// Storage returns the underlying storage provider.
func (s *Service) Storage() storage.StorageProvider {
	return s.storage
}

// Signer returns the URLSigner if configured.
func (s *Service) Signer() *storage.URLSigner {
	return s.signer
}

// Capabilities reports the active storage provider's feature capabilities.
func (s *Service) Capabilities() storage.ProviderCapabilities {
	if s.storage == nil {
		return storage.ProviderCapabilities{}
	}
	if reporter, ok := s.storage.(storage.CapabilityReporter); ok {
		return reporter.Capabilities()
	}
	return storage.ProviderCapabilities{
		Provider:              s.storage.Name(),
		SupportsHLS:           true,
		SupportsSignedURLs:    true,
		SupportsTransforms:    false,
		SupportsFolderMapping: true,
	}
}

func (s *Service) activeStorage() storage.StorageProvider {
	if s.storage == nil {
		return nil
	}
	if resolver, ok := s.storage.(storage.ProviderResolver); ok {
		if name := s.storage.Name(); name != "" {
			if active, found := resolver.ResolveProvider(name); found && active != nil {
				return active
			}
		}
	}
	return s.storage
}

func (s *Service) storageFor(providerName string) storage.StorageProvider {
	if providerName != "" && s.storage != nil {
		if resolver, ok := s.storage.(storage.ProviderResolver); ok {
			if p, found := resolver.ResolveProvider(providerName); found {
				return p
			}
		}
	}
	return s.activeStorage()
}

func assetRefFromAsset(asset *MediaAsset) storage.AssetRef {
	ref := storage.AssetRef{
		Key:      asset.StorageKey,
		MIMEType: asset.MIMEType,
		Format:   asset.Metadata.Format,
	}
	if asset.Metadata.Custom != nil {
		if rawCloud, ok := asset.Metadata.Custom["cloudinary"].(map[string]any); ok {
			if v, ok := rawCloud["public_id"].(string); ok {
				ref.PublicID = v
			}
			if v, ok := rawCloud["resource_type"].(string); ok {
				ref.ResourceType = v
			}
			if v, ok := rawCloud["delivery_type"].(string); ok {
				ref.DeliveryType = v
			}
			if v, ok := rawCloud["format"].(string); ok && v != "" {
				ref.Format = v
			}
			if v, ok := rawCloud["secure_url"].(string); ok {
				ref.SecureURL = v
			}
			switch v := rawCloud["version"].(type) {
			case int64:
				ref.Version = v
			case int:
				ref.Version = int64(v)
			case float64:
				ref.Version = int64(v)
			}
		}
	}
	return ref
}

func applyUploadResultToMetadata(metadata *MediaMetadata, res *storage.UploadResult) {
	if res == nil {
		return
	}
	if metadata.Width == 0 && res.Width > 0 {
		metadata.Width = res.Width
	}
	if metadata.Height == 0 && res.Height > 0 {
		metadata.Height = res.Height
	}
	if metadata.Format == "" && res.Format != "" {
		metadata.Format = res.Format
	}
	if len(res.Variants) > 0 {
		if metadata.Thumbnails == nil {
			metadata.Thumbnails = make(map[string]string, len(res.Variants))
		}
		for k, v := range res.Variants {
			metadata.Thumbnails[k] = v
		}
	}
	if res.PublicID != "" || res.AssetID != "" || res.SecureURL != "" {
		if metadata.Custom == nil {
			metadata.Custom = make(map[string]any)
		}
		metadata.Custom["cloudinary"] = map[string]any{
			"asset_id":      res.AssetID,
			"public_id":     res.PublicID,
			"resource_type": res.ResourceType,
			"delivery_type": res.DeliveryType,
			"version":       res.Version,
			"format":        res.Format,
			"secure_url":    res.SecureURL,
			"folder":        res.Folder,
		}
	}
}

type storedPayload struct {
	data         []byte
	actualSize   int64
	mimeType     string
	sanitized    string
	assetID      string
	storageKey   string
	metadata     MediaMetadata
}

func (s *Service) readAndValidatePayload(ctx context.Context, rt RuntimeConfig, r io.Reader, size int64, originalName string) ([]byte, int64, string, string, error) {
	maxSize := rt.MaxFileSizeBytes
	if maxSize <= 0 {
		maxSize = 50 << 20
	}
	if size > maxSize {
		return nil, 0, "", "", fmt.Errorf("%w: uploaded file (%d bytes) exceeds maximum allowed size (%d bytes)", ErrMediaValidation, size, maxSize)
	}

	data, err := io.ReadAll(io.LimitReader(r, maxSize+1))
	if err != nil {
		return nil, 0, "", "", fmt.Errorf("failed to read upload payload: %w", err)
	}

	actualSize := int64(len(data))
	if actualSize == 0 {
		return nil, 0, "", "", fmt.Errorf("%w: uploaded file is empty", ErrMediaValidation)
	}
	if actualSize > maxSize {
		return nil, 0, "", "", fmt.Errorf("%w: uploaded file exceeds maximum allowed size (%d bytes)", ErrMediaValidation, maxSize)
	}

	mimeType, _, err := SniffMIME(bytes.NewReader(data))
	if err != nil || mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if !matchesAllowedMIME(mimeType, rt.AllowedMIMETypes) {
		return nil, 0, "", "", fmt.Errorf("%w: file MIME type %q is not allowed by media settings", ErrMediaValidation, mimeType)
	}

	return data, actualSize, mimeType, SanitizeFilename(originalName), nil
}

func (s *Service) processAndStoreMedia(ctx context.Context, rt RuntimeConfig, storageKey, assetID, originalName, mimeType, altText, caption string, data []byte) (MediaMetadata, error) {
	metadata := MediaMetadata{
		AltText:    altText,
		Caption:    caption,
		Thumbnails: make(map[string]string),
	}

	supportsTransforms := s.Capabilities().SupportsTransforms
	if strings.HasPrefix(mimeType, "image/") && mimeType != "image/svg+xml" {
		if w, h, format, inspectErr := InspectImage(data); inspectErr == nil {
			metadata.Width = w
			metadata.Height = h
			metadata.Format = format
		}

		if rt.ResponsiveSizesEnabled && !supportsTransforms {
			baseExt := filepath.Ext(storageKey)
			baseStem := strings.TrimSuffix(storageKey, baseExt)
			for _, spec := range []struct {
				enabled bool
				name    string
				maxDim  int
			}{
				{rt.GenerateSmall, "small", 250},
				{rt.GenerateMedium, "medium", 500},
				{rt.GenerateLarge, "large", 1000},
			} {
				if !spec.enabled {
					continue
				}
				thumbBytes, tw, th, thumbErr := GenerateThumbnail(bytes.NewReader(data), spec.maxDim)
				if thumbErr != nil || len(thumbBytes) == 0 {
					continue
				}
				thumbMIME, _, _ := SniffMIME(bytes.NewReader(thumbBytes))
				thumbExt := ".png"
				switch thumbMIME {
				case "image/jpeg":
					thumbExt = ".jpg"
				case "image/gif":
					thumbExt = ".gif"
				default:
					if thumbMIME == "" || !strings.HasPrefix(thumbMIME, "image/") {
						thumbMIME = "image/png"
					}
					thumbExt = ".png"
				}
				thumbKey := baseStem + "_thumb_" + spec.name + thumbExt
				if saveThumbErr := s.storage.Save(ctx, thumbKey, bytes.NewReader(thumbBytes), int64(len(thumbBytes)), thumbMIME); saveThumbErr == nil {
					metadata.Thumbnails[spec.name] = thumbKey
					s.logger.Debug("Generated thumbnail for media asset", "asset_id", assetID, "size", spec.name, "width", tw, "height", th, "mime", thumbMIME)
				}
			}
		}
	}

	activeProv := s.activeStorage()
	actualSize := int64(len(data))
	if rich, ok := activeProv.(storage.RichUploader); ok {
		upRes, err := rich.SaveWithOptions(ctx, storageKey, bytes.NewReader(data), actualSize, mimeType, storage.UploadOptions{
			Filename:  originalName,
			Overwrite: true,
			Context: map[string]string{
				"alt":     altText,
				"caption": caption,
			},
		})
		if err != nil {
			s.cleanupLocalThumbs(ctx, activeProv, metadata.Thumbnails)
			return metadata, fmt.Errorf("failed to save file to storage: %w", err)
		}
		applyUploadResultToMetadata(&metadata, upRes)
		if !rt.ResponsiveSizesEnabled {
			metadata.Thumbnails = nil
		}
		return metadata, nil
	}

	if err := activeProv.Save(ctx, storageKey, bytes.NewReader(data), actualSize, mimeType); err != nil {
		s.cleanupLocalThumbs(ctx, activeProv, metadata.Thumbnails)
		return metadata, fmt.Errorf("failed to save file to storage: %w", err)
	}
	return metadata, nil
}

func (s *Service) cleanupLocalThumbs(ctx context.Context, prov storage.StorageProvider, thumbs map[string]string) {
	if prov == nil {
		return
	}
	for _, tk := range thumbs {
		if !strings.HasPrefix(tk, "http://") && !strings.HasPrefix(tk, "https://") {
			_ = prov.Delete(ctx, tk)
		}
	}
}

// Upload handles receiving a file, sniffing MIME type, extracting image metadata,
// generating thumbnails, persisting to storage, and storing the media record in database.
// Supports optional folderID (variadic for backwards compatibility).
func (s *Service) Upload(
	ctx context.Context,
	r io.Reader,
	originalName string,
	size int64,
	altText string,
	caption string,
	folderID ...*string,
) (*MediaAsset, error) {
	rt := s.currentRuntimeConfig(ctx)
	if rt.RequireAltText && strings.TrimSpace(altText) == "" {
		return nil, fmt.Errorf("%w: alt_text is required by media library settings", ErrMediaValidation)
	}

	data, actualSize, mimeType, sanitizedName, err := s.readAndValidatePayload(ctx, rt, r, size, originalName)
	if err != nil {
		return nil, err
	}

	var targetFolderID *string
	var assetID, storageKey string

	if len(folderID) > 0 && folderID[0] != nil && strings.TrimSpace(*folderID[0]) != "" {
		fid := strings.TrimSpace(*folderID[0])
		folder, err := s.repo.GetFolderByID(ctx, fid)
		if err != nil {
			return nil, fmt.Errorf("target folder not found: %w", err)
		}
		targetFolderID = &fid
		assetID, storageKey = GenerateFolderStorageKey(folder.StoragePath, sanitizedName)
	} else if rt.DefaultFolderID != nil && strings.TrimSpace(*rt.DefaultFolderID) != "" {
		defFID := strings.TrimSpace(*rt.DefaultFolderID)
		folder, err := s.repo.GetFolderByID(ctx, defFID)
		if err == nil && folder != nil {
			targetFolderID = &folder.ID
			assetID, storageKey = GenerateFolderStorageKey(folder.StoragePath, sanitizedName)
		} else {
			assetID, storageKey = GenerateStorageKey(sanitizedName)
		}
	} else {
		assetID, storageKey = GenerateStorageKey(sanitizedName)
	}

	metadata, err := s.processAndStoreMedia(ctx, rt, storageKey, assetID, originalName, mimeType, altText, caption, data)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	asset := &MediaAsset{
		ID:              assetID,
		FolderID:        targetFolderID,
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
		_ = s.storage.Delete(ctx, storageKey)
		s.cleanupLocalThumbs(ctx, s.storage, metadata.Thumbnails)
		return nil, fmt.Errorf("failed to save media record in database: %w", err)
	}

	asset.URL, _ = s.ResolveAssetURL(ctx, asset)
	return asset, nil
}

// ReplaceAsset replaces an existing media asset's underlying file while preserving asset ID and references.
func (s *Service) ReplaceAsset(
	ctx context.Context,
	id string,
	r io.Reader,
	originalName string,
	size int64,
	altText string,
	caption string,
) (*MediaAsset, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	rt := s.currentRuntimeConfig(ctx)
	data, actualSize, mimeType, sanitizedFilename, err := s.readAndValidatePayload(ctx, rt, r, size, originalName)
	if err != nil {
		return nil, err
	}

	var storageKey string
	if existing.FolderID != nil && *existing.FolderID != "" {
		if folder, ferr := s.repo.GetFolderByID(ctx, *existing.FolderID); ferr == nil {
			_, storageKey = GenerateFolderStorageKey(folder.StoragePath, sanitizedFilename)
		}
	}
	if storageKey == "" {
		_, storageKey = GenerateStorageKey(sanitizedFilename)
	}

	finalAlt := altText
	if finalAlt == "" {
		finalAlt = existing.Metadata.AltText
	}
	finalCaption := caption
	if finalCaption == "" {
		finalCaption = existing.Metadata.Caption
	}

	newMeta, err := s.processAndStoreMedia(ctx, rt, storageKey, existing.ID, originalName, mimeType, finalAlt, finalCaption, data)
	if err != nil {
		return nil, err
	}

	oldProv := s.storageFor(existing.StorageProvider)
	oldRef := assetRefFromAsset(existing)
	oldThumbs := existing.Metadata.Thumbnails

	existing.Filename = filepath.Base(storageKey)
	existing.OriginalName = originalName
	existing.MIMEType = mimeType
	existing.FileSize = actualSize
	existing.StorageProvider = s.storage.Name()
	existing.StorageKey = storageKey
	existing.Metadata = newMeta
	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, existing); err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		s.cleanupLocalThumbs(ctx, s.storage, newMeta.Thumbnails)
		return nil, fmt.Errorf("failed to update replaced media record: %w", err)
	}

	if oldProv != nil && oldRef.Key != "" {
		if op, ok := oldProv.(storage.AssetRefOperator); ok {
			if delErr := op.DeleteAsset(ctx, oldRef); delErr != nil && !errors.Is(delErr, storage.ErrFileNotFound) {
				s.logger.Warn("Failed deleting previous asset object after replace", "asset_id", existing.ID, "provider", oldProv.Name(), "error", delErr)
			}
		} else if delErr := oldProv.Delete(ctx, oldRef.Key); delErr != nil && !errors.Is(delErr, storage.ErrFileNotFound) {
			s.logger.Warn("Failed deleting previous asset object after replace", "asset_id", existing.ID, "provider", oldProv.Name(), "error", delErr)
		}
		s.cleanupLocalThumbs(ctx, oldProv, oldThumbs)
	}

	existing.URL, _ = s.ResolveAssetURL(ctx, existing)
	return existing, nil
}

// UploadHLSPackage processes and uploads a full HLS package zip archive.
// It verifies safe limits, checks playlist references, uploads files with bounded concurrency,
// and records a root HLS MediaAsset.
func (s *Service) UploadHLSPackage(
	ctx context.Context,
	r io.Reader,
	originalZipName string,
	size int64,
	title string,
	folderID *string,
) (*MediaAsset, error) {
	caps := s.Capabilities()
	if !caps.SupportsHLS {
		return nil, fmt.Errorf("%w: HLS zip packages require filesystem-style multi-file storage and are not supported on the active %q provider", ErrMediaValidation, caps.Provider)
	}

	// Write zip to a temporary file to allow random access for zip.Reader
	tmpFile, err := os.CreateTemp("", "hls_upload_*.zip")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file for HLS zip: %w", err)
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	written, err := io.Copy(tmpFile, io.LimitReader(r, MaxHLSZipSizeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to buffer HLS zip package: %w", err)
	}
	if written > MaxHLSZipSizeBytes {
		return nil, fmt.Errorf("HLS zip package exceeds maximum allowed size (%d MB)", MaxHLSZipSizeBytes/(1<<20))
	}

	zr, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to open HLS zip archive: %w", err)
	}
	defer zr.Close()

	// 1. Validate package structure, playlist integrity, and anti-zip-bomb limits
	entries, masterRelPath, meta, err := InspectAndValidateHLSPackage(&zr.Reader)
	if err != nil {
		return nil, err
	}

	// 2. Determine target storage directory prefix
	var folderStoragePath string
	var targetFolderID *string
	if folderID != nil && *folderID != "" {
		folder, err := s.repo.GetFolderByID(ctx, *folderID)
		if err != nil {
			return nil, fmt.Errorf("target folder not found: %w", err)
		}
		targetFolderID = folderID
		folderStoragePath = folder.StoragePath
	}

	pkgName := title
	if pkgName == "" {
		pkgName = strings.TrimSuffix(originalZipName, filepath.Ext(originalZipName))
	}
	pkgSlug := SlugifyFolder(pkgName)
	packagePrefix := fmt.Sprintf("%shls/%s_%s/", folderStoragePath, uuid.New().String()[:8], pkgSlug)

	// 3. Upload extracted entries with bounded concurrency
	var uploadErr error
	var uploadMu sync.Mutex
	var uploadedKeys []string

	entryChan := make(chan *ValidatedZipEntry, len(entries))
	for _, entry := range entries {
		entryChan <- entry
	}
	close(entryChan)

	var wg sync.WaitGroup
	ctxWorker, cancel := context.WithCancel(ctx)
	defer cancel()

	for w := 0; w < HLSUploadConcurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range entryChan {
				select {
				case <-ctxWorker.Done():
					return
				default:
				}

				rc, err := entry.File.Open()
				if err != nil {
					uploadMu.Lock()
					if uploadErr == nil {
						uploadErr = fmt.Errorf("failed to read entry %q: %w", entry.RelPath, err)
						cancel()
					}
					uploadMu.Unlock()
					return
				}

				storageKey := path.Join(packagePrefix, entry.RelPath)
				saveErr := s.storage.Save(ctxWorker, storageKey, rc, entry.Size, entry.MIMEType)
				_ = rc.Close()

				if saveErr != nil {
					uploadMu.Lock()
					if uploadErr == nil {
						uploadErr = fmt.Errorf("failed to save entry %q to storage: %w", entry.RelPath, saveErr)
						cancel()
					}
					uploadMu.Unlock()
					return
				}

				uploadMu.Lock()
				uploadedKeys = append(uploadedKeys, storageKey)
				uploadMu.Unlock()
			}
		}()
	}

	wg.Wait()

	if uploadErr != nil {
		// Compensating cleanup
		_ = s.storage.DeletePrefix(ctx, packagePrefix)
		return nil, uploadErr
	}

	// 4. Create root HLS MediaAsset
	assetID := uuid.New().String()
	masterStorageKey := path.Join(packagePrefix, masterRelPath)
	now := time.Now().UTC()

	var customMeta = map[string]any{
		"is_hls":             true,
		"package_type":       "hls",
		"master_playlist":    masterStorageKey,
		"package_prefix":     packagePrefix,
		"total_files":        meta.TotalFiles,
		"total_package_size": meta.TotalSize,
	}
	if len(meta.Variants) > 0 {
		customMeta["variants"] = meta.Variants
	}

	asset := &MediaAsset{
		ID:              assetID,
		FolderID:        targetFolderID,
		Filename:        pkgSlug + ".m3u8",
		OriginalName:    originalZipName,
		MIMEType:        "application/vnd.apple.mpegurl",
		FileSize:        meta.TotalSize,
		StorageProvider: s.storage.Name(),
		StorageKey:      masterStorageKey,
		Metadata: MediaMetadata{
			AltText: title,
			Caption: fmt.Sprintf("HLS Package (%d files, %.2f MB)", meta.TotalFiles, float64(meta.TotalSize)/(1024*1024)),
			Format:  "hls",
			Custom:  customMeta,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.Create(ctx, asset); err != nil {
		_ = s.storage.DeletePrefix(ctx, packagePrefix)
		return nil, fmt.Errorf("failed to save HLS asset in database: %w", err)
	}

	asset.URL, _ = s.ResolveURL(ctx, asset.StorageKey)
	return asset, nil
}

// GetAsset retrieves a media asset by ID with its resolved access URL.
func (s *Service) GetAsset(ctx context.Context, id string) (*MediaAsset, error) {
	asset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	asset.URL, _ = s.ResolveAssetURL(ctx, asset)
	return asset, nil
}

// ListAssets returns paginated media assets.
func (s *Service) ListAssets(ctx context.Context, filter MediaFilter) ([]*MediaAsset, int, error) {
	assets, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	for _, a := range assets {
		a.URL, _ = s.ResolveAssetURL(ctx, a)
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

	asset.URL, _ = s.ResolveAssetURL(ctx, asset)
	return asset, nil
}

// MoveAsset relocates an asset to a new folder and moves its storage key accordingly.
func (s *Service) MoveAsset(ctx context.Context, assetID string, targetFolderID *string) (*MediaAsset, error) {
	asset, err := s.repo.GetByID(ctx, assetID)
	if err != nil {
		return nil, err
	}

	var targetPrefix string
	if targetFolderID != nil && *targetFolderID != "" {
		tf, err := s.repo.GetFolderByID(ctx, *targetFolderID)
		if err != nil {
			return nil, fmt.Errorf("target folder not found: %w", err)
		}
		targetPrefix = tf.StoragePath
	} else {
		targetFolderID = nil
		targetPrefix = ""
	}

	filename := filepath.Base(asset.StorageKey)
	newStorageKey := targetPrefix + filename

	if newStorageKey != asset.StorageKey {
		prov := s.storageFor(asset.StorageProvider)
		oldRef := assetRefFromAsset(asset)
		prevKey := asset.StorageKey
		prevMeta := asset.Metadata

		if op, ok := prov.(storage.AssetRefOperator); ok {
			movedRes, moveErr := op.MoveAsset(ctx, oldRef, newStorageKey)
			if moveErr != nil {
				return nil, fmt.Errorf("failed to move file in storage: %w", moveErr)
			}
			applyUploadResultToMetadata(&asset.Metadata, movedRes)
		} else {
			if err := prov.Move(ctx, asset.StorageKey, newStorageKey); err != nil {
				return nil, fmt.Errorf("failed to move file in storage: %w", err)
			}
			for thumbName, oldThumbKey := range asset.Metadata.Thumbnails {
				if strings.HasPrefix(oldThumbKey, "http://") || strings.HasPrefix(oldThumbKey, "https://") {
					continue
				}
				newThumbKey := targetPrefix + filepath.Base(oldThumbKey)
				if err := prov.Move(ctx, oldThumbKey, newThumbKey); err == nil {
					asset.Metadata.Thumbnails[thumbName] = newThumbKey
				}
			}
		}

		// Update database record
		if err := s.repo.UpdateAssetFolder(ctx, asset.ID, targetFolderID, newStorageKey); err != nil {
			// Attempt compensating storage move back
			if op, ok := prov.(storage.AssetRefOperator); ok {
				_, _ = op.MoveAsset(ctx, assetRefFromAsset(asset), prevKey)
			} else {
				_ = prov.Move(ctx, newStorageKey, prevKey)
			}
			asset.Metadata = prevMeta
			return nil, fmt.Errorf("failed to update asset folder in database: %w", err)
		}

		asset.StorageKey = newStorageKey
		asset.Filename = filename
		asset.FolderID = targetFolderID
		_ = s.repo.Update(ctx, asset)
	}

	asset.URL, _ = s.ResolveAssetURL(ctx, asset)
	return asset, nil
}

// DeleteAsset removes the file and any thumbnails from storage and deletes the database record.
func (s *Service) DeleteAsset(ctx context.Context, id string) error {
	asset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	prov := s.storageFor(asset.StorageProvider)
	ref := assetRefFromAsset(asset)

	// If HLS package, delete full package prefix
	if asset.Metadata.Format == "hls" || asset.Metadata.Custom["is_hls"] == true {
		if pkgPrefix, ok := asset.Metadata.Custom["package_prefix"].(string); ok && pkgPrefix != "" {
			if delErr := prov.DeletePrefix(ctx, pkgPrefix); delErr != nil && !errors.Is(delErr, storage.ErrFileNotFound) {
				return fmt.Errorf("failed to delete HLS package from storage: %w", delErr)
			}
		}
	}

	// Delete primary file; abort DB removal on real remote/storage operational errors
	var primaryDelErr error
	if op, ok := prov.(storage.AssetRefOperator); ok {
		primaryDelErr = op.DeleteAsset(ctx, ref)
	} else {
		primaryDelErr = prov.Delete(ctx, asset.StorageKey)
	}
	if primaryDelErr != nil && !errors.Is(primaryDelErr, storage.ErrFileNotFound) {
		return fmt.Errorf("failed to delete file from storage: %w", primaryDelErr)
	}

	// Delete locally stored thumbnails (external CDN variant URLs are destroyed automatically with the primary asset)
	for _, thumbKey := range asset.Metadata.Thumbnails {
		if strings.HasPrefix(thumbKey, "http://") || strings.HasPrefix(thumbKey, "https://") {
			continue
		}
		_ = prov.Delete(ctx, thumbKey)
	}

	return s.repo.Delete(ctx, id)
}

// CreateFolder creates a new folder in the media library and sets up its path hierarchy.
func (s *Service) CreateFolder(ctx context.Context, name string, parentID *string, createdBy *string) (*MediaFolder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("folder name cannot be empty")
	}

	slug := SlugifyFolder(name)

	var path, storagePath string
	if parentID != nil && *parentID != "" {
		parent, err := s.repo.GetFolderByID(ctx, *parentID)
		if err != nil {
			return nil, fmt.Errorf("parent folder not found: %w", err)
		}
		path = parent.Path + "/" + slug
		storagePath = parent.StoragePath + slug + "/"
	} else {
		parentID = nil
		path = slug
		storagePath = slug + "/"
	}

	if _, err := s.repo.GetFolderByPath(ctx, path); err == nil {
		return nil, ErrDuplicateFolder
	}

	now := time.Now().UTC()
	folder := &MediaFolder{
		ID:          uuid.New().String(),
		Name:        name,
		Slug:        slug,
		ParentID:    parentID,
		Path:        path,
		StoragePath: storagePath,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateFolder(ctx, folder); err != nil {
		return nil, err
	}

	return folder, nil
}

// GetFolder retrieves a folder by ID.
func (s *Service) GetFolder(ctx context.Context, id string) (*MediaFolder, error) {
	return s.repo.GetFolderByID(ctx, id)
}

// ListFolders returns folders under parentID (or top-level folders if parentID is nil or empty).
func (s *Service) ListFolders(ctx context.Context, parentID *string) ([]*MediaFolder, error) {
	return s.repo.ListFolders(ctx, parentID)
}

// GetFolderTree returns the complete folder hierarchy as a tree with asset counts.
func (s *Service) GetFolderTree(ctx context.Context) ([]*MediaFolderTreeNode, error) {
	allFolders, err := s.repo.GetAllFolders(ctx)
	if err != nil {
		return nil, err
	}

	nodeMap := make(map[string]*MediaFolderTreeNode)
	for _, f := range allFolders {
		node := &MediaFolderTreeNode{
			MediaFolder: *f,
			Children:    make([]*MediaFolderTreeNode, 0),
		}
		assets, _ := s.repo.GetAssetsByFolderID(ctx, f.ID)
		node.AssetCount = len(assets)
		nodeMap[f.ID] = node
	}

	var roots []*MediaFolderTreeNode
	for _, f := range allFolders {
		node := nodeMap[f.ID]
		if f.ParentID == nil || *f.ParentID == "" || nodeMap[*f.ParentID] == nil {
			roots = append(roots, node)
		} else {
			parent := nodeMap[*f.ParentID]
			parent.Children = append(parent.Children, node)
		}
	}

	return roots, nil
}

func (s *Service) relocateFolderTree(ctx context.Context, rootFolderID, oldPath, newPath, oldStoragePath, newStoragePath string) {
	// Attempt fast prefix/directory move on active provider first
	if err := s.storage.Move(ctx, oldStoragePath, newStoragePath); err != nil {
		s.logger.Debug("Directory prefix move note", "error", err, "old", oldStoragePath, "new", newStoragePath)
	}

	affectedFolderIDs := []string{rootFolderID}
	allFolders, _ := s.repo.GetAllFolders(ctx)
	for _, sub := range allFolders {
		if strings.HasPrefix(sub.Path, oldPath+"/") {
			affectedFolderIDs = append(affectedFolderIDs, sub.ID)
			sub.Path = strings.Replace(sub.Path, oldPath, newPath, 1)
			sub.StoragePath = strings.Replace(sub.StoragePath, oldStoragePath, newStoragePath, 1)
			_ = s.repo.UpdateFolder(ctx, sub)
		}
	}

	for _, fid := range affectedFolderIDs {
		assets, _ := s.repo.GetAssetsByFolderID(ctx, fid)
		for _, a := range assets {
			newKey := strings.Replace(a.StorageKey, oldStoragePath, newStoragePath, 1)
			if newKey == a.StorageKey {
				continue
			}
			prov := s.storageFor(a.StorageProvider)
			if op, ok := prov.(storage.AssetRefOperator); ok {
				if movedRes, moveErr := op.MoveAsset(ctx, assetRefFromAsset(a), newKey); moveErr == nil {
					applyUploadResultToMetadata(&a.Metadata, movedRes)
					a.StorageKey = newKey
					_ = s.repo.Update(ctx, a)
				}
			} else if len(a.Metadata.Thumbnails) > 0 {
				for tn, tk := range a.Metadata.Thumbnails {
					if strings.HasPrefix(tk, "http://") || strings.HasPrefix(tk, "https://") {
						continue
					}
					a.Metadata.Thumbnails[tn] = strings.Replace(tk, oldStoragePath, newStoragePath, 1)
				}
				_ = s.repo.Update(ctx, a)
			}
			_ = s.repo.UpdateAssetFolder(ctx, a.ID, a.FolderID, newKey)
		}
	}
}

// RenameFolder modifies a folder's name and synchronizes its path and storage prefix.
func (s *Service) RenameFolder(ctx context.Context, id string, newName string) (*MediaFolder, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return nil, errors.New("folder name cannot be empty")
	}

	folder, err := s.repo.GetFolderByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if folder.Name == newName {
		return folder, nil
	}

	newSlug := SlugifyFolder(newName)
	oldPath := folder.Path
	oldStoragePath := folder.StoragePath

	var newPath, newStoragePath string
	if folder.ParentID != nil && *folder.ParentID != "" {
		parent, err := s.repo.GetFolderByID(ctx, *folder.ParentID)
		if err != nil {
			return nil, err
		}
		newPath = parent.Path + "/" + newSlug
		newStoragePath = parent.StoragePath + newSlug + "/"
	} else {
		newPath = newSlug
		newStoragePath = newSlug + "/"
	}

	if newPath != oldPath {
		if existing, err := s.repo.GetFolderByPath(ctx, newPath); err == nil && existing.ID != folder.ID {
			return nil, ErrDuplicateFolder
		}
		s.relocateFolderTree(ctx, folder.ID, oldPath, newPath, oldStoragePath, newStoragePath)
	}

	folder.Name = newName
	folder.Slug = newSlug
	folder.Path = newPath
	folder.StoragePath = newStoragePath

	if err := s.repo.UpdateFolder(ctx, folder); err != nil {
		return nil, err
	}

	return folder, nil
}

// MoveFolder moves a folder to a new parent, updating its path and descendants.
func (s *Service) MoveFolder(ctx context.Context, id string, targetParentID *string) (*MediaFolder, error) {
	folder, err := s.repo.GetFolderByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Moving to itself
	if targetParentID != nil && *targetParentID == id {
		return nil, ErrCircularFolder
	}

	// Verify target parent is not a descendant
	if targetParentID != nil && *targetParentID != "" {
		allFolders, _ := s.repo.GetAllFolders(ctx)
		isDescendant := false
		var checkID = *targetParentID
		for checkID != "" {
			if checkID == id {
				isDescendant = true
				break
			}
			found := false
			for _, f := range allFolders {
				if f.ID == checkID {
					if f.ParentID != nil {
						checkID = *f.ParentID
						found = true
					}
					break
				}
			}
			if !found {
				break
			}
		}
		if isDescendant {
			return nil, ErrCircularFolder
		}
	}

	oldPath := folder.Path
	oldStoragePath := folder.StoragePath

	var newPath, newStoragePath string
	if targetParentID != nil && *targetParentID != "" {
		parent, err := s.repo.GetFolderByID(ctx, *targetParentID)
		if err != nil {
			return nil, fmt.Errorf("target parent folder not found: %w", err)
		}
		newPath = parent.Path + "/" + folder.Slug
		newStoragePath = parent.StoragePath + folder.Slug + "/"
	} else {
		targetParentID = nil
		newPath = folder.Slug
		newStoragePath = folder.Slug + "/"
	}

	if newPath != oldPath {
		if existing, err := s.repo.GetFolderByPath(ctx, newPath); err == nil && existing.ID != folder.ID {
			return nil, ErrDuplicateFolder
		}
		s.relocateFolderTree(ctx, folder.ID, oldPath, newPath, oldStoragePath, newStoragePath)
	}

	folder.ParentID = targetParentID
	folder.Path = newPath
	folder.StoragePath = newStoragePath

	if err := s.repo.UpdateFolder(ctx, folder); err != nil {
		return nil, err
	}

	return folder, nil
}

// DeleteFolder removes a folder from the library.
// If recursive is false, it returns an error if the folder contains subfolders or media assets.
func (s *Service) DeleteFolder(ctx context.Context, id string, recursive bool) error {
	folder, err := s.repo.GetFolderByID(ctx, id)
	if err != nil {
		return err
	}

	subs, err := s.repo.GetSubfolders(ctx, id)
	if err != nil {
		return err
	}
	assets, err := s.repo.GetAssetsByFolderID(ctx, id)
	if err != nil {
		return err
	}

	if !recursive && (len(subs) > 0 || len(assets) > 0) {
		return ErrFolderNotEmpty
	}

	if recursive {
		// Delete each asset via its recorded storage provider, plus prefix cleanup on active storage
		for _, a := range assets {
			if err := s.DeleteAsset(ctx, a.ID); err != nil {
				s.logger.Warn("Failed deleting asset during recursive folder delete", "asset_id", a.ID, "error", err)
			}
		}
		_ = s.storage.DeletePrefix(ctx, folder.StoragePath)

		// Recursively delete subfolders
		for _, sub := range subs {
			_ = s.DeleteFolder(ctx, sub.ID, true)
		}
	}

	return s.repo.DeleteFolder(ctx, id)
}

// GenerateSignedURL generates a temporary signed access URL for the asset.
func (s *Service) GenerateSignedURL(ctx context.Context, id string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = s.DefaultSignedURLTTL(ctx)
	}
	if ttl > maxSignedURLTTL {
		return "", fmt.Errorf("%w: signed URL TTL must be between 1 second and 7 days", ErrMediaValidation)
	}
	asset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}

	prov := s.storageFor(asset.StorageProvider)
	ref := assetRefFromAsset(asset)
	if sg, ok := prov.(storage.SignedAssetURLGenerator); ok {
		return sg.SignedURL(ctx, ref, int64(ttl.Seconds()))
	}

	var rawURL string
	if op, ok := prov.(storage.AssetRefOperator); ok {
		rawURL, err = op.AssetURL(ctx, ref)
	} else {
		rawURL, err = prov.URL(ctx, asset.StorageKey)
	}
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

// ResolveAssetURL generates the access URL for an asset using its recorded storage provider.
func (s *Service) ResolveAssetURL(ctx context.Context, asset *MediaAsset) (string, error) {
	if asset == nil {
		return "", nil
	}
	prov := s.storageFor(asset.StorageProvider)
	if prov == nil {
		return "", nil
	}
	ref := assetRefFromAsset(asset)
	var (
		rawURL string
		err    error
	)
	if op, ok := prov.(storage.AssetRefOperator); ok {
		rawURL, err = op.AssetURL(ctx, ref)
	} else {
		rawURL, err = prov.URL(ctx, asset.StorageKey)
	}
	if err != nil {
		return "", err
	}

	// Local/relative paths honor Go CMS HMAC signed URL enforcement; remote providers use their own SignedURL when needed.
	if s.ShouldRequireSignedURLs(ctx) && s.signer != nil && !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		tenantID := ""
		if tenant := tenancy.GetTenant(ctx); tenant != nil {
			tenantID = tenant.ID
		}
		return s.signer.SignForTenant(rawURL, s.DefaultSignedURLTTL(ctx), tenantID)
	}
	return rawURL, nil
}

// TransformAssetURL builds a validated transformed delivery URL when the asset's provider supports it.
func (s *Service) TransformAssetURL(ctx context.Context, id string, opts storage.ImageTransformOptions) (string, error) {
	asset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	prov := s.storageFor(asset.StorageProvider)
	if tx, ok := prov.(storage.TransformURLGenerator); ok {
		return tx.TransformURL(ctx, assetRefFromAsset(asset), opts)
	}
	return s.ResolveAssetURL(ctx, asset)
}

// ResolveURL generates the standard access URL for an asset key, applying signatures if required.
func (s *Service) ResolveURL(ctx context.Context, storageKey string) (string, error) {
	rawURL, err := s.storage.URL(ctx, storageKey)
	if err != nil {
		return "", err
	}

	if s.ShouldRequireSignedURLs(ctx) && s.signer != nil && !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		tenantID := ""
		if tenant := tenancy.GetTenant(ctx); tenant != nil {
			tenantID = tenant.ID
		}
		return s.signer.SignForTenant(rawURL, s.DefaultSignedURLTTL(ctx), tenantID)
	}

	return rawURL, nil
}

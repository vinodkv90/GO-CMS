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

	var targetFolderID *string
	var assetID, storageKey string

	if len(folderID) > 0 && folderID[0] != nil && *folderID[0] != "" {
		fid := *folderID[0]
		folder, err := s.repo.GetFolderByID(ctx, fid)
		if err != nil {
			return nil, fmt.Errorf("target folder not found: %w", err)
		}
		targetFolderID = &fid
		assetID, storageKey = GenerateFolderStorageKey(folder.StoragePath, sanitizedName)
	} else {
		assetID, storageKey = GenerateStorageKey(sanitizedName)
	}

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
		// Cleanup saved file on database failure
		_ = s.storage.Delete(ctx, storageKey)
		return nil, fmt.Errorf("failed to save media record in database: %w", err)
	}

	// Resolve asset URL
	asset.URL, _ = s.ResolveURL(ctx, asset.StorageKey)
	return asset, nil
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
		// Move primary file in storage
		if err := s.storage.Move(ctx, asset.StorageKey, newStorageKey); err != nil {
			return nil, fmt.Errorf("failed to move file in storage: %w", err)
		}

		// Move thumbnails
		for thumbName, oldThumbKey := range asset.Metadata.Thumbnails {
			newThumbKey := targetPrefix + filepath.Base(oldThumbKey)
			if err := s.storage.Move(ctx, oldThumbKey, newThumbKey); err == nil {
				asset.Metadata.Thumbnails[thumbName] = newThumbKey
			}
		}

		// Update database record
		if err := s.repo.UpdateAssetFolder(ctx, asset.ID, targetFolderID, newStorageKey); err != nil {
			// Attempt compensating storage move back
			_ = s.storage.Move(ctx, newStorageKey, asset.StorageKey)
			return nil, fmt.Errorf("failed to update asset folder in database: %w", err)
		}

		asset.StorageKey = newStorageKey
		asset.Filename = filename
		asset.FolderID = targetFolderID
		_ = s.repo.Update(ctx, asset)
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

	// If HLS package, delete full package prefix
	if asset.Metadata.Format == "hls" || asset.Metadata.Custom["is_hls"] == true {
		if pkgPrefix, ok := asset.Metadata.Custom["package_prefix"].(string); ok && pkgPrefix != "" {
			_ = s.storage.DeletePrefix(ctx, pkgPrefix)
		}
	}

	// Delete primary file
	_ = s.storage.Delete(ctx, asset.StorageKey)

	// Delete thumbnails
	for _, thumbKey := range asset.Metadata.Thumbnails {
		_ = s.storage.Delete(ctx, thumbKey)
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

		// Move storage directory
		if err := s.storage.Move(ctx, oldStoragePath, newStoragePath); err != nil {
			s.logger.Warn("Storage move folder prefix note", "error", err, "old", oldStoragePath, "new", newStoragePath)
		}

		// Update descendant folders
		allFolders, _ := s.repo.GetAllFolders(ctx)
		for _, sub := range allFolders {
			if strings.HasPrefix(sub.Path, oldPath+"/") {
				sub.Path = strings.Replace(sub.Path, oldPath, newPath, 1)
				sub.StoragePath = strings.Replace(sub.StoragePath, oldStoragePath, newStoragePath, 1)
				_ = s.repo.UpdateFolder(ctx, sub)
			}
		}

		// Update descendant assets
		assets, _ := s.repo.GetAssetsByFolderID(ctx, folder.ID)
		for _, a := range assets {
			newKey := strings.Replace(a.StorageKey, oldStoragePath, newStoragePath, 1)
			_ = s.repo.UpdateAssetFolder(ctx, a.ID, a.FolderID, newKey)
		}
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

		if err := s.storage.Move(ctx, oldStoragePath, newStoragePath); err != nil {
			s.logger.Warn("Storage move folder prefix note", "error", err)
		}

		// Update descendant folders
		allFolders, _ := s.repo.GetAllFolders(ctx)
		for _, sub := range allFolders {
			if strings.HasPrefix(sub.Path, oldPath+"/") {
				sub.Path = strings.Replace(sub.Path, oldPath, newPath, 1)
				sub.StoragePath = strings.Replace(sub.StoragePath, oldStoragePath, newStoragePath, 1)
				_ = s.repo.UpdateFolder(ctx, sub)
			}
		}

		// Update descendant assets
		assets, _ := s.repo.GetAssetsByFolderID(ctx, folder.ID)
		for _, a := range assets {
			newKey := strings.Replace(a.StorageKey, oldStoragePath, newStoragePath, 1)
			_ = s.repo.UpdateAssetFolder(ctx, a.ID, a.FolderID, newKey)
		}
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
		// Clean up storage prefix
		_ = s.storage.DeletePrefix(ctx, folder.StoragePath)

		// Delete all assets under folder and subfolders
		for _, a := range assets {
			_ = s.repo.Delete(ctx, a.ID)
		}

		// Recursively delete subfolders
		for _, sub := range subs {
			_ = s.DeleteFolder(ctx, sub.ID, true)
		}
	}

	return s.repo.DeleteFolder(ctx, id)
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

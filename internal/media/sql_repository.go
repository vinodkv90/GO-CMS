package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/database/interfaces"
)

// SQLRepository implements Repository using standard SQL databases.
type SQLRepository struct {
	db interfaces.SQLDatabase
}

// NewSQLRepository creates a new SQL media repository.
func NewSQLRepository(db interfaces.SQLDatabase) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) rebind(query string) string {
	if r.db.DriverName() != "postgres" {
		return query
	}

	var sb strings.Builder
	paramIdx := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			sb.WriteByte('$')
			sb.WriteString(strconv.Itoa(paramIdx))
			paramIdx++
		} else {
			sb.WriteByte(query[i])
		}
	}
	return sb.String()
}

// Create inserts a new media record into cms_media.
func (r *SQLRepository) Create(ctx context.Context, asset *MediaAsset) error {
	metaJSON, err := json.Marshal(asset.Metadata)
	if err != nil {
		metaJSON = []byte("{}")
	}

	query := r.rebind(`
		INSERT INTO cms_media (
			id, folder_id, filename, original_name, mime_type, file_size,
			storage_provider, storage_key, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)

	now := time.Now().UTC()
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = now
	}
	if asset.UpdatedAt.IsZero() {
		asset.UpdatedAt = now
	}

	_, err = r.db.ExecContext(
		ctx,
		query,
		asset.ID,
		asset.FolderID,
		asset.Filename,
		asset.OriginalName,
		asset.MIMEType,
		asset.FileSize,
		asset.StorageProvider,
		asset.StorageKey,
		string(metaJSON),
		asset.CreatedAt,
		asset.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert media record: %w", err)
	}

	return nil
}

// GetByID retrieves a media record by ID.
func (r *SQLRepository) GetByID(ctx context.Context, id string) (*MediaAsset, error) {
	query := r.rebind(`
		SELECT id, folder_id, filename, original_name, mime_type, file_size,
		       storage_provider, storage_key, metadata, created_at, updated_at
		FROM cms_media
		WHERE id = ?
	`)

	row := r.db.QueryRowContext(ctx, query, id)
	a, err := scanAsset(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAssetNotFound
		}
		return nil, fmt.Errorf("failed to scan media row: %w", err)
	}
	return a, nil
}

// GetByKey retrieves a media record by its storage key.
func (r *SQLRepository) GetByKey(ctx context.Context, storageKey string) (*MediaAsset, error) {
	query := r.rebind(`
		SELECT id, folder_id, filename, original_name, mime_type, file_size,
		       storage_provider, storage_key, metadata, created_at, updated_at
		FROM cms_media
		WHERE storage_key = ?
	`)

	row := r.db.QueryRowContext(ctx, query, storageKey)
	a, err := scanAsset(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAssetNotFound
		}
		return nil, fmt.Errorf("failed to scan media row: %w", err)
	}
	return a, nil
}

// List queries media assets with pagination and filters.
func (r *SQLRepository) List(ctx context.Context, filter MediaFilter) ([]*MediaAsset, int, error) {
	var whereClauses []string
	var args []any

	if filter.FolderID != nil {
		if *filter.FolderID == "" {
			whereClauses = append(whereClauses, "folder_id IS NULL")
		} else {
			whereClauses = append(whereClauses, "folder_id = ?")
			args = append(args, *filter.FolderID)
		}
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, "(filename LIKE ? OR original_name LIKE ?)")
		searchTerm := "%" + filter.Search + "%"
		args = append(args, searchTerm, searchTerm)
	}

	if filter.MIMEType != "" {
		if strings.HasSuffix(filter.MIMEType, "/*") || strings.HasSuffix(filter.MIMEType, "/") {
			prefix := strings.TrimSuffix(strings.TrimSuffix(filter.MIMEType, "*"), "/")
			whereClauses = append(whereClauses, "mime_type LIKE ?")
			args = append(args, prefix+"/%")
		} else {
			whereClauses = append(whereClauses, "mime_type = ?")
			args = append(args, filter.MIMEType)
		}
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// 1. Get Total Count
	countQuery := r.rebind("SELECT COUNT(*) FROM cms_media" + whereSQL)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count media assets: %w", err)
	}

	// 2. Fetch Records
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	selectSQL := fmt.Sprintf(`
		SELECT id, folder_id, filename, original_name, mime_type, file_size,
		       storage_provider, storage_key, metadata, created_at, updated_at
		FROM cms_media
		%s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, whereSQL)

	fetchArgs := append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, r.rebind(selectSQL), fetchArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list media assets: %w", err)
	}
	defer rows.Close()

	var assets []*MediaAsset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan media row: %w", err)
		}
		assets = append(assets, a)
	}

	return assets, total, nil
}

// Update modifies an existing asset's metadata.
func (r *SQLRepository) Update(ctx context.Context, asset *MediaAsset) error {
	metaJSON, err := json.Marshal(asset.Metadata)
	if err != nil {
		metaJSON = []byte("{}")
	}

	query := r.rebind(`
		UPDATE cms_media
		SET filename = ?, original_name = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`)

	asset.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, query, asset.Filename, asset.OriginalName, string(metaJSON), asset.UpdatedAt, asset.ID)
	if err != nil {
		return fmt.Errorf("failed to update media record: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrAssetNotFound
	}

	return nil
}

// UpdateAssetFolder moves an asset to a new folder and updates its storage key.
func (r *SQLRepository) UpdateAssetFolder(ctx context.Context, assetID string, targetFolderID *string, newStorageKey string) error {
	filename := filepath.Base(newStorageKey)
	now := time.Now().UTC()
	query := r.rebind(`
		UPDATE cms_media
		SET folder_id = ?, storage_key = ?, filename = ?, updated_at = ?
		WHERE id = ?
	`)
	res, err := r.db.ExecContext(ctx, query, targetFolderID, newStorageKey, filename, now, assetID)
	if err != nil {
		return fmt.Errorf("failed to update asset folder: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrAssetNotFound
	}
	return nil
}

// Delete removes an asset record from cms_media.
func (r *SQLRepository) Delete(ctx context.Context, id string) error {
	query := r.rebind("DELETE FROM cms_media WHERE id = ?")
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete media record: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrAssetNotFound
	}

	return nil
}

// CreateFolder creates a new media folder.
func (r *SQLRepository) CreateFolder(ctx context.Context, folder *MediaFolder) error {
	now := time.Now().UTC()
	if folder.CreatedAt.IsZero() {
		folder.CreatedAt = now
	}
	if folder.UpdatedAt.IsZero() {
		folder.UpdatedAt = now
	}

	query := r.rebind(`
		INSERT INTO cms_media_folders (
			id, name, slug, parent_id, path, storage_path, created_at, updated_at, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)

	_, err := r.db.ExecContext(
		ctx,
		query,
		folder.ID,
		folder.Name,
		folder.Slug,
		folder.ParentID,
		folder.Path,
		folder.StoragePath,
		folder.CreatedAt,
		folder.UpdatedAt,
		folder.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to insert media folder: %w", err)
	}
	return nil
}

// GetFolderByID retrieves a media folder by ID.
func (r *SQLRepository) GetFolderByID(ctx context.Context, id string) (*MediaFolder, error) {
	query := r.rebind(`
		SELECT id, name, slug, parent_id, path, storage_path, created_at, updated_at, created_by
		FROM cms_media_folders
		WHERE id = ?
	`)
	row := r.db.QueryRowContext(ctx, query, id)
	f, err := scanFolder(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrFolderNotFound
		}
		return nil, fmt.Errorf("failed to get folder by id: %w", err)
	}
	return f, nil
}

// GetFolderByPath retrieves a folder by canonical path.
func (r *SQLRepository) GetFolderByPath(ctx context.Context, path string) (*MediaFolder, error) {
	query := r.rebind(`
		SELECT id, name, slug, parent_id, path, storage_path, created_at, updated_at, created_by
		FROM cms_media_folders
		WHERE path = ?
	`)
	row := r.db.QueryRowContext(ctx, query, path)
	f, err := scanFolder(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrFolderNotFound
		}
		return nil, fmt.Errorf("failed to get folder by path: %w", err)
	}
	return f, nil
}

// ListFolders returns folders under parentID (or all folders if parentID is nil).
func (r *SQLRepository) ListFolders(ctx context.Context, parentID *string) ([]*MediaFolder, error) {
	var query string
	var args []any

	if parentID == nil {
		query = r.rebind(`
			SELECT id, name, slug, parent_id, path, storage_path, created_at, updated_at, created_by
			FROM cms_media_folders
			ORDER BY name ASC
		`)
	} else if *parentID == "" {
		query = r.rebind(`
			SELECT id, name, slug, parent_id, path, storage_path, created_at, updated_at, created_by
			FROM cms_media_folders
			WHERE parent_id IS NULL
			ORDER BY name ASC
		`)
	} else {
		query = r.rebind(`
			SELECT id, name, slug, parent_id, path, storage_path, created_at, updated_at, created_by
			FROM cms_media_folders
			WHERE parent_id = ?
			ORDER BY name ASC
		`)
		args = append(args, *parentID)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}
	defer rows.Close()

	var folders []*MediaFolder
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan folder: %w", err)
		}
		folders = append(folders, f)
	}
	return folders, nil
}

// GetAllFolders returns all folders ordered by path.
func (r *SQLRepository) GetAllFolders(ctx context.Context) ([]*MediaFolder, error) {
	query := r.rebind(`
		SELECT id, name, slug, parent_id, path, storage_path, created_at, updated_at, created_by
		FROM cms_media_folders
		ORDER BY path ASC
	`)
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all folders: %w", err)
	}
	defer rows.Close()

	var folders []*MediaFolder
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan folder: %w", err)
		}
		folders = append(folders, f)
	}
	return folders, nil
}

// UpdateFolder updates folder metadata and path.
func (r *SQLRepository) UpdateFolder(ctx context.Context, folder *MediaFolder) error {
	folder.UpdatedAt = time.Now().UTC()
	query := r.rebind(`
		UPDATE cms_media_folders
		SET name = ?, slug = ?, parent_id = ?, path = ?, storage_path = ?, updated_at = ?
		WHERE id = ?
	`)
	res, err := r.db.ExecContext(ctx, query, folder.Name, folder.Slug, folder.ParentID, folder.Path, folder.StoragePath, folder.UpdatedAt, folder.ID)
	if err != nil {
		return fmt.Errorf("failed to update folder: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrFolderNotFound
	}
	return nil
}

// DeleteFolder removes a folder from the database.
func (r *SQLRepository) DeleteFolder(ctx context.Context, id string) error {
	query := r.rebind("DELETE FROM cms_media_folders WHERE id = ?")
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete folder: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrFolderNotFound
	}
	return nil
}

// GetAssetsByFolderID retrieves all media assets within a folder.
func (r *SQLRepository) GetAssetsByFolderID(ctx context.Context, folderID string) ([]*MediaAsset, error) {
	query := r.rebind(`
		SELECT id, folder_id, filename, original_name, mime_type, file_size,
		       storage_provider, storage_key, metadata, created_at, updated_at
		FROM cms_media
		WHERE folder_id = ?
		ORDER BY created_at DESC
	`)
	rows, err := r.db.QueryContext(ctx, query, folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get assets by folder: %w", err)
	}
	defer rows.Close()

	var assets []*MediaAsset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan asset: %w", err)
		}
		assets = append(assets, a)
	}
	return assets, nil
}

// GetSubfolders returns subfolders directly under parentID.
func (r *SQLRepository) GetSubfolders(ctx context.Context, parentID string) ([]*MediaFolder, error) {
	p := &parentID
	return r.ListFolders(ctx, p)
}

func scanAsset(scanner interface{ Scan(dest ...any) error }) (*MediaAsset, error) {
	var a MediaAsset
	var folderID sql.NullString
	var metaRaw []byte

	err := scanner.Scan(
		&a.ID,
		&folderID,
		&a.Filename,
		&a.OriginalName,
		&a.MIMEType,
		&a.FileSize,
		&a.StorageProvider,
		&a.StorageKey,
		&metaRaw,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if folderID.Valid {
		a.FolderID = &folderID.String
	}
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &a.Metadata)
	}

	return &a, nil
}

func scanFolder(scanner interface{ Scan(dest ...any) error }) (*MediaFolder, error) {
	var f MediaFolder
	var parentID sql.NullString
	var createdBy sql.NullString

	err := scanner.Scan(
		&f.ID,
		&f.Name,
		&f.Slug,
		&parentID,
		&f.Path,
		&f.StoragePath,
		&f.CreatedAt,
		&f.UpdatedAt,
		&createdBy,
	)
	if err != nil {
		return nil, err
	}

	if parentID.Valid {
		f.ParentID = &parentID.String
	}
	if createdBy.Valid {
		f.CreatedBy = &createdBy.String
	}

	return &f, nil
}

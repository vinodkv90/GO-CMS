package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
			id, filename, original_name, mime_type, file_size,
			storage_provider, storage_key, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		SELECT id, filename, original_name, mime_type, file_size,
		       storage_provider, storage_key, metadata, created_at, updated_at
		FROM cms_media
		WHERE id = ?
	`)

	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanRow(row)
}

// GetByKey retrieves a media record by its storage key.
func (r *SQLRepository) GetByKey(ctx context.Context, storageKey string) (*MediaAsset, error) {
	query := r.rebind(`
		SELECT id, filename, original_name, mime_type, file_size,
		       storage_provider, storage_key, metadata, created_at, updated_at
		FROM cms_media
		WHERE storage_key = ?
	`)

	row := r.db.QueryRowContext(ctx, query, storageKey)
	return r.scanRow(row)
}

// List queries media assets with pagination and filters.
func (r *SQLRepository) List(ctx context.Context, filter MediaFilter) ([]*MediaAsset, int, error) {
	var whereClauses []string
	var args []any

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
		SELECT id, filename, original_name, mime_type, file_size,
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
		var a MediaAsset
		var metaRaw []byte
		err := rows.Scan(
			&a.ID,
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
			return nil, 0, fmt.Errorf("failed to scan media row: %w", err)
		}

		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &a.Metadata)
		}

		assets = append(assets, &a)
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

func (r *SQLRepository) scanRow(row *sql.Row) (*MediaAsset, error) {
	var a MediaAsset
	var metaRaw []byte

	err := row.Scan(
		&a.ID,
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAssetNotFound
		}
		return nil, fmt.Errorf("failed to scan media row: %w", err)
	}

	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &a.Metadata)
	}

	return &a, nil
}


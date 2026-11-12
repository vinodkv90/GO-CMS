package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/go-cms/cms/internal/fields"
)

// SQLContentRepository implements ContentTypeRepository, ContentEntryRepository, and ContentVersionRepository.
type SQLContentRepository struct {
	db     sqlContentRunner
	root   interfaces.SQLDatabase
	driver string
}

// NewSQLContentRepository creates a new SQLContentRepository.
func NewSQLContentRepository(db interfaces.SQLDatabase) *SQLContentRepository {
	return &SQLContentRepository{
		db:     db,
		root:   db,
		driver: db.DriverName(),
	}
}

func (r *SQLContentRepository) rebind(query string) string {
	if r.driver != "postgres" {
		return query
	}

	var b strings.Builder
	paramIndex := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			b.WriteString(fmt.Sprintf("$%d", paramIndex))
			paramIndex++
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

// --- ContentTypeRepository ---

func (r *SQLContentRepository) rawCreateType(ctx context.Context, ct *ContentType) error {
	settingsJSON, err := json.Marshal(ct.Settings)
	if err != nil {
		return err
	}

	query := r.rebind(`
		INSERT INTO content_types (id, uid, name, kind, settings, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	_, err = r.db.ExecContext(ctx, query, ct.ID, ct.UID, ct.Name, string(ct.Kind), string(settingsJSON), ct.CreatedAt, ct.UpdatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return ErrContentTypeExists
		}
		return fmt.Errorf("failed to create content type: %w", err)
	}
	return nil
}

func (r *SQLContentRepository) GetTypeByID(ctx context.Context, id string) (*ContentType, error) {
	query := r.rebind(`SELECT id, uid, name, kind, settings, created_at, updated_at FROM content_types WHERE id = ?`)
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanContentType(ctx, row)
}

func (r *SQLContentRepository) GetTypeByUID(ctx context.Context, uid string) (*ContentType, error) {
	query := r.rebind(`SELECT id, uid, name, kind, settings, created_at, updated_at FROM content_types WHERE LOWER(uid) = LOWER(?)`)
	row := r.db.QueryRowContext(ctx, query, uid)
	return r.scanContentType(ctx, row)
}

func (r *SQLContentRepository) rawUpdateType(ctx context.Context, ct *ContentType) error {
	settingsJSON, err := json.Marshal(ct.Settings)
	if err != nil {
		return err
	}

	query := r.rebind(`
		UPDATE content_types
		SET name = ?, settings = ?, updated_at = ?
		WHERE id = ?
	`)
	res, err := r.db.ExecContext(ctx, query, ct.Name, string(settingsJSON), ct.UpdatedAt, ct.ID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrContentTypeNotFound
	}
	return nil
}

func (r *SQLContentRepository) rawDeleteType(ctx context.Context, id string) error {
	query := r.rebind(`DELETE FROM content_types WHERE id = ?`)
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrContentTypeNotFound
	}
	return nil
}

func (r *SQLContentRepository) ListTypes(ctx context.Context) ([]*ContentType, error) {
	query := `SELECT id, uid, name, kind, settings, created_at, updated_at FROM content_types ORDER BY name ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []*ContentType
	for rows.Next() {
		var ct ContentType
		var kind string
		var settingsRaw []byte
		if err := rows.Scan(&ct.ID, &ct.UID, &ct.Name, &kind, &settingsRaw, &ct.CreatedAt, &ct.UpdatedAt); err != nil {
			return nil, err
		}
		ct.Kind = ContentKind(kind)
		if len(settingsRaw) > 0 {
			_ = json.Unmarshal(settingsRaw, &ct.Settings)
		}
		types = append(types, &ct)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, ct := range types {
		fieldsList, err := r.ListFields(ctx, ct.ID)
		if err != nil {
			return nil, err
		}
		ct.Fields = fieldsList
	}
	return types, nil
}

func (r *SQLContentRepository) rawCreateField(ctx context.Context, f *ContentField) error {
	configJSON, err := json.Marshal(f.Configuration)
	if err != nil {
		return err
	}

	query := r.rebind(`
		INSERT INTO content_fields (id, content_type_id, name, field_key, field_type, configuration, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	_, err = r.db.ExecContext(ctx, query, f.ID, f.ContentTypeID, f.Name, f.FieldKey, string(f.FieldType), string(configJSON), f.CreatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return ErrFieldExists
		}
		return err
	}
	return nil
}

func (r *SQLContentRepository) ListFields(ctx context.Context, contentTypeID string) ([]ContentField, error) {
	query := r.rebind(`
		SELECT id, content_type_id, name, field_key, field_type, configuration, created_at
		FROM content_fields
		WHERE content_type_id = ?
		ORDER BY created_at ASC
	`)
	rows, err := r.db.QueryContext(ctx, query, contentTypeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fieldsList []ContentField
	for rows.Next() {
		var f ContentField
		var ft string
		var cfgRaw []byte
		if err := rows.Scan(&f.ID, &f.ContentTypeID, &f.Name, &f.FieldKey, &ft, &cfgRaw, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.FieldType = fields.FieldType(ft)
		if len(cfgRaw) > 0 {
			_ = json.Unmarshal(cfgRaw, &f.Configuration)
		}
		fieldsList = append(fieldsList, f)
	}
	return fieldsList, rows.Err()
}

func (r *SQLContentRepository) rawUpdateField(ctx context.Context, f *ContentField) error {
	configJSON, err := json.Marshal(f.Configuration)
	if err != nil {
		return err
	}
	query := r.rebind(`UPDATE content_fields SET name = ?, field_type = ?, configuration = ? WHERE id = ?`)
	result, err := r.db.ExecContext(ctx, query, f.Name, string(f.FieldType), string(configJSON), f.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrFieldNotFound
	}
	return nil
}

func (r *SQLContentRepository) rawDeleteField(ctx context.Context, fieldID string) error {
	query := r.rebind(`DELETE FROM content_fields WHERE id = ?`)
	res, err := r.db.ExecContext(ctx, query, fieldID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrFieldNotFound
	}
	return nil
}

func (r *SQLContentRepository) scanContentType(ctx context.Context, row *sql.Row) (*ContentType, error) {
	var ct ContentType
	var kind string
	var settingsRaw []byte
	if err := row.Scan(&ct.ID, &ct.UID, &ct.Name, &kind, &settingsRaw, &ct.CreatedAt, &ct.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrContentTypeNotFound
		}
		return nil, err
	}
	ct.Kind = ContentKind(kind)
	if len(settingsRaw) > 0 {
		_ = json.Unmarshal(settingsRaw, &ct.Settings)
	}
	fieldsList, err := r.ListFields(ctx, ct.ID)
	if err != nil {
		return nil, err
	}
	ct.Fields = fieldsList
	return &ct, nil
}

// --- ContentEntryRepository ---

func (r *SQLContentRepository) rawCreateEntry(ctx context.Context, e *ContentEntry) error {
	draftJSON, err := json.Marshal(e.DraftData)
	if err != nil {
		return err
	}

	var pubJSON []byte
	if e.PublishedData != nil {
		pubJSON, err = json.Marshal(e.PublishedData)
		if err != nil {
			return err
		}
	}

	query := r.rebind(`
		INSERT INTO content_entries (
			id, content_type_id, slug, draft_data, published_data, status,
			draft_version, published_version, singleton_key, created_by, updated_by,
			published_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)

	var singletonParam any
	if e.SingletonKey != nil {
		singletonParam = *e.SingletonKey
	}

	var pubJSONParam any
	if pubJSON != nil {
		pubJSONParam = string(pubJSON)
	}

	_, err = r.db.ExecContext(ctx, query,
		e.ID, e.ContentTypeID, e.Slug, string(draftJSON), pubJSONParam, string(e.Status),
		e.DraftVersion, e.PublishedVersion, singletonParam, e.CreatedBy, e.UpdatedBy,
		e.PublishedAt, e.CreatedAt, e.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "singleton") {
			return ErrSingletonExists
		}
		return fmt.Errorf("failed to create entry: %w", err)
	}
	return nil
}

func (r *SQLContentRepository) GetEntryByID(ctx context.Context, id string) (*ContentEntry, error) {
	query := r.rebind(`
		SELECT id, content_type_id, slug, draft_data, published_data, status,
		       draft_version, published_version, singleton_key, created_by, updated_by,
		       published_at, created_at, updated_at, deleted_at
		FROM content_entries
		WHERE id = ? AND deleted_at IS NULL
	`)
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanEntry(row)
}

func (r *SQLContentRepository) GetEntryBySlug(ctx context.Context, contentTypeID, slug string) (*ContentEntry, error) {
	query := r.rebind(`
		SELECT id, content_type_id, slug, draft_data, published_data, status,
		       draft_version, published_version, singleton_key, created_by, updated_by,
		       published_at, created_at, updated_at, deleted_at
		FROM content_entries
		WHERE content_type_id = ? AND slug = ? AND deleted_at IS NULL
	`)
	row := r.db.QueryRowContext(ctx, query, contentTypeID, slug)
	return r.scanEntry(row)
}

func (r *SQLContentRepository) GetSingleEntry(ctx context.Context, contentTypeID string) (*ContentEntry, error) {
	query := r.rebind(`
		SELECT id, content_type_id, slug, draft_data, published_data, status,
		       draft_version, published_version, singleton_key, created_by, updated_by,
		       published_at, created_at, updated_at, deleted_at
		FROM content_entries
		WHERE content_type_id = ? AND deleted_at IS NULL
		LIMIT 1
	`)
	row := r.db.QueryRowContext(ctx, query, contentTypeID)
	return r.scanEntry(row)
}

func (r *SQLContentRepository) rawUpdateEntry(ctx context.Context, e *ContentEntry, expectedDraftVersion int) error {
	draftJSON, err := json.Marshal(e.DraftData)
	if err != nil {
		return err
	}
	var published any
	if e.PublishedData != nil {
		data, err := json.Marshal(e.PublishedData)
		if err != nil {
			return err
		}
		published = string(data)
	}
	result, err := r.db.ExecContext(ctx, r.rebind(`UPDATE content_entries SET slug = ?, draft_data = ?, published_data = ?, status = ?, draft_version = ?, published_version = ?, published_at = ?, updated_by = ?, updated_at = ? WHERE id = ? AND draft_version = ? AND deleted_at IS NULL`),
		e.Slug, string(draftJSON), published, e.Status, e.DraftVersion, e.PublishedVersion, e.PublishedAt, e.UpdatedBy, e.UpdatedAt, e.ID, expectedDraftVersion)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrVersionMismatch
	}
	return nil
}

func (r *SQLContentRepository) rawPublishEntry(ctx context.Context, id string, publishedData map[string]any, publishedVersion int, publishedAt time.Time, updatedBy string) error {
	pubJSON, err := json.Marshal(publishedData)
	if err != nil {
		return err
	}
	query := r.rebind(`
		UPDATE content_entries
		SET published_data = ?, status = 'published', published_version = ?,
		    published_at = ?, updated_by = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`)
	res, err := r.db.ExecContext(ctx, query, string(pubJSON), publishedVersion, publishedAt, updatedBy, publishedAt, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrEntryNotFound
	}
	return nil
}

func (r *SQLContentRepository) rawUnpublishEntry(ctx context.Context, id string, updatedBy string) error {
	now := time.Now().UTC()
	query := r.rebind(`
		UPDATE content_entries
		SET published_data = NULL, status = 'draft', published_at = NULL,
		    updated_by = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`)
	res, err := r.db.ExecContext(ctx, query, updatedBy, now, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrEntryNotFound
	}
	return nil
}

func (r *SQLContentRepository) rawDeleteEntry(ctx context.Context, id string, hardDelete bool) error {
	query := "DELETE FROM content_entries WHERE id = ?"
	args := []any{id}
	if !hardDelete {
		query = "UPDATE content_entries SET deleted_at = ?, singleton_key = NULL WHERE id = ? AND deleted_at IS NULL"
		args = []any{time.Now().UTC(), id}
	}
	result, err := r.db.ExecContext(ctx, r.rebind(query), args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrEntryNotFound
	}
	return nil
}

func (r *SQLContentRepository) ListEntries(ctx context.Context, contentTypeID string, filter EntryFilter) ([]*ContentEntry, int, error) {
	whereClauses := []string{"content_type_id = ?", "deleted_at IS NULL"}
	args := []any{contentTypeID}

	if filter.Status != "" {
		whereClauses = append(whereClauses, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Search != "" {
		jsonText := "CAST(draft_data AS CHAR)"
		if r.driver == "postgres" {
			jsonText = "CAST(draft_data AS TEXT)"
		}
		// ! is an explicit escape so search treats %, _ and backslashes literally.
		pattern := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(filter.Search)
		whereClauses = append(whereClauses, "(LOWER(slug) LIKE LOWER(?) ESCAPE '!' OR LOWER("+jsonText+") LIKE LOWER(?) ESCAPE '!')")
		args = append(args, "%"+pattern+"%", "%"+pattern+"%")
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countQuery := r.rebind(fmt.Sprintf("SELECT COUNT(*) FROM content_entries WHERE %s", whereSQL))
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortBy := "created_at"
	if filter.SortBy != "" {
		allowed := map[string]bool{"created_at": true, "updated_at": true, "slug": true, "status": true}
		if allowed[filter.SortBy] {
			sortBy = filter.SortBy
		}
	}
	sortOrder := "DESC"
	if strings.ToUpper(filter.SortOrder) == "ASC" {
		sortOrder = "ASC"
	}

	limit := 20
	if filter.Limit > 0 {
		limit = filter.Limit
	}
	offset := 0
	if filter.Offset > 0 {
		offset = filter.Offset
	}

	querySQL := fmt.Sprintf(`
		SELECT id, content_type_id, slug, draft_data, published_data, status,
		       draft_version, published_version, singleton_key, created_by, updated_by,
		       published_at, created_at, updated_at, deleted_at
		FROM content_entries
		WHERE %s
		ORDER BY %s %s, id ASC
		LIMIT ? OFFSET ?
	`, whereSQL, sortBy, sortOrder)

	args = append(args, limit, offset)
	query := r.rebind(querySQL)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []*ContentEntry
	for rows.Next() {
		entry, err := r.scanEntryRows(rows)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}

	return entries, total, rows.Err()
}

func (r *SQLContentRepository) scanEntry(row *sql.Row) (*ContentEntry, error) {
	var e ContentEntry
	var status string
	var slug, sKey, cBy, uBy sql.NullString
	var draftRaw []byte
	var pubRaw []byte
	var pubAt, delAt sql.NullTime

	if err := row.Scan(
		&e.ID, &e.ContentTypeID, &slug, &draftRaw, &pubRaw, &status,
		&e.DraftVersion, &e.PublishedVersion, &sKey, &cBy, &uBy,
		&pubAt, &e.CreatedAt, &e.UpdatedAt, &delAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrEntryNotFound
		}
		return nil, err
	}

	e.Status = ContentStatus(status)
	e.Slug = slug.String
	e.CreatedBy = cBy.String
	e.UpdatedBy = uBy.String
	if sKey.Valid {
		e.SingletonKey = &sKey.String
	}
	if pubAt.Valid {
		e.PublishedAt = &pubAt.Time
	}
	if delAt.Valid {
		e.DeletedAt = &delAt.Time
	}

	if len(draftRaw) > 0 {
		_ = json.Unmarshal(draftRaw, &e.DraftData)
	}
	if len(pubRaw) > 0 {
		_ = json.Unmarshal(pubRaw, &e.PublishedData)
	}

	return &e, nil
}

func (r *SQLContentRepository) scanEntryRows(rows *sql.Rows) (*ContentEntry, error) {
	var e ContentEntry
	var status string
	var slug, sKey, cBy, uBy sql.NullString
	var draftRaw []byte
	var pubRaw []byte
	var pubAt, delAt sql.NullTime

	if err := rows.Scan(
		&e.ID, &e.ContentTypeID, &slug, &draftRaw, &pubRaw, &status,
		&e.DraftVersion, &e.PublishedVersion, &sKey, &cBy, &uBy,
		&pubAt, &e.CreatedAt, &e.UpdatedAt, &delAt,
	); err != nil {
		return nil, err
	}

	e.Status = ContentStatus(status)
	e.Slug = slug.String
	e.CreatedBy = cBy.String
	e.UpdatedBy = uBy.String
	if sKey.Valid {
		e.SingletonKey = &sKey.String
	}
	if pubAt.Valid {
		e.PublishedAt = &pubAt.Time
	}
	if delAt.Valid {
		e.DeletedAt = &delAt.Time
	}

	if len(draftRaw) > 0 {
		_ = json.Unmarshal(draftRaw, &e.DraftData)
	}
	if len(pubRaw) > 0 {
		_ = json.Unmarshal(pubRaw, &e.PublishedData)
	}

	return &e, nil
}

// --- ContentVersionRepository ---

func (r *SQLContentRepository) rawCreateVersion(ctx context.Context, v *ContentVersion) error {
	snapshotJSON, err := json.Marshal(v.Snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal version snapshot: %w", err)
	}

	query := r.rebind(`
		INSERT INTO content_versions (id, entry_id, version_number, snapshot, action, changed_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	_, err = r.db.ExecContext(ctx, query, v.ID, v.EntryID, v.VersionNumber, string(snapshotJSON), v.Action, v.ChangedBy, v.CreatedAt)
	return err
}

func (r *SQLContentRepository) ListVersions(ctx context.Context, entryID string) ([]*ContentVersion, error) {
	query := r.rebind(`
		SELECT id, entry_id, version_number, snapshot, action, changed_by, created_at
		FROM content_versions
		WHERE entry_id = ?
		ORDER BY version_number DESC, created_at DESC, id DESC
	`)
	rows, err := r.db.QueryContext(ctx, query, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*ContentVersion
	for rows.Next() {
		var v ContentVersion
		var snapshotRaw []byte
		var changedBy sql.NullString
		if err := rows.Scan(&v.ID, &v.EntryID, &v.VersionNumber, &snapshotRaw, &v.Action, &changedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.ChangedBy = changedBy.String
		if len(snapshotRaw) > 0 {
			_ = json.Unmarshal(snapshotRaw, &v.Snapshot)
		}
		versions = append(versions, &v)
	}
	return versions, rows.Err()
}

func (r *SQLContentRepository) GetVersion(ctx context.Context, entryID string, versionNumber int) (*ContentVersion, error) {
	query := r.rebind(`
		SELECT id, entry_id, version_number, snapshot, action, changed_by, created_at
		FROM content_versions
		WHERE entry_id = ? AND version_number = ?
 ORDER BY created_at DESC, id DESC LIMIT 1
	`)
	row := r.db.QueryRowContext(ctx, query, entryID, versionNumber)
	var v ContentVersion
	var snapshotRaw []byte
	var changedBy sql.NullString
	if err := row.Scan(&v.ID, &v.EntryID, &v.VersionNumber, &snapshotRaw, &v.Action, &changedBy, &v.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrVersionNotFound
		}
		return nil, err
	}
	v.ChangedBy = changedBy.String
	if len(snapshotRaw) > 0 {
		_ = json.Unmarshal(snapshotRaw, &v.Snapshot)
	}
	return &v, nil
}

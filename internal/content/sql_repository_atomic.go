package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-cms/cms/internal/database/interfaces"
)

type sqlContentRunner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *SQLContentRepository) txRepo(tx *sql.Tx) *SQLContentRepository {
	return &SQLContentRepository{db: tx, root: r.root, driver: r.driver}
}

func (r *SQLContentRepository) CreateType(ctx context.Context, ct *ContentType) error {
	return r.rawCreateType(ctx, ct)
}
func (r *SQLContentRepository) UpdateType(ctx context.Context, ct *ContentType) error {
	return r.rawUpdateType(ctx, ct)
}
func (r *SQLContentRepository) DeleteType(ctx context.Context, id string) error {
	return r.rawDeleteType(ctx, id)
}
func (r *SQLContentRepository) UpdateField(ctx context.Context, f *ContentField) error {
	return r.rawUpdateField(ctx, f)
}
func (r *SQLContentRepository) DeleteField(ctx context.Context, id string) error {
	return r.rawDeleteField(ctx, id)
}
func (r *SQLContentRepository) CreateEntry(ctx context.Context, e *ContentEntry) error {
	return r.rawCreateEntry(ctx, e)
}
func (r *SQLContentRepository) UpdateEntry(ctx context.Context, e *ContentEntry, expected int) error {
	return r.rawUpdateEntry(ctx, e, expected)
}
func (r *SQLContentRepository) PublishEntry(ctx context.Context, id string, data map[string]any, version int, at time.Time, by string) error {
	return r.rawPublishEntry(ctx, id, data, version, at, by)
}
func (r *SQLContentRepository) UnpublishEntry(ctx context.Context, id, by string) error {
	return r.rawUnpublishEntry(ctx, id, by)
}
func (r *SQLContentRepository) DeleteEntry(ctx context.Context, id string, hard bool) error {
	return r.rawDeleteEntry(ctx, id, hard)
}
func (r *SQLContentRepository) CreateVersion(ctx context.Context, v *ContentVersion) error {
	return r.rawCreateVersion(ctx, v)
}

func (r *SQLContentRepository) CreateTypeWithFields(ctx context.Context, ct *ContentType) error {
	tx, err := r.root.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tr := r.txRepo(tx)
	if err := tr.rawCreateType(ctx, ct); err != nil {
		return err
	}
	for i := range ct.Fields {
		if err := tr.rawCreateField(ctx, &ct.Fields[i]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *SQLContentRepository) CommitEntry(ctx context.Context, proposed *ContentEntry, expected int, version *ContentVersion) error {
	if proposed == nil || version == nil {
		return fmt.Errorf("invalid entry commit")
	}
	tx, err := r.root.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tr := r.txRepo(tx)
	var locked string
	if err := tr.db.QueryRowContext(ctx, tr.rebind(`SELECT id FROM content_types WHERE id=? FOR UPDATE`), proposed.ContentTypeID).Scan(&locked); err != nil {
		if err == sql.ErrNoRows {
			return ErrContentTypeNotFound
		}
		return err
	}
	current, err := tr.GetEntryByID(ctx, proposed.ID)
	if err != nil && err != ErrEntryNotFound {
		return err
	}
	if err == ErrEntryNotFound {
		current = nil
	}
	if current != nil && current.ContentTypeID != proposed.ContentTypeID {
		return ErrEntryNotFound
	}
	next, history, err := prepareEntryCommit(current, proposed, expected, version)
	if err != nil {
		return err
	}
	fieldsList, err := tr.ListFields(ctx, proposed.ContentTypeID)
	if err != nil {
		return err
	}
	entries, _, err := tr.ListEntries(ctx, proposed.ContentTypeID, EntryFilter{Limit: int(^uint(0) >> 1)})
	if err != nil {
		return err
	}
	if current != nil {
		kept := entries[:0]
		for _, item := range entries {
			if item.ID != current.ID {
				kept = append(kept, item)
			}
		}
		entries = kept
	}
	entries = append(entries, next)
	if err := validatePersistenceUnique(fieldsList, entries); err != nil {
		return err
	}
	if next.SingletonKey != nil {
		for _, item := range entries {
			if item.ID != next.ID && item.DeletedAt == nil && item.ContentTypeID == next.ContentTypeID {
				return ErrSingletonExists
			}
		}
	}
	if current == nil {
		if err := tr.rawCreateEntry(ctx, next); err != nil {
			return err
		}
	} else {
		var query string
		var args []any
		switch history.Action {
		case "update_draft", "restore":
			draft, err := json.Marshal(next.DraftData)
			if err != nil {
				return err
			}
			query = `UPDATE content_entries SET slug=?, draft_data=?, draft_version=?, updated_by=?, updated_at=? WHERE id=? AND draft_version=? AND deleted_at IS NULL`
			args = []any{next.Slug, string(draft), next.DraftVersion, next.UpdatedBy, next.UpdatedAt, next.ID, expected}
		case "publish":
			pub, err := json.Marshal(next.PublishedData)
			if err != nil {
				return err
			}
			query = `UPDATE content_entries SET published_data=?, status=?, published_version=?, published_at=?, updated_by=?, updated_at=? WHERE id=? AND draft_version=? AND deleted_at IS NULL`
			args = []any{string(pub), string(next.Status), next.PublishedVersion, next.PublishedAt, next.UpdatedBy, next.UpdatedAt, next.ID, expected}
		case "unpublish":
			query = `UPDATE content_entries SET published_data=NULL, status=?, published_version=0, published_at=NULL, updated_by=?, updated_at=? WHERE id=? AND draft_version=? AND deleted_at IS NULL`
			args = []any{string(next.Status), next.UpdatedBy, next.UpdatedAt, next.ID, expected}
		default:
			return fmt.Errorf("unsupported entry mutation %q", history.Action)
		}
		result, err := tr.db.ExecContext(ctx, tr.rebind(query), args...)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrVersionMismatch
		}
	}
	if err := tr.rawCreateVersion(ctx, history); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*proposed = persistenceClone(*next)
	*version = persistenceClone(*history)
	return nil
}

func (r *SQLContentRepository) lockType(ctx context.Context, id string) error {
	var found string
	err := r.db.QueryRowContext(ctx, r.rebind(`SELECT id FROM content_types WHERE id=? FOR UPDATE`), id).Scan(&found)
	if err == sql.ErrNoRows {
		return ErrContentTypeNotFound
	}
	return err
}

func (r *SQLContentRepository) CreateField(ctx context.Context, field *ContentField) error {
	tx, err := r.root.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tr := r.txRepo(tx)
	if err := tr.lockType(ctx, field.ContentTypeID); err != nil {
		return err
	}
	if err := tr.rawCreateField(ctx, field); err != nil {
		return err
	}
	return tx.Commit()
}

var _ ContentTypeRepository = (*SQLContentRepository)(nil)
var _ ContentEntryRepository = (*SQLContentRepository)(nil)
var _ ContentVersionRepository = (*SQLContentRepository)(nil)
var _ sqlContentRunner = (interfaces.SQLDatabase)(nil)

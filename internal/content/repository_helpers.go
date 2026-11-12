package content

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-cms/cms/internal/fields"
)

// Clone all mutable JSON containers, configuration pointers, and timestamps.
// Unlike a JSON round trip this also preserves callers' numeric Go types.
func persistenceClone[T any](value T) T {
	return clonePersistenceValue(reflect.ValueOf(&value)).Elem().Interface().(T)
}

func clonePersistenceValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		if v.Kind() == reflect.Pointer {
			x := reflect.New(v.Type().Elem())
			x.Elem().Set(clonePersistenceValue(v.Elem()))
			return x
		}
		x := reflect.New(v.Type()).Elem()
		x.Set(clonePersistenceValue(v.Elem()))
		return x
	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		x := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			x.SetMapIndex(iter.Key(), clonePersistenceValue(iter.Value()))
		}
		return x
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		x := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			x.Index(i).Set(clonePersistenceValue(v.Index(i)))
		}
		return x
	case reflect.Array, reflect.Struct:
		x := reflect.New(v.Type()).Elem()
		x.Set(v)
		if v.Kind() == reflect.Array {
			for i := 0; i < v.Len(); i++ {
				x.Index(i).Set(clonePersistenceValue(v.Index(i)))
			}
		} else {
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).IsExported() {
					x.Field(i).Set(clonePersistenceValue(v.Field(i)))
				}
			}
		}
		return x
	default:
		return v
	}
}

// Both draft and published values are reservations owned by the same entry.
// Canonical JSON avoids fmt.Sprint collisions between strings and numbers and
// matches SQL JSON numeric representations after a round trip.
func validatePersistenceUnique(schema []ContentField, entries []*ContentEntry) error {
	for _, f := range schema {
		if f.FieldType != fields.TypeUID && !f.Configuration.Unique {
			continue
		}
		owners := make(map[string]string)
		for _, e := range entries {
			if e.DeletedAt != nil {
				continue
			}
			for _, data := range []map[string]any{e.DraftData, e.PublishedData} {
				value, ok := data[f.FieldKey]
				if !ok || value == nil {
					continue
				}
				if s, ok := value.(string); ok && s == "" {
					continue
				}
				key, err := json.Marshal(value)
				if err != nil {
					return err
				}
				if owner, exists := owners[string(key)]; exists && owner != e.ID {
					return fmt.Errorf("%w: %s", errUniqueField, f.FieldKey)
				}
				owners[string(key)] = e.ID
			}
		}
	}
	return nil
}

func prepareEntryCommit(current, proposed *ContentEntry, expected int, version *ContentVersion) (*ContentEntry, *ContentVersion, error) {
	if proposed == nil || version == nil || version.ID == "" || version.EntryID != proposed.ID {
		return nil, nil, fmt.Errorf("invalid entry history record")
	}
	if _, err := json.Marshal(proposed); err != nil {
		return nil, nil, err
	}
	if _, err := json.Marshal(version); err != nil {
		return nil, nil, err
	}
	next := persistenceClone(proposed)
	if version.Action == "create_draft" {
		if expected != 0 || current != nil {
			return nil, nil, ErrVersionMismatch
		}
		next.DraftVersion = 1
	} else {
		if current == nil || current.DeletedAt != nil {
			return nil, nil, ErrEntryNotFound
		}
		if expected <= 0 || current.DraftVersion != expected {
			return nil, nil, ErrVersionMismatch
		}
		if proposed.ContentTypeID != current.ContentTypeID {
			return nil, nil, fmt.Errorf("entry content type cannot change")
		}
		next = persistenceClone(current)
		next.UpdatedBy, next.UpdatedAt = proposed.UpdatedBy, proposed.UpdatedAt
		switch version.Action {
		case "update_draft", "restore":
			next.DraftData, next.Slug = persistenceClone(proposed.DraftData), proposed.Slug
			next.DraftVersion = expected + 1
		case "publish":
			// The committed snapshot must be the draft protected by OCC.
			next.PublishedData = persistenceClone(current.DraftData)
			next.Status, next.PublishedVersion = StatusPublished, expected
			next.PublishedAt = persistenceClone(proposed.PublishedAt)
			if next.PublishedAt == nil {
				next.PublishedAt = persistenceClone(&proposed.UpdatedAt)
			}
		case "unpublish":
			next.PublishedData, next.PublishedAt = nil, nil
			next.Status, next.PublishedVersion = StatusDraft, 0
		default:
			return nil, nil, fmt.Errorf("unsupported entry action %q", version.Action)
		}
	}
	history := persistenceClone(version)
	history.VersionNumber = next.DraftVersion
	history.Snapshot = persistenceClone(next.DraftData)
	return next, history, nil
}

func persistenceVersionLess(a, b *ContentVersion) bool {
	if a.VersionNumber != b.VersionNumber {
		return a.VersionNumber > b.VersionNumber
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID > b.ID
}

func sortPersistenceEntries(entries []*ContentEntry, filter EntryFilter) {
	asc := strings.EqualFold(filter.SortOrder, "asc")
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		var cmp int
		switch filter.SortBy {
		case "slug":
			cmp = strings.Compare(a.Slug, b.Slug)
		case "status":
			cmp = strings.Compare(string(a.Status), string(b.Status))
		case "updated_at":
			cmp = a.UpdatedAt.Compare(b.UpdatedAt)
		default:
			cmp = a.CreatedAt.Compare(b.CreatedAt)
		}
		if cmp == 0 {
			return a.ID < b.ID
		}
		if asc {
			return cmp < 0
		}
		return cmp > 0
	})
}

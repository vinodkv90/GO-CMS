package content

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-cms/cms/internal/fields"
)

const MaxComponentDepth = 8

var validComponentUID = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*\.[a-z0-9]+(?:-[a-z0-9]+)*$`)
var validFieldKey = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

var (
	ErrComponentEntries = errors.New("components cannot have standalone entries")
	ErrReferenced       = errors.New("resource is still referenced")
	ErrFieldForbidden   = errors.New("field access forbidden")
)

func fieldDefinition(f ContentField) fields.FieldDefinition {
	return fields.FieldDefinition{Name: f.FieldKey, Label: f.Name, Type: f.FieldType, Rules: f.Configuration}
}

func componentReferences(f ContentField) []string {
	switch f.FieldType {
	case fields.TypeComponent, fields.TypeRepeatableComponent:
		return []string{f.Configuration.Component}
	case fields.TypeDynamicZone:
		return f.Configuration.Components
	}
	return nil
}

// validateSchema overlays the proposed schema so self references and edits are
// checked before any repository mutation. All roots are checked because an edit
// can make an existing parent exceed the nesting limit.
func (s *Service) validateSchema(ctx context.Context, proposed *ContentType) error {
	types, err := s.typeRepo.ListTypes(ctx)
	if err != nil {
		return err
	}
	byUID := make(map[string]*ContentType, len(types)+1)
	for _, ct := range types {
		byUID[ct.UID] = ct
	}
	byUID[proposed.UID] = proposed
	seen := map[string]bool{}
	for _, f := range proposed.Fields {
		if !validFieldKey.MatchString(f.FieldKey) || strings.TrimSpace(f.Name) == "" {
			return fmt.Errorf("field %q requires a name and a lower-case identifier key", f.FieldKey)
		}
		if seen[f.FieldKey] {
			return ErrFieldExists
		}
		seen[f.FieldKey] = true
		if !s.fieldReg.IsValid(f.FieldType) {
			return wrapFieldError(f.FieldKey, fmt.Errorf("unsupported field type: %s", f.FieldType))
		}
		if err := s.fieldReg.ValidateFieldConfiguration(fieldDefinition(f)); err != nil {
			return wrapFieldError(f.FieldKey, err)
		}
		if err := validateFieldConfiguration(f, proposed.Fields); err != nil {
			return err
		}
	}
	for _, ct := range byUID {
		for _, f := range ct.Fields {
			if f.FieldType == fields.TypeSingleRelation || f.FieldType == fields.TypeMultipleRelation {
				target := byUID[f.Configuration.TargetType]
				if target == nil || target.Kind == KindComponent {
					return fmt.Errorf("relation %s.%s requires a non-component target content type", ct.UID, f.FieldKey)
				}
			}
			for _, uid := range componentReferences(f) {
				target := byUID[uid]
				if target == nil || target.Kind != KindComponent {
					return fmt.Errorf("field %s.%s references unknown component %q", ct.UID, f.FieldKey, uid)
				}
			}
		}
	}
	var visit func(string, int, map[string]bool) error
	visit = func(uid string, depth int, stack map[string]bool) error {
		if depth > MaxComponentDepth {
			return fmt.Errorf("component nesting exceeds %d", MaxComponentDepth)
		}
		if stack[uid] {
			return fmt.Errorf("component cycle involving %q", uid)
		}
		stack[uid] = true
		defer delete(stack, uid)
		for _, f := range byUID[uid].Fields {
			for _, child := range componentReferences(f) {
				if err := visit(child, depth+1, stack); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for uid := range byUID {
		if err := visit(uid, 0, map[string]bool{}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureTypeUnreferenced(ctx context.Context, ct *ContentType) error {
	types, err := s.typeRepo.ListTypes(ctx)
	if err != nil {
		return err
	}
	for _, other := range types {
		if other.ID == ct.ID {
			continue
		}
		for _, f := range other.Fields {
			refs := componentReferences(f)
			if f.FieldType == fields.TypeSingleRelation || f.FieldType == fields.TypeMultipleRelation {
				refs = append(refs, f.Configuration.TargetType)
			}
			for _, uid := range refs {
				if uid == ct.UID {
					return fmt.Errorf("%w: %s.%s", ErrReferenced, other.UID, f.FieldKey)
				}
			}
		}
	}
	return nil
}

func (s *Service) entryType(ctx context.Context, uid string) (*ContentType, error) {
	ct, err := s.typeRepo.GetTypeByUID(ctx, uid)
	if err != nil {
		return nil, err
	}
	if ct.Kind == KindComponent {
		return nil, ErrComponentEntries
	}
	return ct, nil
}

func (s *Service) ownedEntry(ctx context.Context, uid, id string) (*ContentType, *ContentEntry, error) {
	ct, err := s.entryType(ctx, uid)
	if err != nil {
		return nil, nil, err
	}
	entry, err := s.entryRepo.GetEntryByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if entry.ContentTypeID != ct.ID {
		return nil, nil, ErrEntryNotFound
	}
	return ct, entry, nil
}

func (s *Service) createTypeWithFields(ctx context.Context, ct *ContentType) error {
	if repo, ok := s.typeRepo.(AtomicContentTypeRepository); ok {
		return repo.CreateTypeWithFields(ctx, ct)
	}
	// Compatibility for custom repositories: validation has already completed.
	if err := s.typeRepo.CreateType(ctx, ct); err != nil {
		return err
	}
	for i := range ct.Fields {
		if err := s.typeRepo.CreateField(ctx, &ct.Fields[i]); err != nil {
			if rollbackErr := s.typeRepo.DeleteType(ctx, ct.ID); rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}
	}
	return nil
}

func (s *Service) commitEntry(ctx context.Context, entry *ContentEntry, expected int, version *ContentVersion) error {
	if repo, ok := s.entryRepo.(AtomicContentEntryRepository); ok {
		return repo.CommitEntry(ctx, entry, expected, version)
	}
	var err error
	switch version.Action {
	case "create_draft":
		err = s.entryRepo.CreateEntry(ctx, entry)
	case "publish":
		err = s.entryRepo.PublishEntry(ctx, entry.ID, entry.PublishedData, entry.PublishedVersion, *entry.PublishedAt, entry.UpdatedBy)
	case "unpublish":
		err = s.entryRepo.UnpublishEntry(ctx, entry.ID, entry.UpdatedBy)
	default:
		err = s.entryRepo.UpdateEntry(ctx, entry, expected)
	}
	if err != nil {
		return err
	}
	version.VersionNumber = entry.DraftVersion
	return s.versionRepo.CreateVersion(ctx, version)
}

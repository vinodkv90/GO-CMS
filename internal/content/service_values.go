package content

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/fields"
)

// SetMediaLookup connects field reference validation to the media service. The
// callback must fail for missing/inaccessible assets and return their MIME type.
// Configure once during startup before serving requests.
func (s *Service) SetMediaLookup(lookup func(context.Context, string) (string, error)) {
	s.mediaLookup = lookup
}

func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(v))
		for key, item := range v {
			copy[key] = cloneValue(item)
		}
		return copy
	case []any:
		copy := make([]any, len(v))
		for i, item := range v {
			copy[i] = cloneValue(item)
		}
		return copy
	case []string:
		return append([]string(nil), v...)
	default:
		return v
	}
}

func cloneData(data map[string]any) map[string]any {
	if data == nil {
		return map[string]any{}
	}
	return cloneValue(data).(map[string]any)
}

func hasFieldRole(ctx context.Context, roles []string) bool {
	if len(roles) == 0 {
		return true
	}
	user := auth.GetUserFromContext(ctx)
	return auth.HasAnyRole(user, auth.RoleSuperAdmin) || auth.HasAnyRole(user, roles...)
}

// prepareData retains omitted values and legacy unknown keys. Newly submitted
// unknown keys fail explicitly; a schema edit never silently erases stored data.
// trusted means input came from storage, so secret hashes must not be rehashed.
func (s *Service) prepareData(ctx context.Context, ct *ContentType, input, previous map[string]any, depth int, path string, trusted, checkEdits bool) (map[string]any, error) {
	if depth > MaxComponentDepth {
		return nil, fmt.Errorf("%s: component nesting exceeds %d", path, MaxComponentDepth)
	}
	result := cloneData(previous)
	definitions := make(map[string]ContentField, len(ct.Fields))
	for _, f := range ct.Fields {
		definitions[f.FieldKey] = f
	}
	for key, value := range input {
		if _, known := definitions[key]; !known {
			if key == "__component" && ct.Kind == KindComponent {
				result[key] = cloneValue(value)
				continue
			}
			old, existed := previous[key]
			if !trusted && (!existed || !reflect.DeepEqual(value, old)) {
				return nil, fields.FieldError{Field: path + key, Message: "unknown field"}
			}
		}
		result[key] = cloneValue(value)
	}
	for _, f := range ct.Fields {
		key := f.FieldKey
		value, supplied := input[key]
		old, existed := previous[key]
		if supplied && checkEdits && !reflect.DeepEqual(value, old) {
			if f.Configuration.ReadOnly || (auth.GetUserFromContext(ctx) != nil && !hasFieldRole(ctx, f.Configuration.EditRoles)) {
				return nil, fmt.Errorf("%w: %s%s", ErrFieldForbidden, path, key)
			}
		}
		if !supplied && !existed && f.Configuration.Default != nil {
			result[key] = cloneValue(f.Configuration.Default)
		}
	}
	applyUIDFields(ct, result)
	for _, f := range ct.Fields {
		key := f.FieldKey
		value := result[key]
		_, supplied := input[key]
		old := previous[key]
		storedSecret := f.FieldType == fields.TypeSecret && (trusted || (!supplied && old != nil))
		if f.FieldType == fields.TypeRichText {
			if text, ok := value.(string); ok {
				value = fields.SanitizeRichText(text)
				result[key] = value
			}
		}
		if !storedSecret {
			if err := s.fieldReg.ValidateField(fieldDefinition(f), value); err != nil {
				return nil, wrapFieldError(path+key, err)
			}
		} else if value != nil {
			if _, ok := value.(string); !ok {
				return nil, fields.FieldError{Field: path + key, Message: "invalid stored secret"}
			}
		}
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && text == "" {
			continue
		}
		switch f.FieldType {
		case fields.TypeSecret:
			if !storedSecret {
				text, ok := value.(string)
				if !ok {
					return nil, fields.FieldError{Field: path + key, Message: "expected secret string"}
				}
				hash, err := auth.HashPassword(text, nil)
				if err != nil {
					return nil, err
				}
				result[key] = hash
			}
		case fields.TypeSingleRelation, fields.TypeMultipleRelation:
			if err := s.validateRelationValue(ctx, f, value); err != nil {
				return nil, wrapFieldError(path+key, err)
			}
		case fields.TypeMedia:
			if err := s.validateMedia(ctx, f, value); err != nil {
				return nil, wrapFieldError(path+key, err)
			}
		case fields.TypeComponent, fields.TypeRepeatableComponent, fields.TypeDynamicZone:
			repeat := f.FieldType != fields.TypeComponent || f.Configuration.Repeatable
			items := []any{value}
			oldItems := []any{old}
			if repeat {
				var ok bool
				items, ok = value.([]any)
				if !ok {
					return nil, fields.FieldError{Field: path + key, Message: "expected component array"}
				}
				oldItems, _ = old.([]any)
			}
			for i, item := range items {
				object, ok := item.(map[string]any)
				if !ok {
					return nil, fields.FieldError{Field: path + key, Message: "expected component object"}
				}
				uid := f.Configuration.Component
				if f.FieldType == fields.TypeDynamicZone {
					uid, _ = object["__component"].(string)
					allowed := false
					for _, candidate := range f.Configuration.Components {
						if candidate == uid {
							allowed = true
						}
					}
					if !allowed {
						return nil, fields.FieldError{Field: path + key, Message: "component is not allowed in this dynamic zone"}
					}
				} else if marker, exists := object["__component"]; exists && marker != uid {
					return nil, fields.FieldError{Field: path + key, Message: "component discriminator does not match schema"}
				}
				component, err := s.typeRepo.GetTypeByUID(ctx, uid)
				if err != nil || component.Kind != KindComponent {
					return nil, fmt.Errorf("%s%s: component %q not found", path, key, uid)
				}
				var oldObject map[string]any
				if i < len(oldItems) {
					oldObject, _ = oldItems[i].(map[string]any)
				}
				if f.FieldType == fields.TypeDynamicZone && oldObject["__component"] != uid {
					oldObject = nil
				}
				childPath := path + key + "."
				if repeat {
					childPath = fmt.Sprintf("%s%s[%d].", path, key, i)
				}
				// Omitted component values are stored data, not fresh secret input.
				prepared, err := s.prepareData(ctx, component, object, oldObject, depth+1, childPath, trusted || !supplied, checkEdits && supplied)
				if err != nil {
					return nil, err
				}
				items[i] = prepared
			}
			if repeat {
				result[key] = items
			} else {
				result[key] = items[0]
			}
		}
	}
	return result, nil
}

func referenceIDs(value any, multiple bool) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	if !multiple {
		id, ok := value.(string)
		if !ok || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("expected a non-empty reference ID")
		}
		return []string{id}, nil
	}
	var ids []string
	switch list := value.(type) {
	case []string:
		ids = append(ids, list...)
	case []any:
		for _, value := range list {
			id, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("each reference ID must be a string")
			}
			ids = append(ids, id)
		}
	default:
		return nil, fmt.Errorf("expected reference ID array")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || seen[id] {
			return nil, fmt.Errorf("reference IDs must be non-empty and distinct")
		}
		seen[id] = true
	}
	return ids, nil
}

func (s *Service) validateMedia(ctx context.Context, f ContentField, value any) error {
	ids, err := referenceIDs(value, f.Configuration.Multiple)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if s.mediaLookup == nil {
		return fmt.Errorf("media validation is unavailable")
	}
	for _, id := range ids {
		mime, err := s.mediaLookup(ctx, id)
		if err != nil {
			return fmt.Errorf("invalid media asset %q", id)
		}
		if !mediaMIMEAllowed(mime, f.Configuration.AllowedTypes) {
			return fmt.Errorf("media asset %q has disallowed MIME type", id)
		}
	}
	return nil
}

func isVideoMediaMIME(mime string) bool {
	return strings.HasPrefix(mime, "video/") ||
		mime == "application/vnd.apple.mpegurl" ||
		mime == "application/x-mpegurl"
}

func mediaMIMEAllowed(rawMIME string, allowedTypes []string) bool {
	if len(allowedTypes) == 0 {
		return true
	}
	mime := strings.ToLower(strings.TrimSpace(rawMIME))
	for _, rawKind := range allowedTypes {
		kind := strings.ToLower(strings.TrimSpace(rawKind))
		switch kind {
		case "":
			return true
		case "image", "images":
			if strings.HasPrefix(mime, "image/") {
				return true
			}
		case "video", "videos":
			if isVideoMediaMIME(mime) {
				return true
			}
		case "audio", "audios":
			if strings.HasPrefix(mime, "audio/") {
				return true
			}
		case "file", "files":
			if !strings.HasPrefix(mime, "image/") && !isVideoMediaMIME(mime) && !strings.HasPrefix(mime, "audio/") {
				return true
			}
		default:
			if kind == mime ||
				(strings.HasSuffix(kind, "/*") && strings.HasPrefix(mime, strings.TrimSuffix(kind, "*"))) ||
				(strings.HasSuffix(kind, "/") && strings.HasPrefix(mime, kind)) {
				return true
			}
		}
	}
	return false
}

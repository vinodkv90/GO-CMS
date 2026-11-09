package pluginsdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

var validPluginIDRegex = regexp.MustCompile(`^[a-z0-9]+(?:[.\-_][a-z0-9]+)*$`)

var (
	ErrInvalidManifest   = errors.New("invalid plugin manifest")
	ErrMissingPluginID   = errors.New("plugin manifest missing required 'id'")
	ErrMissingPluginName = errors.New("plugin manifest missing required 'name'")
)

// RouteDef describes an HTTP endpoint exposed by a plugin.
type RouteDef struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Permission string `json:"permission,omitempty"`
}

// Manifest defines the metadata, capabilities, permissions, and configuration schema of a plugin.
type Manifest struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Version           string         `json:"version"`
	Description       string         `json:"description"`
	Author            string         `json:"author"`
	MinimumCMSVersion string         `json:"minimumCmsVersion"`
	Runtime           string         `json:"runtime"` // "go-native", "wasm", "process"
	Entrypoint        string         `json:"entrypoint,omitempty"`
	Permissions       []string       `json:"permissions"`
	SettingsSchema    map[string]any `json:"settingsSchema,omitempty"`
	Routes            []RouteDef     `json:"routes,omitempty"`
}

// Validate checks whether the manifest satisfies CMS plugin specification rules.
func (m *Manifest) Validate() error {
	if m.ID == "" {
		return ErrMissingPluginID
	}
	if !validPluginIDRegex.MatchString(m.ID) {
		return fmt.Errorf("%w: plugin id must be lowercase alphanumeric with dots, hyphens, or underscores (e.g. 'com.cms.audit' or 'audit-log')", ErrInvalidManifest)
	}
	if m.Name == "" {
		return ErrMissingPluginName
	}
	if m.Version == "" {
		m.Version = "1.0.0"
	}
	if m.Runtime == "" {
		m.Runtime = "go-native"
	}
	return nil
}

// ParseManifest parses and validates a plugin manifest JSON string.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: JSON parse failure: %w", ErrInvalidManifest, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

package themes

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrThemeNotFound      = errors.New("theme not found")
	ErrInvalidManifest    = errors.New("invalid theme.json manifest")
	ErrIncompatibleEngine = errors.New("theme requires an incompatible template engine")
)

// Manifest defines the metadata of a theme.
type Manifest struct {
	Name              string         `json:"name"`
	Slug              string         `json:"slug"`
	Version           string         `json:"version"`
	Description       string         `json:"description"`
	Author            string         `json:"author"`
	Engine            string         `json:"engine"` // Must be "go-html-template"
	MinimumCMSVersion string         `json:"minimumCmsVersion"`
	Settings          map[string]any `json:"settings,omitempty"`
}

// Theme encapsulates a loaded theme and its filesystem directory.
type Theme struct {
	Manifest Manifest
	RootDir  string
}

// Manager coordinates theme discovery, activation, and loading.
type Manager struct {
	mu          sync.RWMutex
	themesDir   string
	activeTheme string
	themes      map[string]*Theme
	logger      *slog.Logger
}

// NewManager initializes the theme manager and discovers installed themes.
func NewManager(themesDir string, activeTheme string, logger *slog.Logger) (*Manager, error) {
	if logger == nil {
		logger = slog.Default()
	}

	m := &Manager{
		themesDir:   themesDir,
		activeTheme: activeTheme,
		themes:      make(map[string]*Theme),
		logger:      logger,
	}

	if err := m.Scan(); err != nil {
		logger.Warn("Theme scanning warning", "themes_dir", themesDir, "error", err)
	}

	return m, nil
}

// Scan scans the themes directory and loads valid theme manifests.
func (m *Manager) Scan() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(m.themesDir)
	if err != nil {
		if os.IsNotExist(err) {
			_ = os.MkdirAll(m.themesDir, 0755)
			return nil
		}
		return fmt.Errorf("failed to read themes dir %q: %w", m.themesDir, err)
	}

	m.themes = make(map[string]*Theme)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		themeDir := filepath.Join(m.themesDir, entry.Name())
		manifestFile := filepath.Join(themeDir, "theme.json")

		data, err := os.ReadFile(manifestFile)
		if err != nil {
			continue // No theme.json, skip
		}

		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			m.logger.Warn("Failed to parse theme.json", "dir", entry.Name(), "error", err)
			continue
		}

		if manifest.Slug == "" {
			manifest.Slug = entry.Name()
		}

		if manifest.Engine != "" && manifest.Engine != "go-html-template" {
			m.logger.Warn("Skipping theme with incompatible engine", "slug", manifest.Slug, "engine", manifest.Engine)
			continue
		}

		m.themes[manifest.Slug] = &Theme{
			Manifest: manifest,
			RootDir:  themeDir,
		}
	}

	return nil
}

// List returns all discovered themes.
func (m *Manager) List() []*Manifest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []*Manifest
	for _, t := range m.themes {
		clone := t.Manifest
		list = append(list, &clone)
	}
	return list
}

// Get retrieves a theme by slug.
func (m *Manager) Get(slug string) (*Theme, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, ok := m.themes[slug]
	if !ok {
		return nil, ErrThemeNotFound
	}
	return t, nil
}

// GetActive returns the currently active theme.
func (m *Manager) GetActive() (*Theme, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, ok := m.themes[m.activeTheme]
	if !ok {
		return nil, fmt.Errorf("%w: active theme %q is not found", ErrThemeNotFound, m.activeTheme)
	}
	return t, nil
}

// SetActive changes the active theme.
func (m *Manager) SetActive(slug string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.themes[slug]; !ok {
		return ErrThemeNotFound
	}
	m.activeTheme = slug
	m.logger.Info("Active theme changed", "active_theme", slug)
	return nil
}

// ActiveThemeSlug returns the active theme slug string.
func (m *Manager) ActiveThemeSlug() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeTheme
}

// ResolveAssetPath returns the full safe filepath to a theme asset, guarding against path traversal.
func (t *Theme) ResolveAssetPath(assetRelPath string) (string, error) {
	if strings.HasPrefix(assetRelPath, "/") || strings.HasPrefix(assetRelPath, "\\") {
		return "", errors.New("path traversal denied: absolute or leading slash not allowed")
	}

	cleanRel := filepath.Clean(filepath.FromSlash(assetRelPath))
	if strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, string(filepath.Separator)) {
		return "", errors.New("path traversal denied")
	}

	assetsDir := filepath.Join(t.RootDir, "assets")
	fullPath := filepath.Join(assetsDir, cleanRel)

	if !strings.HasPrefix(fullPath, assetsDir) {
		return "", errors.New("unauthorized path access")
	}

	return fullPath, nil
}

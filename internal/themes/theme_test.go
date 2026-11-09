package themes_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-cms/cms/internal/themes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThemeManager_DiscoveryAndActivation(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create theme 1: valid "aurora"
	auroraDir := filepath.Join(tempDir, "aurora")
	require.NoError(t, os.MkdirAll(filepath.Join(auroraDir, "assets"), 0755))
	auroraManifest := themes.Manifest{
		Name:              "Aurora Theme",
		Slug:              "aurora",
		Version:           "1.0.0",
		Description:       "A modern clean theme",
		Author:            "Test Author",
		Engine:            "go-html-template",
		MinimumCMSVersion: "1.0.0",
		Settings: map[string]any{
			"primaryColor": "#ff0000",
		},
	}
	auroraJSON, _ := json.Marshal(auroraManifest)
	require.NoError(t, os.WriteFile(filepath.Join(auroraDir, "theme.json"), auroraJSON, 0644))

	// Create theme 2: incompatible engine "pug"
	incompatDir := filepath.Join(tempDir, "incompat")
	require.NoError(t, os.MkdirAll(incompatDir, 0755))
	incompatManifest := themes.Manifest{
		Name:   "Pug Theme",
		Slug:   "pug-theme",
		Engine: "pug",
	}
	incompatJSON, _ := json.Marshal(incompatManifest)
	require.NoError(t, os.WriteFile(filepath.Join(incompatDir, "theme.json"), incompatJSON, 0644))

	// Create theme 3: directory without theme.json
	noManifestDir := filepath.Join(tempDir, "empty-dir")
	require.NoError(t, os.MkdirAll(noManifestDir, 0755))

	// Initialize manager
	mgr, err := themes.NewManager(tempDir, "aurora", logger)
	require.NoError(t, err)

	// List discovered themes
	list := mgr.List()
	assert.Len(t, list, 1)
	assert.Equal(t, "aurora", list[0].Slug)
	assert.Equal(t, "Aurora Theme", list[0].Name)

	// Get active theme
	active, err := mgr.GetActive()
	require.NoError(t, err)
	assert.Equal(t, "aurora", active.Manifest.Slug)
	assert.Equal(t, "aurora", mgr.ActiveThemeSlug())

	// Get non-existent theme
	_, err = mgr.Get("nonexistent")
	assert.ErrorIs(t, err, themes.ErrThemeNotFound)

	// SetActive to non-existent
	err = mgr.SetActive("nonexistent")
	assert.ErrorIs(t, err, themes.ErrThemeNotFound)
}

func TestTheme_ResolveAssetPath_Security(t *testing.T) {
	tempDir := t.TempDir()
	themeDir := filepath.Join(tempDir, "test-theme")
	assetsDir := filepath.Join(themeDir, "assets")
	cssDir := filepath.Join(assetsDir, "css")
	require.NoError(t, os.MkdirAll(cssDir, 0755))

	// Create sample asset
	styleCSS := filepath.Join(cssDir, "style.css")
	require.NoError(t, os.WriteFile(styleCSS, []byte("body { color: red; }"), 0644))

	theme := &themes.Theme{
		Manifest: themes.Manifest{Slug: "test-theme"},
		RootDir:  themeDir,
	}

	// 1. Valid asset path
	resolved, err := theme.ResolveAssetPath("css/style.css")
	require.NoError(t, err)
	assert.Equal(t, styleCSS, resolved)

	// 2. Path traversal attack: relative ../
	_, err = theme.ResolveAssetPath("../theme.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "traversal denied")

	// 3. Path traversal attack: deeply nested
	_, err = theme.ResolveAssetPath("css/../../../../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "traversal denied")

	// 4. Absolute path
	_, err = theme.ResolveAssetPath("/etc/passwd")
	assert.Error(t, err)
}

package templates

import (
	"bytes"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/themes"
)

func BenchmarkRenderer_Render(b *testing.B) {
	cfg := config.DefaultConfig()
	cfg.Themes.Cache = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	themesDir := filepath.Join("..", "..", "themes")
	mgr, err := themes.NewManager(themesDir, "default", logger)
	if err != nil {
		b.Skip("Themes directory not available for benchmark")
	}

	renderer := NewRenderer(cfg, mgr, nil, logger)

	data := &TemplateData{
		Site: SiteInfo{
			Title:   "Benchmark CMS Site",
			Tagline: "High throughput benchmark suite",
			BaseURL: "http://localhost:8080",
		},
		SEO: SEOMetadata{
			Title:       "Home",
			Description: "Fast server rendering",
			Canonical:   "http://localhost:8080/",
		},
		Breadcrumbs: []BreadcrumbItem{{Title: "Home", URL: "/"}},
		Page:        map[string]any{"headline": "Welcome to Go CMS"},
	}

	buf := new(bytes.Buffer)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = renderer.Render(buf, filepath.Join("pages", "home.html"), data)
	}
}

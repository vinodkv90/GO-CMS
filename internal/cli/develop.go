package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-cms/cms/internal/api"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/core/logging"
	"github.com/go-cms/cms/internal/database/factory"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/go-cms/cms/internal/database/migrations"
	"github.com/go-cms/cms/internal/fields"
	"github.com/go-cms/cms/internal/media"
	"github.com/go-cms/cms/internal/plugins"
	"github.com/go-cms/cms/internal/storage"
	"github.com/go-cms/cms/internal/storage/local"
	"github.com/go-cms/cms/internal/templates"
	"github.com/go-cms/cms/internal/themes"
	auditlog "github.com/go-cms/cms/plugins/audit-log"
	seoanalyzer "github.com/go-cms/cms/plugins/seo-analyzer"
)

// RunServer runs the CMS server in development or production mode.
func RunServer(configPath string, isProduction bool) error {
	startTime := time.Now()

	// 1. Load Configuration & .env
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}

	if isProduction {
		cfg.Server.Env = "production"
		cfg.Themes.Cache = true
	} else {
		cfg.Server.Env = "development"
	}

	// 2. Initialize Structured Logger
	logger := logging.NewLogger(cfg.Logging, os.Stdout)

	// 3. Connect to Database
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer dbCancel()

	db, err := factory.Connect(dbCtx, cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// 4. Auto-run pending database migrations
	runner := migrations.NewRunner(db)
	runner.Register(migrations.DefaultMigrations(db.DriverName())...)
	applied, err := runner.Up(context.Background())
	if err != nil {
		return fmt.Errorf("database migrations failed: %w", err)
	}

	// 5. Initialize Services
	fieldReg := fields.NewRegistry()
	var authSvc *auth.Service
	var contentSvc *content.Service

	if sqlDB, ok := db.(interfaces.SQLDatabase); ok {
		authRepo := auth.NewSQLAuthRepository(sqlDB)
		authSvc = auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, logger)

		// Seed initial roles and admin if fresh DB
		_, _ = authSvc.SeedDefaultRolesAndAdmin(context.Background(), "admin", "admin@cms.local", "Admin@CMS2026!")

		contentRepo := content.NewSQLContentRepository(sqlDB)
		contentSvc = content.NewService(contentRepo, contentRepo, contentRepo, fieldReg, content.NewEventBus(), logger)

		// Seed "pages" content type for Visual Page Builder
		if _, err := contentSvc.GetContentType(context.Background(), "pages"); err != nil {
			_, _ = contentSvc.CreateContentType(context.Background(), content.CreateTypeRequest{
				Name: "Pages",
				UID:  "pages",
				Kind: content.KindCollection,
				Fields: []content.ContentField{
					{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Required: true}},
					{Name: "Slug", FieldKey: "slug", FieldType: fields.TypeShortText},
					{Name: "Template", FieldKey: "template", FieldType: fields.TypeShortText},
					{Name: "Blocks", FieldKey: "blocks", FieldType: fields.TypeJSON},
					{Name: "SEO", FieldKey: "seo", FieldType: fields.TypeJSON},
				},
			})
		}

		// Seed default "home" page entry if missing
		if _, err := contentSvc.GetEntryBySlug(context.Background(), "pages", "home", true); err != nil {
			homeBlocks := []map[string]any{
				{
					"id":   "home-sec-1",
					"type": "core/section",
					"attributes": map[string]any{
						"padding":         "large",
						"backgroundColor": "#0f172a",
						"textColor":       "#ffffff",
					},
					"innerBlocks": []map[string]any{
						{
							"id":   "home-cnt-1",
							"type": "core/container",
							"attributes": map[string]any{
								"maxWidth": "boxed",
							},
							"innerBlocks": []map[string]any{
								{
									"id":   "home-h-1",
									"type": "core/heading",
									"attributes": map[string]any{
										"level":     1,
										"content":   "Build High-Performance Websites with Go CMS",
										"textAlign": "center",
										"textColor": "#38bdf8",
									},
								},
								{
									"id":   "home-p-1",
									"type": "core/paragraph",
									"attributes": map[string]any{
										"content":   "Experience lightning-fast server-side rendering paired with the flexibility of visual Gutenberg block editing.",
										"textAlign": "center",
									},
								},
								{
									"id":   "home-btn-1",
									"type": "core/button",
									"attributes": map[string]any{
										"text":      "Explore Features",
										"url":       "/about-us",
										"variant":   "primary",
										"alignment": "center",
									},
								},
							},
						},
					},
				},
			}
			createdHome, cErr := contentSvc.CreateEntry(context.Background(), "pages", map[string]any{
				"title":    "Home Page",
				"slug":     "home",
				"template": "default",
				"blocks":   homeBlocks,
				"seo": map[string]any{
					"meta_title":       "Home | Go CMS",
					"meta_description": "Welcome to our high-performance website built with Go CMS.",
				},
			}, "system")
			if cErr == nil {
				_, _ = contentSvc.PublishEntry(context.Background(), "pages", createdHome.ID, "system")
			}
		}
	}

	// 6. Media Service
	var mediaSvc *media.Service
	localStorage, errStorage := local.New(cfg.Media.UploadDir, "/uploads")
	if errStorage == nil && db != nil {
		if sqlDB, ok := db.(interfaces.SQLDatabase); ok {
			mediaRepo := media.NewSQLRepository(sqlDB)
			swappableStorage := storage.NewSwappable(localStorage)
			mediaSvc = media.NewService(mediaRepo, swappableStorage, cfg, logger)
			if contentSvc != nil {
				contentSvc.SetMediaLookup(func(ctx context.Context, id string) (string, error) {
					asset, aErr := mediaSvc.GetAsset(ctx, id)
					if aErr != nil {
						return "", aErr
					}
					return asset.MIMEType, nil
				})
			}
		}
	}

	// 7. Themes & Template Renderer
	var renderer *templates.Renderer
	themeMgr, errTheme := themes.NewManager(cfg.Themes.ThemesDir, cfg.Themes.ActiveTheme, logger)
	if errTheme == nil {
		renderer = templates.NewRenderer(cfg, themeMgr, contentSvc, logger)
	}

	// 8. Plugin Manager
	var eventBus *content.EventBus
	if contentSvc != nil {
		eventBus = contentSvc.EventBus()
	}
	pluginMgr := plugins.NewManager(cfg.Plugins.PluginsDir, eventBus, fieldReg, db, logger)
	_ = pluginMgr.Scan()
	_ = pluginMgr.RegisterNative(auditlog.New())
	_ = pluginMgr.RegisterNative(seoanalyzer.New())
	_ = pluginMgr.BootstrapPlugins(context.Background(), "audit-log", "seo-analyzer")

	// 9. Construct and Start API Server
	apiServer := api.NewServer(cfg, db, authSvc, contentSvc, mediaSvc, pluginMgr, renderer, logger)

	elapsed := time.Since(startTime).Milliseconds()

	modeLabel := "Templated CMS (SSR + Gutenberg Page Builder)"
	if _, err := os.Stat("themes"); err != nil {
		modeLabel = "Headless CMS (REST APIs)"
	}

	// Print Next.js / Strapi Style Startup Banner
	fmt.Println()
	fmt.Println(" ┌─────────────────────────────────────────────────────────────┐")
	fmt.Printf(" │                       Go CMS %-31s│\n", Version)
	fmt.Println(" │     Modern High-Performance Content Management System       │")
	fmt.Println(" ├─────────────────────────────────────────────────────────────┤")
	fmt.Printf(" │  ▸ Mode:        %-44s│\n", modeLabel)
	fmt.Printf(" │  ▸ Database:    %-44s│\n", fmt.Sprintf("%s (%s:%d/%s)", cfg.Database.Driver, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database))
	fmt.Printf(" │  ▸ Admin Panel: %-44s│\n", fmt.Sprintf("http://localhost:%d/admin/", cfg.Server.Port))
	fmt.Printf(" │  ▸ Public Site: %-44s│\n", fmt.Sprintf("http://localhost:%d/", cfg.Server.Port))
	fmt.Printf(" │  ▸ REST API:    %-44s│\n", fmt.Sprintf("http://localhost:%d/api/v1", cfg.Server.Port))
	fmt.Printf(" │  ▸ Healthz:     %-44s│\n", fmt.Sprintf("http://localhost:%d/healthz", cfg.Server.Port))
	fmt.Println(" ├─────────────────────────────────────────────────────────────┤")
	fmt.Println(" │  Default Admin: admin / Admin@CMS2026!                      │")
	if len(applied) > 0 {
		fmt.Printf(" │  Migrations:    %d migration(s) applied on startup           │\n", len(applied))
	}
	fmt.Printf(" │  Ready in %dms. Press Ctrl+C to stop.                        │\n", elapsed)
	fmt.Println(" └─────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Start server in background goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := apiServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for OS shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		fmt.Printf("\nShutting down server gracefully (%s)...\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()
		if err := apiServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}
		fmt.Println("Server stopped.")
		return nil
	}
}

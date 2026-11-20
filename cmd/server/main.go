package main

import (
	"context"
	"errors"
	"flag"
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

func main() {
	configPath := flag.String("config", "", "Path to configuration file")
	autoMigrate := flag.Bool("auto-migrate", true, "Automatically run pending database migrations on startup")
	flag.Parse()

	// 1. Load Configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Structured Logger
	logger := logging.NewLogger(cfg.Logging, os.Stdout)
	logger.Info("Initializing CMS platform...",
		"env", cfg.Server.Env,
		"version", api.CMSVersion,
		"db_driver", cfg.Database.Driver,
	)

	// 3. Connect to Database
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer dbCancel()

	db, err := factory.Connect(dbCtx, cfg.Database)
	if err != nil {
		logger.Error("Failed to initialize database adapter", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("Error closing database connection", "error", err)
		}
	}()

	// 4. Run Migrations if enabled
	if *autoMigrate {
		logger.Info("Checking database migrations...")
		runner := migrations.NewRunner(db)
		runner.Register(migrations.DefaultMigrations(db.DriverName())...)

		applied, err := runner.Up(context.Background())
		if err != nil {
			logger.Error("Database migrations failed; refusing to start with an unknown schema", "error", err)
			os.Exit(1)
		} else if len(applied) > 0 {
			logger.Info("Successfully applied database migrations", "count", len(applied), "versions", applied)
		} else {
			logger.Info("Database schema is up to date")
		}
	}

	// 5. Initialize Auth and Content Services & Seed Standard Roles/Admin
	fieldReg := fields.NewRegistry()
	var authSvc *auth.Service
	var contentSvc *content.Service
	if sqlDB, ok := db.(interfaces.SQLDatabase); ok {
		authRepo := auth.NewSQLAuthRepository(sqlDB)
		authSvc = auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, logger)

		// Seed initial roles and admin
		if _, err := authSvc.SeedDefaultRolesAndAdmin(context.Background(), "admin", "admin@cms.local", "Admin@CMS2026!"); err != nil {
			logger.Warn("Failed to seed default roles and admin", "error", err)
		}

		contentRepo := content.NewSQLContentRepository(sqlDB)
		contentSvc = content.NewService(contentRepo, contentRepo, contentRepo, fieldReg, content.NewEventBus(), logger)

		// Seed default "pages" content type for Visual Page Builder if not exists
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
			logger.Info("Default 'pages' content type bootstrapped for Visual Page Builder")
		}

		// Seed default "home" page entry if not exists
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
										"text":      "Explore Articles",
										"url":       "/blog-post",
										"variant":   "primary",
										"alignment": "center",
									},
								},
							},
						},
					},
				},
				{
					"id":   "home-sec-2",
					"type": "core/section",
					"attributes": map[string]any{
						"padding":         "medium",
						"backgroundColor": "#f8fafc",
					},
					"innerBlocks": []map[string]any{
						{
							"id":   "home-dyn-1",
							"type": "core/dynamic-posts",
							"attributes": map[string]any{
								"collection": "blog-post",
								"limit":      6,
								"title":      "Latest Articles & Insights",
							},
						},
					},
				},
			}
			createdHome, err := contentSvc.CreateEntry(context.Background(), "pages", map[string]any{
				"title":    "Home Page",
				"slug":     "home",
				"template": "default",
				"blocks":   homeBlocks,
				"seo": map[string]any{
					"meta_title":       "Home | Go CMS",
					"meta_description": "Welcome to our high-performance website built with Go CMS.",
				},
			}, "system")
			if err == nil {
				_, _ = contentSvc.PublishEntry(context.Background(), "pages", createdHome.ID, "system")
				logger.Info("Default 'home' page bootstrapped and published for Page Builder")
			}
		}
	}

	// Initialize Storage Provider and Media Service
	var mediaSvc *media.Service
	localStorage, errStorage := local.New(cfg.Media.UploadDir, "/uploads")
	if errStorage != nil {
		logger.Warn("Failed to initialize local storage provider", "error", errStorage)
	} else if sqlDB, ok := db.(interfaces.SQLDatabase); ok {
		mediaRepo := media.NewSQLRepository(sqlDB)
		swappableStorage := storage.NewSwappable(localStorage)
		mediaSvc = media.NewService(mediaRepo, swappableStorage, cfg, logger)
		if contentSvc != nil {
			contentSvc.SetMediaLookup(func(ctx context.Context, id string) (string, error) {
				asset, err := mediaSvc.GetAsset(ctx, id)
				if err != nil {
					return "", err
				}
				return asset.MIMEType, nil
			})
		}
		logger.Info("Media library service initialized", "provider", localStorage.Name(), "upload_dir", cfg.Media.UploadDir)
	}

	// Initialize Themes Manager and Template Renderer
	var renderer *templates.Renderer
	themeMgr, errTheme := themes.NewManager(cfg.Themes.ThemesDir, cfg.Themes.ActiveTheme, logger)
	if errTheme != nil {
		logger.Warn("Failed to initialize theme manager", "error", errTheme)
	} else {
		renderer = templates.NewRenderer(cfg, themeMgr, contentSvc, logger)
		logger.Info("Theme manager and template renderer initialized", "active_theme", cfg.Themes.ActiveTheme)
	}

	// Initialize Plugin Manager and Register Reference Plugins
	var eventBus *content.EventBus
	if contentSvc != nil {
		eventBus = contentSvc.EventBus()
	}

	pluginMgr := plugins.NewManager(cfg.Plugins.PluginsDir, eventBus, fieldReg, db, logger)
	if err := pluginMgr.Scan(); err != nil {
		logger.Warn("Failed to scan plugins directory", "error", err)
	}

	// Register built-in reference plugins and reconcile with persisted cms_plugins_state
	_ = pluginMgr.RegisterNative(auditlog.New())
	_ = pluginMgr.RegisterNative(seoanalyzer.New())
	if err := pluginMgr.BootstrapPlugins(context.Background(), "audit-log", "seo-analyzer"); err != nil {
		logger.Warn("Plugin bootstrap completed with warnings", "error", err)
	}
	logger.Info("Plugin architecture initialized and reconciled with database state")

	// 6. Initialize HTTP Server
	server := api.NewServer(cfg, db, authSvc, contentSvc, mediaSvc, pluginMgr, renderer, logger)

	// 6. Graceful Shutdown Signal Handling
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	serverErrChan := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrChan <- err
		}
	}()

	// Block until signal or fatal server error
	select {
	case sig := <-stopChan:
		logger.Info("Received shutdown signal", "signal", sig.String())
	case err := <-serverErrChan:
		logger.Error("Fatal HTTP server error", "error", err)
		os.Exit(1)
	}

	// Execute graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during server shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("CMS platform shut down gracefully.")
}

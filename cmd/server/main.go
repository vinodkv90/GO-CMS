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
	}

	// Initialize Storage Provider and Media Service
	var mediaSvc *media.Service
	localStorage, errStorage := local.New(cfg.Media.UploadDir, "/uploads")
	if errStorage != nil {
		logger.Warn("Failed to initialize local storage provider", "error", errStorage)
	} else if sqlDB, ok := db.(interfaces.SQLDatabase); ok {
		mediaRepo := media.NewSQLRepository(sqlDB)
		mediaSvc = media.NewService(mediaRepo, localStorage, cfg, logger)
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

	// Register built-in reference plugins
	_ = pluginMgr.RegisterNative(auditlog.New())
	_ = pluginMgr.RegisterNative(seoanalyzer.New())
	_ = pluginMgr.Enable(context.Background(), "audit-log")
	_ = pluginMgr.Enable(context.Background(), "seo-analyzer")
	logger.Info("Plugin architecture initialized and reference plugins enabled")

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

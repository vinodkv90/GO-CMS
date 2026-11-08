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
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/core/logging"
	"github.com/go-cms/cms/internal/database/factory"
	"github.com/go-cms/cms/internal/database/migrations"
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
			logger.Warn("Notice: Database migrations encountered an issue (check connection if DB not yet started)", "error", err)
		} else if len(applied) > 0 {
			logger.Info("Successfully applied database migrations", "count", len(applied), "versions", applied)
		} else {
			logger.Info("Database schema is up to date")
		}
	}

	// 5. Initialize HTTP Server
	server := api.NewServer(cfg, db, logger)

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


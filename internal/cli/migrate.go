package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/factory"
	"github.com/go-cms/cms/internal/database/migrations"
)

// RunMigrate executes database migration actions.
func RunMigrate(configPath string, args []string) error {
	action := "up"
	if len(args) > 0 {
		action = strings.ToLower(args[0])
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := factory.Connect(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("database connection error: %w", err)
	}
	defer func() { _ = db.Close() }()

	runner := migrations.NewRunner(db)
	runner.Register(migrations.DefaultMigrations(db.DriverName())...)

	switch action {
	case "up":
		fmt.Printf("Executing database migrations for %s (%s)...\n", cfg.Database.Driver, cfg.Database.Database)
		applied, err := runner.Up(ctx)
		if err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
		if len(applied) == 0 {
			fmt.Println("✅ Database schema is already up to date. No pending migrations.")
		} else {
			fmt.Printf("✅ Successfully applied %d migration(s):\n", len(applied))
			for _, v := range applied {
				fmt.Printf("   - %s\n", v)
			}
		}
		return nil

	case "status":
		applied, pending, err := runner.Status(ctx)
		if err != nil {
			return fmt.Errorf("failed to retrieve migration status: %w", err)
		}
		fmt.Printf("Database Migration Status (%s - %s):\n", db.DriverName(), cfg.Database.Database)
		fmt.Println("Applied Migrations:")
		if len(applied) == 0 {
			fmt.Println("  (none)")
		} else {
			for _, r := range applied {
				fmt.Printf("  [✓] %s: %s (applied: %s)\n", r.Version, r.Name, r.AppliedAt.Format("2006-01-02 15:04:05"))
			}
		}

		fmt.Println("\nPending Migrations:")
		if len(pending) == 0 {
			fmt.Println("  (none - database is up to date)")
		} else {
			for _, p := range pending {
				fmt.Printf("  [ ] %s: %s\n", p.Version, p.Name)
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown migration action %q; expected 'up' or 'status'", action)
	}
}


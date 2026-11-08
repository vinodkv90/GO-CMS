package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/factory"
	"github.com/go-cms/cms/internal/database/migrations"
)

func main() {
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	command := args[0]

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := factory.Connect(ctx, cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Database connection error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	runner := migrations.NewRunner(db)
	runner.Register(migrations.DefaultMigrations(db.DriverName())...)

	switch command {
	case "up":
		fmt.Printf("Executing pending migrations for database: %s...\n", db.DriverName())
		applied, err := runner.Up(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
			os.Exit(1)
		}
		if len(applied) == 0 {
			fmt.Println("No pending migrations to apply.")
		} else {
			fmt.Printf("Successfully applied %d migration(s):\n", len(applied))
			for _, v := range applied {
				fmt.Printf("  - %s\n", v)
			}
		}

	case "status":
		applied, pending, err := runner.Status(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to retrieve migration status: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Database Migration Status (%s):\n", db.DriverName())
		fmt.Println("Applied Migrations:")
		if len(applied) == 0 {
			fmt.Println("  (none)")
		} else {
			for _, r := range applied {
				fmt.Printf("  [X] %s: %s (applied: %s)\n", r.Version, r.Name, r.AppliedAt.Format("2006-01-02 15:04:05"))
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

	default:
		fmt.Fprintf(os.Stderr, "Unknown migration command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: cms-migrate [options] <command>")
	fmt.Println("\nCommands:")
	fmt.Println("  up       Apply all pending migrations")
	fmt.Println("  status   Print migration status and pending list")
	fmt.Println("\nOptions:")
	fmt.Println("  -config <path>   Path to config.yaml")
}

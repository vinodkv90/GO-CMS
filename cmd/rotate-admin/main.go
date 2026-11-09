package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/factory"
	"github.com/go-cms/cms/internal/database/interfaces"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	newPassword := flag.String("password", "Admin#GoCMS2026!Prod", "New admin password")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := factory.Connect(ctx, cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Database connection error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	sqlDB, ok := db.(interfaces.SQLDatabase)
	if !ok {
		fmt.Fprintf(os.Stderr, "Database is not a SQL database\n")
		os.Exit(1)
	}

	newHash, err := auth.HashPassword(*newPassword, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to hash password: %v\n", err)
		os.Exit(1)
	}

	query := "UPDATE cms_users SET password_hash = $1, updated_at = CURRENT_TIMESTAMP WHERE username = 'admin'"
	if db.DriverName() == "mysql" {
		query = "UPDATE cms_users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE username = 'admin'"
	}

	res, err := sqlDB.ExecContext(ctx, query, newHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update admin password: %v\n", err)
		os.Exit(1)
	}

	rows, _ := res.RowsAffected()
	fmt.Printf("Successfully rotated admin credentials!\n")
	fmt.Printf("  Username: admin\n")
	fmt.Printf("  New Password: %s\n", *newPassword)
	fmt.Printf("  Rows Affected: %d\n", rows)
}


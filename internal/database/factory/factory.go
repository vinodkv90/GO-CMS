package factory

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/go-cms/cms/internal/database/mongodb"
	"github.com/go-cms/cms/internal/database/mysql"
	"github.com/go-cms/cms/internal/database/postgres"
)

// Connect connects to the database specified in the configuration.
func Connect(ctx context.Context, cfg config.DatabaseConfig) (interfaces.Database, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	switch driver {
	case "postgres":
		return postgres.NewPostgres(ctx, cfg)
	case "mysql":
		return mysql.NewMySQL(ctx, cfg)
	case "mongodb":
		return mongodb.NewMongoDB(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported database driver: %q", cfg.Driver)
	}
}


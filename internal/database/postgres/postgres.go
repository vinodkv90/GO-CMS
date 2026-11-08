package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/interfaces"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresDB implements the interfaces.SQLDatabase interface for PostgreSQL.
type PostgresDB struct {
	db           *sql.DB
	capabilities interfaces.Capabilities
}

// NewPostgres creates and initializes a connection to PostgreSQL.
func NewPostgres(ctx context.Context, cfg config.DatabaseConfig) (*PostgresDB, error) {
	connStr := cfg.URI
	if connStr == "" {
		sslMode := cfg.SSLMode
		if sslMode == "" {
			sslMode = "disable"
		}
		connStr = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			url.QueryEscape(cfg.User),
			url.QueryEscape(cfg.Password),
			cfg.Host,
			cfg.Port,
			cfg.Database,
			sslMode,
		)
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	// Apply connection pooling options
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}

	caps := interfaces.NewCapabilitySet(
		interfaces.CapTransactions,
		interfaces.CapForeignKeys,
		interfaces.CapUniqueConstraints,
		interfaces.CapJSONIndexing,
		interfaces.CapFullTextSearch,
		interfaces.CapTransactionalDDL,
		interfaces.CapNestedTx,
		interfaces.CapCursorPagination,
		interfaces.CapBulkUpsert,
	)

	return &PostgresDB{
		db:           db,
		capabilities: caps,
	}, nil
}

// DriverName returns the identifier for this database driver.
func (p *PostgresDB) DriverName() string {
	return "postgres"
}

// Ping checks if the PostgreSQL database connection is alive.
func (p *PostgresDB) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

// Close closes the underlying database pool.
func (p *PostgresDB) Close() error {
	return p.db.Close()
}

// Capabilities returns the capability set supported by PostgreSQL.
func (p *PostgresDB) Capabilities() interfaces.Capabilities {
	return p.capabilities
}

// DB returns the underlying *sql.DB handle.
func (p *PostgresDB) DB() *sql.DB {
	return p.db
}

// ExecContext executes a query without returning any rows.
func (p *PostgresDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, args...)
}

// QueryContext executes a query that returns rows.
func (p *PostgresDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that is expected to return at most one row.
func (p *PostgresDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return p.db.QueryRowContext(ctx, query, args...)
}

// BeginTx starts a new transaction.
func (p *PostgresDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return p.db.BeginTx(ctx, opts)
}

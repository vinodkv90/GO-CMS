package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/interfaces"
	_ "github.com/go-sql-driver/mysql"
)

// MySQLDB implements the interfaces.SQLDatabase interface for MySQL.
type MySQLDB struct {
	db           *sql.DB
	capabilities interfaces.Capabilities
}

// NewMySQL creates and initializes a connection to MySQL.
func NewMySQL(ctx context.Context, cfg config.DatabaseConfig) (*MySQLDB, error) {
	dsn := cfg.URI
	if dsn == "" {
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci",
			cfg.User,
			cfg.Password,
			cfg.Host,
			cfg.Port,
			cfg.Database,
		)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql database: %w", err)
	}

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

	// MySQL does NOT support transactional DDL (implicit commits occur on DDL)
	caps := interfaces.NewCapabilitySet(
		interfaces.CapTransactions,
		interfaces.CapForeignKeys,
		interfaces.CapUniqueConstraints,
		interfaces.CapJSONIndexing,
		interfaces.CapFullTextSearch,
		interfaces.CapNestedTx,
		interfaces.CapCursorPagination,
		interfaces.CapBulkUpsert,
	)

	return &MySQLDB{
		db:           db,
		capabilities: caps,
	}, nil
}

// DriverName returns the identifier for this database driver.
func (m *MySQLDB) DriverName() string {
	return "mysql"
}

// Ping checks if the MySQL database connection is alive.
func (m *MySQLDB) Ping(ctx context.Context) error {
	return m.db.PingContext(ctx)
}

// Close closes the underlying database pool.
func (m *MySQLDB) Close() error {
	return m.db.Close()
}

// Capabilities returns the capability set supported by MySQL.
func (m *MySQLDB) Capabilities() interfaces.Capabilities {
	return m.capabilities
}

// DB returns the underlying *sql.DB handle.
func (m *MySQLDB) DB() *sql.DB {
	return m.db
}

// ExecContext executes a query without returning any rows.
func (m *MySQLDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return m.db.ExecContext(ctx, query, args...)
}

// QueryContext executes a query that returns rows.
func (m *MySQLDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return m.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that is expected to return at most one row.
func (m *MySQLDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return m.db.QueryRowContext(ctx, query, args...)
}

// BeginTx starts a new transaction.
func (m *MySQLDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return m.db.BeginTx(ctx, opts)
}

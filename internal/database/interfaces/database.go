package interfaces

import (
	"context"
	"database/sql"

	"go.mongodb.org/mongo-driver/mongo"
)

// Database is the base interface that every database adapter must implement.
type Database interface {
	DriverName() string
	Ping(ctx context.Context) error
	Close() error
	Capabilities() Capabilities
}

// SQLDatabase defines operations supported by relational SQL database adapters (PostgreSQL, MySQL).
type SQLDatabase interface {
	Database
	DB() *sql.DB
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// MongoDatabase defines operations supported by the MongoDB document database adapter.
type MongoDatabase interface {
	Database
	Client() *mongo.Client
	Database() *mongo.Database
	Collection(name string) *mongo.Collection
}

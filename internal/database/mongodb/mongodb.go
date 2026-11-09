package mongodb

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/interfaces"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// MongoDB implements interfaces.MongoDatabase for MongoDB.
type MongoDB struct {
	client       *mongo.Client
	db           *mongo.Database
	capabilities interfaces.Capabilities
}

// NewMongoDB creates and initializes a connection to MongoDB.
func NewMongoDB(ctx context.Context, cfg config.DatabaseConfig) (*MongoDB, error) {
	uri := cfg.URI
	if uri == "" {
		if cfg.User != "" && cfg.Password != "" {
			uri = fmt.Sprintf("mongodb://%s:%s@%s:%d/%s",
				url.QueryEscape(cfg.User),
				url.QueryEscape(cfg.Password),
				cfg.Host,
				cfg.Port,
				cfg.Database,
			)
		} else {
			uri = fmt.Sprintf("mongodb://%s:%d/%s",
				cfg.Host,
				cfg.Port,
				cfg.Database,
			)
		}
	}

	clientOpts := options.Client().ApplyURI(uri)
	if cfg.MaxOpenConns > 0 {
		clientOpts.SetMaxPoolSize(uint64(cfg.MaxOpenConns))
	}
	if cfg.MaxIdleConns > 0 {
		clientOpts.SetMinPoolSize(uint64(cfg.MaxIdleConns))
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(connectCtx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create mongo client: %w", err)
	}

	dbName := cfg.Database
	if dbName == "" {
		dbName = "cms_db"
	}

	// Base capabilities supported by MongoDB
	capsList := []interfaces.Capability{
		interfaces.CapUniqueConstraints,
		interfaces.CapJSONIndexing,
		interfaces.CapFullTextSearch,
		interfaces.CapCursorPagination,
		interfaces.CapBulkUpsert,
	}

	return &MongoDB{
		client:       client,
		db:           client.Database(dbName),
		capabilities: interfaces.NewCapabilitySet(capsList...),
	}, nil
}

// DriverName returns the identifier for this database driver.
func (m *MongoDB) DriverName() string {
	return "mongodb"
}

// Ping checks if the MongoDB connection is alive.
func (m *MongoDB) Ping(ctx context.Context) error {
	return m.client.Ping(ctx, readpref.Primary())
}

// Close disconnects the MongoDB client.
func (m *MongoDB) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.client.Disconnect(ctx)
}

// Capabilities returns the capability set supported by MongoDB.
func (m *MongoDB) Capabilities() interfaces.Capabilities {
	return m.capabilities
}

// Client returns the underlying *mongo.Client handle.
func (m *MongoDB) Client() *mongo.Client {
	return m.client
}

// Database returns the active *mongo.Database handle.
func (m *MongoDB) Database() *mongo.Database {
	return m.db
}

// Collection returns a *mongo.Collection handle for the specified collection name.
func (m *MongoDB) Collection(name string) *mongo.Collection {
	return m.db.Collection(name)
}

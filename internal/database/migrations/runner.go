package migrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/database/interfaces"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Migration represents an individual versioned database migration.
type Migration struct {
	Version   string
	Name      string
	UpSQL     string // SQL script for relational databases
	DownSQL   string // Rollback SQL
	UpMongo   func(ctx context.Context, db *mongo.Database) error
	DownMongo func(ctx context.Context, db *mongo.Database) error
}

// Record represents a recorded migration in the tracking table/collection.
type Record struct {
	Version   string    `bson:"version"`
	Name      string    `bson:"name"`
	Checksum  string    `bson:"checksum"`
	AppliedAt time.Time `bson:"applied_at"`
}

// Checksum computes a SHA-256 checksum of the migration payload.
func (m *Migration) Checksum() string {
	h := sha256.New()
	if m.UpSQL != "" {
		h.Write([]byte(m.UpSQL))
	} else {
		h.Write([]byte(m.Version + ":" + m.Name))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Runner coordinates executing database migrations.
type Runner struct {
	db         interfaces.Database
	migrations []Migration
}

// NewRunner creates a new migration Runner.
func NewRunner(db interfaces.Database) *Runner {
	return &Runner{
		db:         db,
		migrations: make([]Migration, 0),
	}
}

// Register registers one or more migrations in order.
func (r *Runner) Register(m ...Migration) {
	r.migrations = append(r.migrations, m...)
}

// Migrations returns all registered migrations.
func (r *Runner) Migrations() []Migration {
	return r.migrations
}

// Up runs all unapplied migrations.
func (r *Runner) Up(ctx context.Context) ([]string, error) {
	switch db := r.db.(type) {
	case interfaces.SQLDatabase:
		return r.upSQL(ctx, db)
	case interfaces.MongoDatabase:
		return r.upMongo(ctx, db)
	default:
		return nil, fmt.Errorf("unsupported database type for migrations")
	}
}

// Status returns the list of applied and pending migrations.
func (r *Runner) Status(ctx context.Context) (applied []Record, pending []Migration, err error) {
	switch db := r.db.(type) {
	case interfaces.SQLDatabase:
		appliedMap, err := r.getAppliedSQL(ctx, db)
		if err != nil {
			return nil, nil, err
		}
		for _, m := range r.migrations {
			if rec, ok := appliedMap[m.Version]; ok {
				applied = append(applied, rec)
			} else {
				pending = append(pending, m)
			}
		}
		return applied, pending, nil
	case interfaces.MongoDatabase:
		appliedMap, err := r.getAppliedMongo(ctx, db)
		if err != nil {
			return nil, nil, err
		}
		for _, m := range r.migrations {
			if rec, ok := appliedMap[m.Version]; ok {
				applied = append(applied, rec)
			} else {
				pending = append(pending, m)
			}
		}
		return applied, pending, nil
	default:
		return nil, nil, fmt.Errorf("unsupported database type")
	}
}

// --- SQL Migration Execution ---

func (r *Runner) ensureTrackingTableSQL(ctx context.Context, sqlDB interfaces.SQLDatabase) error {
	var query string
	if sqlDB.DriverName() == "postgres" {
		query = `
		CREATE TABLE IF NOT EXISTS cms_schema_migrations (
			version VARCHAR(64) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			checksum VARCHAR(64) NOT NULL,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`
	} else {
		// MySQL
		query = `
		CREATE TABLE IF NOT EXISTS cms_schema_migrations (
			version VARCHAR(64) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			checksum VARCHAR(64) NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`
	}

	_, err := sqlDB.ExecContext(ctx, query)
	return err
}

func (r *Runner) getAppliedSQL(ctx context.Context, sqlDB interfaces.SQLDatabase) (map[string]Record, error) {
	if err := r.ensureTrackingTableSQL(ctx, sqlDB); err != nil {
		return nil, fmt.Errorf("failed to ensure migrations tracking table: %w", err)
	}

	rows, err := sqlDB.QueryContext(ctx, "SELECT version, name, checksum, applied_at FROM cms_schema_migrations ORDER BY version ASC")
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]Record)
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.Version, &rec.Name, &rec.Checksum, &rec.AppliedAt); err != nil {
			return nil, err
		}
		applied[rec.Version] = rec
	}
	return applied, nil
}

func (r *Runner) upSQL(ctx context.Context, sqlDB interfaces.SQLDatabase) ([]string, error) {
	applied, err := r.getAppliedSQL(ctx, sqlDB)
	if err != nil {
		return nil, err
	}

	var appliedVersions []string
	for _, m := range r.migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}

		if m.UpSQL == "" {
			continue
		}

		// Check capability for transactional DDL
		useTx := sqlDB.Capabilities().Has(interfaces.CapTransactionalDDL)

		if useTx {
			tx, err := sqlDB.BeginTx(ctx, nil)
			if err != nil {
				return appliedVersions, fmt.Errorf("migration %s begin tx error: %w", m.Version, err)
			}

			// Execute migration statements
			queries := splitStatements(m.UpSQL)
			for _, q := range queries {
				if strings.TrimSpace(q) == "" {
					continue
				}
				if _, err := tx.ExecContext(ctx, q); err != nil {
					_ = tx.Rollback()
					return appliedVersions, fmt.Errorf("migration %s (%s) failed: %w", m.Version, m.Name, err)
				}
			}

			insertQuery := "INSERT INTO cms_schema_migrations (version, name, checksum, applied_at) VALUES ($1, $2, $3, $4)"
			if sqlDB.DriverName() == "mysql" {
				insertQuery = "INSERT INTO cms_schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)"
			}

			if _, err := tx.ExecContext(ctx, insertQuery, m.Version, m.Name, m.Checksum(), time.Now().UTC()); err != nil {
				_ = tx.Rollback()
				return appliedVersions, fmt.Errorf("failed to record migration %s: %w", m.Version, err)
			}

			if err := tx.Commit(); err != nil {
				return appliedVersions, fmt.Errorf("migration %s commit failed: %w", m.Version, err)
			}
		} else {
			// Non-transactional DDL (e.g. MySQL)
			queries := splitStatements(m.UpSQL)
			for _, q := range queries {
				if strings.TrimSpace(q) == "" {
					continue
				}
				if _, err := sqlDB.ExecContext(ctx, q); err != nil {
					return appliedVersions, fmt.Errorf("migration %s (%s) failed: %w", m.Version, m.Name, err)
				}
			}

			insertQuery := "INSERT INTO cms_schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)"
			if _, err := sqlDB.ExecContext(ctx, insertQuery, m.Version, m.Name, m.Checksum(), time.Now().UTC()); err != nil {
				return appliedVersions, fmt.Errorf("failed to record migration %s: %w", m.Version, err)
			}
		}

		appliedVersions = append(appliedVersions, m.Version)
	}

	return appliedVersions, nil
}

// --- MongoDB Migration Execution ---

func (r *Runner) ensureTrackingCollectionMongo(ctx context.Context, mongoDB interfaces.MongoDatabase) error {
	col := mongoDB.Collection("cms_schema_migrations")
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "version", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	_, err := col.Indexes().CreateOne(ctx, indexModel)
	return err
}

func (r *Runner) getAppliedMongo(ctx context.Context, mongoDB interfaces.MongoDatabase) (map[string]Record, error) {
	if err := r.ensureTrackingCollectionMongo(ctx, mongoDB); err != nil {
		return nil, fmt.Errorf("failed to ensure mongo migration tracking collection: %w", err)
	}

	col := mongoDB.Collection("cms_schema_migrations")
	cursor, err := col.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to find applied migrations in mongo: %w", err)
	}
	defer cursor.Close(ctx)

	applied := make(map[string]Record)
	for cursor.Next(ctx) {
		var rec Record
		if err := cursor.Decode(&rec); err != nil {
			return nil, err
		}
		applied[rec.Version] = rec
	}
	return applied, nil
}

func (r *Runner) upMongo(ctx context.Context, mongoDB interfaces.MongoDatabase) ([]string, error) {
	applied, err := r.getAppliedMongo(ctx, mongoDB)
	if err != nil {
		return nil, err
	}

	col := mongoDB.Collection("cms_schema_migrations")
	var appliedVersions []string

	for _, m := range r.migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}

		if m.UpMongo != nil {
			if err := m.UpMongo(ctx, mongoDB.Database()); err != nil {
				return appliedVersions, fmt.Errorf("mongo migration %s (%s) failed: %w", m.Version, m.Name, err)
			}
		}

		rec := Record{
			Version:   m.Version,
			Name:      m.Name,
			Checksum:  m.Checksum(),
			AppliedAt: time.Now().UTC(),
		}

		if _, err := col.InsertOne(ctx, rec); err != nil {
			return appliedVersions, fmt.Errorf("failed to record mongo migration %s: %w", m.Version, err)
		}

		appliedVersions = append(appliedVersions, m.Version)
	}

	return appliedVersions, nil
}

// splitStatements splits multi-statement SQL strings by semicolon.
func splitStatements(script string) []string {
	lines := strings.Split(script, ";")
	var statements []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			statements = append(statements, trimmed)
		}
	}
	return statements
}

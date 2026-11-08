package factory

import (
	"context"
	"testing"

	"github.com/go-cms/cms/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestConnect_UnsupportedDriver(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver: "cassandra",
	}

	db, err := Connect(context.Background(), cfg)
	assert.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "unsupported database driver")
}

func TestPostgresCapabilities(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:   "postgres",
		Host:     "localhost",
		Port:     5432,
		User:     "user",
		Password: "password",
		Database: "test_db",
	}

	db, err := Connect(context.Background(), cfg)
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer db.Close()

	assert.Equal(t, "postgres", db.DriverName())
	caps := db.Capabilities()
	assert.True(t, caps.Has("transactions"))
	assert.True(t, caps.Has("foreign_keys"))
	assert.True(t, caps.Has("transactional_ddl"))
	assert.True(t, caps.Has("json_indexing"))
}

func TestMySQLCapabilities(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:   "mysql",
		Host:     "localhost",
		Port:     3306,
		User:     "user",
		Password: "password",
		Database: "test_db",
	}

	db, err := Connect(context.Background(), cfg)
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer db.Close()

	assert.Equal(t, "mysql", db.DriverName())
	caps := db.Capabilities()
	assert.True(t, caps.Has("transactions"))
	assert.True(t, caps.Has("foreign_keys"))
	assert.False(t, caps.Has("transactional_ddl")) // MySQL lacks transactional DDL
	assert.True(t, caps.Has("json_indexing"))
}

func TestMongoDBCapabilities(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:   "mongodb",
		Host:     "localhost",
		Port:     27017,
		Database: "test_db",
	}

	db, err := Connect(context.Background(), cfg)
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer db.Close()

	assert.Equal(t, "mongodb", db.DriverName())
	caps := db.Capabilities()
	assert.True(t, caps.Has("unique_constraints"))
	assert.True(t, caps.Has("json_indexing"))
	assert.True(t, caps.Has("full_text_search"))
	assert.False(t, caps.Has("foreign_keys"))
}

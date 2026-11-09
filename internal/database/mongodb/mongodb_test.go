package mongodb

import (
	"context"
	"testing"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMongoDB_InitializationAndCapabilities(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:       "mongodb",
		Host:         "127.0.0.1",
		Port:         27017,
		User:         "cms_user",
		Password:     "cms_pass",
		Database:     "cms_mongo_test",
		MaxOpenConns: 50,
		MaxIdleConns: 10,
	}

	ctx := context.Background()
	db, err := NewMongoDB(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	assert.Equal(t, "mongodb", db.DriverName())
	assert.NotNil(t, db.Client())
	assert.NotNil(t, db.Database())
	assert.Equal(t, "cms_mongo_test", db.Database().Name())

	col := db.Collection("articles")
	require.NotNil(t, col)
	assert.Equal(t, "articles", col.Name())

	caps := db.Capabilities()
	assert.True(t, caps.Has(interfaces.CapUniqueConstraints))
	assert.True(t, caps.Has(interfaces.CapJSONIndexing))
	assert.True(t, caps.Has(interfaces.CapFullTextSearch))
	assert.True(t, caps.Has(interfaces.CapCursorPagination))
	assert.True(t, caps.Has(interfaces.CapBulkUpsert))
	// MongoDB document database does not have relational foreign keys
	assert.False(t, caps.Has(interfaces.CapForeignKeys))
}

func TestMongoDB_DirectURI(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:   "mongodb",
		URI:      "mongodb://localhost:27017/direct_cms",
		Database: "direct_cms",
	}

	ctx := context.Background()
	db, err := NewMongoDB(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	assert.Equal(t, "mongodb", db.DriverName())
	assert.Equal(t, "direct_cms", db.Database().Name())
}


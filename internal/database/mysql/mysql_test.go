package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMySQL_InitializationAndCapabilities(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:          "mysql",
		Host:            "127.0.0.1",
		Port:            3306,
		User:            "cms_user",
		Password:        "cms_pass",
		Database:        "cms_mysql_test",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 10 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}

	ctx := context.Background()
	db, err := NewMySQL(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	assert.Equal(t, "mysql", db.DriverName())
	assert.NotNil(t, db.DB())

	caps := db.Capabilities()
	assert.True(t, caps.Has(interfaces.CapTransactions))
	assert.True(t, caps.Has(interfaces.CapForeignKeys))
	assert.True(t, caps.Has(interfaces.CapUniqueConstraints))
	assert.True(t, caps.Has(interfaces.CapJSONIndexing))
	assert.True(t, caps.Has(interfaces.CapFullTextSearch))
	// MySQL does NOT support transactional DDL
	assert.False(t, caps.Has(interfaces.CapTransactionalDDL))
}

func TestMySQL_CustomURI(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver: "mysql",
		URI:    "root:secret@tcp(localhost:3306)/custom_db?parseTime=true",
	}

	ctx := context.Background()
	db, err := NewMySQL(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	assert.Equal(t, "mysql", db.DriverName())
}


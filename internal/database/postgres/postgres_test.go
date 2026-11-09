package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgres_Capabilities(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:          "postgres",
		Host:            "127.0.0.1",
		Port:            5432,
		User:            "cms",
		Password:        "cms",
		Database:        "cms_db",
		SSLMode:         "disable",
		MaxOpenConns:    30,
		MaxIdleConns:    15,
		ConnMaxLifetime: 10 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}

	pg, err := NewPostgres(context.Background(), cfg)
	require.NoError(t, err)
	defer pg.Close()

	assert.Equal(t, "postgres", pg.DriverName())
	assert.NotNil(t, pg.DB())

	stats := pg.DB().Stats()
	assert.Equal(t, 30, stats.MaxOpenConnections)

	caps := pg.Capabilities()
	assert.True(t, caps.Has(interfaces.CapTransactions))
	assert.True(t, caps.Has(interfaces.CapTransactionalDDL))
	assert.True(t, caps.Has(interfaces.CapJSONIndexing))
	assert.True(t, caps.Has(interfaces.CapFullTextSearch))
	assert.True(t, caps.Has(interfaces.CapForeignKeys))
}

func TestPostgres_DefaultConnectionPooling(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		User:     "cms",
		Password: "cms",
		Database: "cms_db",
	}

	pg, err := NewPostgres(context.Background(), cfg)
	require.NoError(t, err)
	defer pg.Close()

	stats := pg.DB().Stats()
	assert.Equal(t, 25, stats.MaxOpenConnections)
}

func TestPostgres_PingUnreachable(t *testing.T) {
	// Connect to non-existent port with small timeout
	cfg := config.DatabaseConfig{
		Driver:   "postgres",
		Host:     "127.0.0.1",
		Port:     59999, // Unused port
		User:     "cms",
		Password: "cms",
		Database: "cms_db",
		SSLMode:  "disable",
	}

	pg, err := NewPostgres(context.Background(), cfg)
	require.NoError(t, err)
	defer pg.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err = pg.Ping(ctx)
	assert.Error(t, err)
}


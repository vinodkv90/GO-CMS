package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "postgres", cfg.Database.Driver)
	assert.Equal(t, "development", cfg.Server.Env)
	assert.True(t, cfg.Media.SignedURLs.Enabled)
	assert.NoError(t, cfg.Validate())
}

func TestLoadConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	yamlData := `
server:
  host: "127.0.0.1"
  port: 9000
  env: "development"
database:
  driver: "mysql"
  host: "db.local"
  port: 3306
  database: "test_cms"
media:
  signed_urls:
    enabled: true
    algorithm: "HMAC-SHA256"
    expiration: 2h
`
	err := os.WriteFile(configFile, []byte(yamlData), 0644)
	require.NoError(t, err)

	cfg, err := Load(configFile)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, 9000, cfg.Server.Port)
	assert.Equal(t, "mysql", cfg.Database.Driver)
	assert.Equal(t, "db.local", cfg.Database.Host)
	assert.Equal(t, 3306, cfg.Database.Port)
	assert.Equal(t, "test_cms", cfg.Database.Database)
	assert.Equal(t, 2*time.Hour, cfg.Media.SignedURLs.Expiration)
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("CMS_SERVER_PORT", "9999")
	t.Setenv("CMS_DB_DRIVER", "mongodb")
	t.Setenv("CMS_DB_NAME", "cms_mongo_dev")

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, "mongodb", cfg.Database.Driver)
	assert.Equal(t, "cms_mongo_dev", cfg.Database.Database)
}

func TestConfigValidation(t *testing.T) {
	// Invalid port
	cfg := DefaultConfig()
	cfg.Server.Port = 70000
	assert.Error(t, cfg.Validate())

	// Unsupported DB driver
	cfg = DefaultConfig()
	cfg.Database.Driver = "sqlite"
	assert.Error(t, cfg.Validate())

	// Production with default insecure secrets
	cfg = DefaultConfig()
	cfg.Server.Env = "production"
	assert.Error(t, cfg.Validate())

	// Production with custom secrets
	cfg.Auth.JWTSecret = "very-strong-production-jwt-secret-key-12345"
	cfg.Auth.SessionSecret = "very-strong-production-session-secret-key-67890"
	cfg.Media.SignedURLs.SecretKey = "very-strong-production-media-secret-key-abcde"
	assert.NoError(t, cfg.Validate())
}

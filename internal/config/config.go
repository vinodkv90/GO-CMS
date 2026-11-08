package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/core/logging"
	"gopkg.in/yaml.v3"
)

// ServerConfig defines HTTP server configuration options.
type ServerConfig struct {
	Host               string        `yaml:"host"`
	Port               int           `yaml:"port"`
	Env                string        `yaml:"env"` // "development", "staging", "production"
	ReadTimeout        time.Duration `yaml:"read_timeout"`
	WriteTimeout       time.Duration `yaml:"write_timeout"`
	IdleTimeout        time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout    time.Duration `yaml:"shutdown_timeout"`
	CORSAllowedOrigins []string      `yaml:"cors_allowed_origins"`
	TrustedProxies     []string      `yaml:"trusted_proxies"`
}

// DatabaseConfig defines database connection options.
type DatabaseConfig struct {
	Driver          string        `yaml:"driver"` // "postgres", "mysql", "mongodb"
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	User            string        `yaml:"user"`
	Password        string        `yaml:"password"`
	Database        string        `yaml:"database"`
	SSLMode         string        `yaml:"ssl_mode"` // "disable", "require", "verify-full"
	URI             string        `yaml:"uri"`      // Optional direct connection URI
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time"`
}

// SignedURLConfig defines HMAC signed URL settings for media assets.
type SignedURLConfig struct {
	Enabled    bool          `yaml:"enabled"`
	Algorithm  string        `yaml:"algorithm"` // "HMAC-SHA256"
	Expiration time.Duration `yaml:"expiration"`
	SecretKey  string        `yaml:"secret_key"`
}

// MediaConfig defines media storage and signed URL configuration.
type MediaConfig struct {
	Provider   string          `yaml:"provider"` // "local", "s3", "gcs", "azure", "minio"
	UploadDir  string          `yaml:"upload_dir"`
	SignedURLs SignedURLConfig `yaml:"signed_urls"`
}

// AuthConfig defines authentication and security token settings.
type AuthConfig struct {
	JWTSecret       string        `yaml:"jwt_secret"`
	SessionSecret   string        `yaml:"session_secret"`
	TokenExpiration time.Duration `yaml:"token_expiration"`
}

// ThemesConfig defines theme settings.
type ThemesConfig struct {
	ActiveTheme string `yaml:"active_theme"`
	ThemesDir   string `yaml:"themes_dir"`
	Cache       bool   `yaml:"cache"`
}

// PluginsConfig defines plugin settings.
type PluginsConfig struct {
	PluginsDir string `yaml:"plugins_dir"`
}

// Config is the root configuration structure.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Logging  logging.Config `yaml:"logging"`
	Auth     AuthConfig     `yaml:"auth"`
	Media    MediaConfig    `yaml:"media"`
	Themes   ThemesConfig   `yaml:"themes"`
	Plugins  PluginsConfig  `yaml:"plugins"`
}

// DefaultConfig returns safe and reasonable default configuration values.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:               "0.0.0.0",
			Port:               8080,
			Env:                "development",
			ReadTimeout:        15 * time.Second,
			WriteTimeout:       30 * time.Second,
			IdleTimeout:        60 * time.Second,
			ShutdownTimeout:    10 * time.Second,
			CORSAllowedOrigins: []string{"*"},
			TrustedProxies:     []string{"127.0.0.1/32", "::1/128"},
		},
		Database: DatabaseConfig{
			Driver:          "postgres",
			Host:            "localhost",
			Port:            5432,
			User:            "cms",
			Password:        "cms_secret_password",
			Database:        "cms_db",
			SSLMode:         "disable",
			MaxOpenConns:    25,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
		Logging: logging.Config{
			Level:  "info",
			Format: "text",
		},
		Auth: AuthConfig{
			JWTSecret:       "default-insecure-jwt-secret-change-me",
			SessionSecret:   "default-insecure-session-secret-change-me",
			TokenExpiration: 24 * time.Hour,
		},
		Media: MediaConfig{
			Provider:  "local",
			UploadDir: "./uploads",
			SignedURLs: SignedURLConfig{
				Enabled:    true,
				Algorithm:  "HMAC-SHA256",
				Expiration: 1 * time.Hour,
				SecretKey:  "default-media-secret-key-change-me",
			},
		},
		Themes: ThemesConfig{
			ActiveTheme: "default",
			ThemesDir:   "./themes",
			Cache:       false,
		},
		Plugins: PluginsConfig{
			PluginsDir: "./plugins",
		},
	}
}

// Load loads configuration from an optional file and applies environment variable overrides.
func Load(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	if configPath == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			configPath = "config.yaml"
		}
	}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %q: %w", configPath, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse yaml config file %q: %w", configPath, err)
		}
	}

	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation error: %w", err)
	}

	return cfg, nil
}

// applyEnvOverrides allows overriding config values with CMS_* environment variables.
func applyEnvOverrides(cfg *Config) {
	if val := os.Getenv("CMS_SERVER_HOST"); val != "" {
		cfg.Server.Host = val
	}
	if val := os.Getenv("CMS_SERVER_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.Server.Port = p
		}
	}
	if val := os.Getenv("CMS_SERVER_ENV"); val != "" {
		cfg.Server.Env = strings.ToLower(val)
	}

	if val := os.Getenv("CMS_DB_DRIVER"); val != "" {
		cfg.Database.Driver = strings.ToLower(val)
	}
	if val := os.Getenv("CMS_DB_HOST"); val != "" {
		cfg.Database.Host = val
	}
	if val := os.Getenv("CMS_DB_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.Database.Port = p
		}
	}
	if val := os.Getenv("CMS_DB_USER"); val != "" {
		cfg.Database.User = val
	}
	if val := os.Getenv("CMS_DB_PASSWORD"); val != "" {
		cfg.Database.Password = val
	}
	if val := os.Getenv("CMS_DB_NAME"); val != "" {
		cfg.Database.Database = val
	}
	if val := os.Getenv("CMS_DB_SSLMODE"); val != "" {
		cfg.Database.SSLMode = val
	}
	if val := os.Getenv("CMS_DB_URI"); val != "" {
		cfg.Database.URI = val
	}

	if val := os.Getenv("CMS_LOG_LEVEL"); val != "" {
		cfg.Logging.Level = val
	}
	if val := os.Getenv("CMS_LOG_FORMAT"); val != "" {
		cfg.Logging.Format = val
	}

	if val := os.Getenv("CMS_AUTH_JWT_SECRET"); val != "" {
		cfg.Auth.JWTSecret = val
	}
	if val := os.Getenv("CMS_AUTH_SESSION_SECRET"); val != "" {
		cfg.Auth.SessionSecret = val
	}

	if val := os.Getenv("CMS_MEDIA_PROVIDER"); val != "" {
		cfg.Media.Provider = val
	}
	if val := os.Getenv("CMS_MEDIA_UPLOAD_DIR"); val != "" {
		cfg.Media.UploadDir = val
	}
	if val := os.Getenv("CMS_MEDIA_SIGNED_SECRET"); val != "" {
		cfg.Media.SignedURLs.SecretKey = val
	}
}

// Validate checks that required configuration constraints are satisfied.
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be between 1 and 65535)", c.Server.Port)
	}

	validDrivers := map[string]bool{
		"postgres": true,
		"mysql":    true,
		"mongodb":  true,
	}
	driver := strings.ToLower(c.Database.Driver)
	if !validDrivers[driver] {
		return fmt.Errorf("unsupported database driver: %q (supported: postgres, mysql, mongodb)", c.Database.Driver)
	}

	if c.Server.Env == "production" {
		if c.Auth.JWTSecret == "default-insecure-jwt-secret-change-me" {
			return fmt.Errorf("production environment requires a secure CMS_AUTH_JWT_SECRET")
		}
		if c.Auth.SessionSecret == "default-insecure-session-secret-change-me" {
			return fmt.Errorf("production environment requires a secure CMS_AUTH_SESSION_SECRET")
		}
	}

	return nil
}


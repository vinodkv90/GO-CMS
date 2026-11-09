package cache

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/config"
)

// ErrCacheMiss is returned when a requested key is absent or expired.
var ErrCacheMiss = errors.New("cache: key not found")

// Cache defines the universal contract for caching mechanisms in Go CMS.
type Cache interface {
	// Get retrieves cached bytes for a given key. Returns ErrCacheMiss if not found.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a byte slice with an expiration TTL and optional cache tags.
	Set(ctx context.Context, key string, val []byte, ttl time.Duration, tags ...string) error

	// Delete removes a single key from the cache.
	Delete(ctx context.Context, key string) error

	// DeleteByTag purges all cached keys associated with the specified tag.
	DeleteByTag(ctx context.Context, tag string) error

	// Clear purges all entries in the cache.
	Clear(ctx context.Context) error

	// Close cleanly terminates connections or background cleanup routines.
	Close() error
}

// New constructs a Cache implementation based on the supplied configuration.
func New(cfg config.CacheConfig) (Cache, error) {
	driver := strings.ToLower(cfg.Driver)
	switch driver {
	case "redis":
		if cfg.RedisURL == "" {
			return nil, errors.New("cache: redis_url cannot be empty when redis driver is selected")
		}
		return NewRedisCache(cfg.RedisURL, cfg.Prefix)
	case "memory", "":
		return NewMemoryCache(cfg.Prefix), nil
	default:
		return nil, fmt.Errorf("cache: unsupported cache driver %q (supported: memory, redis)", cfg.Driver)
	}
}

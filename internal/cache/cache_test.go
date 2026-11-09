package cache

import (
	"context"
	"testing"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryCache_BasicOperations(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache("test:")
	defer c.Close()

	// 1. Get non-existent
	_, err := c.Get(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrCacheMiss)

	// 2. Set and Get
	data := []byte("hello world")
	err = c.Set(ctx, "key1", data, 1*time.Minute)
	require.NoError(t, err)

	val, err := c.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, data, val)

	// 3. Delete
	err = c.Delete(ctx, "key1")
	require.NoError(t, err)

	_, err = c.Get(ctx, "key1")
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestMemoryCache_Expiration(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache("")
	defer c.Close()

	// Set with 20ms TTL
	err := c.Set(ctx, "fast_key", []byte("fast"), 20*time.Millisecond)
	require.NoError(t, err)

	// Immediate get
	val, err := c.Get(ctx, "fast_key")
	require.NoError(t, err)
	assert.Equal(t, []byte("fast"), val)

	// Wait for expiration
	time.Sleep(30 * time.Millisecond)

	_, err = c.Get(ctx, "fast_key")
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestMemoryCache_TagInvalidation(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache("cms:")
	defer c.Close()

	// Set multiple items with tags
	err := c.Set(ctx, "article:1", []byte("article 1 data"), 5*time.Minute, "articles", "type:article")
	require.NoError(t, err)
	err = c.Set(ctx, "article:2", []byte("article 2 data"), 5*time.Minute, "articles", "type:article")
	require.NoError(t, err)
	err = c.Set(ctx, "page:home", []byte("home data"), 5*time.Minute, "pages", "type:page")
	require.NoError(t, err)

	// Confirm all exist
	_, err = c.Get(ctx, "article:1")
	require.NoError(t, err)
	_, err = c.Get(ctx, "article:2")
	require.NoError(t, err)
	_, err = c.Get(ctx, "page:home")
	require.NoError(t, err)

	// Invalidate by tag "articles"
	err = c.DeleteByTag(ctx, "articles")
	require.NoError(t, err)

	// article:1 and article:2 should be gone
	_, err = c.Get(ctx, "article:1")
	assert.ErrorIs(t, err, ErrCacheMiss)
	_, err = c.Get(ctx, "article:2")
	assert.ErrorIs(t, err, ErrCacheMiss)

	// page:home should still remain cached
	pageVal, err := c.Get(ctx, "page:home")
	require.NoError(t, err)
	assert.Equal(t, []byte("home data"), pageVal)

	// Clear all
	err = c.Clear(ctx)
	require.NoError(t, err)
	_, err = c.Get(ctx, "page:home")
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestCacheFactory(t *testing.T) {
	// 1. Memory default
	cfgMem := config.CacheConfig{
		Driver: "memory",
		Prefix: "test:",
	}
	c1, err := New(cfgMem)
	require.NoError(t, err)
	require.NotNil(t, c1)
	defer c1.Close()

	// 2. Unsupported driver
	cfgBad := config.CacheConfig{
		Driver: "memcached",
	}
	_, err = New(cfgBad)
	assert.ErrorContains(t, err, "unsupported cache driver")

	// 3. Redis missing URL
	cfgRedisNoURL := config.CacheConfig{
		Driver: "redis",
	}
	_, err = New(cfgRedisNoURL)
	assert.ErrorContains(t, err, "redis_url cannot be empty")
}


package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache provides a distributed cache implementation backed by Redis.
type RedisCache struct {
	client *redis.Client
	prefix string
}

// NewRedisCache initializes a Redis client using the provided connection string.
func NewRedisCache(redisURL, prefix string) (*RedisCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("cache: invalid redis URL %q: %w", redisURL, err)
	}

	client := redis.NewClient(opts)
	return &RedisCache{
		client: client,
		prefix: prefix,
	}, nil
}

func (r *RedisCache) formatKey(key string) string {
	if r.prefix == "" {
		return key
	}
	return r.prefix + key
}

func (r *RedisCache) formatTagKey(tag string) string {
	return r.formatKey("tag:" + tag)
}

// Get fetches data from Redis.
func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	k := r.formatKey(key)
	val, err := r.client.Get(ctx, k).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, fmt.Errorf("cache: redis get failed: %w", err)
	}
	return val, nil
}

// Set stores data in Redis with TTL and indexes tags in Redis sets.
func (r *RedisCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration, tags ...string) error {
	k := r.formatKey(key)

	pipe := r.client.Pipeline()
	pipe.Set(ctx, k, val, ttl)

	// Add key to tag sets
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		tagKey := r.formatTagKey(tag)
		pipe.SAdd(ctx, tagKey, k)
		if ttl > 0 {
			// Ensure tag index does not linger indefinitely if not updated
			pipe.Expire(ctx, tagKey, ttl+1*time.Hour)
		}
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("cache: redis set pipeline failed: %w", err)
	}

	return nil
}

// Delete removes a key from Redis.
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	k := r.formatKey(key)
	return r.client.Del(ctx, k).Err()
}

// DeleteByTag purges all keys stored under the given tag set.
func (r *RedisCache) DeleteByTag(ctx context.Context, tag string) error {
	tagKey := r.formatTagKey(tag)

	// Fetch all keys mapped to this tag
	keys, err := r.client.SMembers(ctx, tagKey).Result()
	if err != nil {
		return fmt.Errorf("cache: redis smembers failed: %w", err)
	}

	if len(keys) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()
	pipe.Del(ctx, keys...)
	pipe.Del(ctx, tagKey)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("cache: redis tag delete pipeline failed: %w", err)
	}

	return nil
}

// Clear flushes keys matching the configured prefix.
func (r *RedisCache) Clear(ctx context.Context) error {
	pattern := r.formatKey("*")
	var cursor uint64
	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("cache: redis scan failed: %w", err)
		}

		if len(keys) > 0 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("cache: redis del failed: %w", err)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// Close disconnects the Redis client.
func (r *RedisCache) Close() error {
	return r.client.Close()
}

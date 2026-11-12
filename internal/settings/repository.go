package settings

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrNotFound indicates a requested settings record or preference does not exist yet.
	ErrNotFound = errors.New("settings record not found")
)

// Record represents a row in cms_settings.
type Record struct {
	ID        string          `json:"id"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	IsPublic  bool            `json:"is_public"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Repository persists typed CMS settings and per-user preferences.
type Repository interface {
	GetSetting(ctx context.Context, key string) (*Record, error)
	UpsertSetting(ctx context.Context, key string, value json.RawMessage, isPublic bool) (*Record, error)
	GetPreferences(ctx context.Context, userID string) (*UserPreferences, error)
	UpsertPreferences(ctx context.Context, prefs *UserPreferences) (*UserPreferences, error)
}


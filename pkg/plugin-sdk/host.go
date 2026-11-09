package pluginsdk

import (
	"context"
	"log/slog"
	"net/http"
)

// EventHandler is a callback invoked when a lifecycle event is triggered.
type EventHandler func(ctx context.Context, eventName string, payload any) error

// FieldValidator validates a field value according to plugin logic.
type FieldValidator func(value any) error

// Host provides the execution environment and CMS extension APIs to an activated plugin.
type Host interface {
	// Logger provides structured contextual logging scoped to the plugin.
	Logger() *slog.Logger

	// SubscribeEvent registers a hook callback for core lifecycle events (e.g. content:afterCreate).
	SubscribeEvent(eventName string, handler EventHandler) error

	// RegisterFieldType registers a custom data field type into the CMS fields engine.
	RegisterFieldType(fieldType string, validator FieldValidator) error

	// MountRoute registers an HTTP endpoint scoped to the plugin (accessible under /api/v1/plugins/:plugin_id/:path).
	MountRoute(method string, path string, handler http.HandlerFunc)

	// GetSetting retrieves a configuration value set by administrators for this plugin.
	GetSetting(key string) (any, bool)

	// SetSetting updates a configuration setting for this plugin.
	SetSetting(key string, value any) error

	// RecordAuditLog records a structured audit event into the CMS audit store.
	RecordAuditLog(ctx context.Context, action string, resourceType string, resourceID string, metadata map[string]any)
}

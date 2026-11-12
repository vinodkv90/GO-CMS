package auditlog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/go-cms/cms/internal/content"
	pluginsdk "github.com/go-cms/cms/pkg/plugin-sdk"
)

// Plugin implements enterprise audit logging for content mutations.
type Plugin struct {
	pluginsdk.BasePlugin
	eventCount int64
}

// New creates a new Audit Log plugin instance.
func New() *Plugin {
	return &Plugin{}
}

// Manifest returns the plugin manifest.
func (p *Plugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:                "audit-log",
		Name:              "Audit Log Plugin",
		Version:           "1.0.0",
		Description:       "Enterprise tamper-evident audit logging for content mutations and security events",
		Author:            "Go CMS Core Team",
		MinimumCMSVersion: "1.0.0",
		Runtime:           "go-native",
		Permissions: []string{
			"hook:content:afterCreate",
			"hook:content:afterUpdate",
			"hook:content:afterDelete",
			"hook:content:afterPublish",
			"audit:write",
		},
		SettingsSchema: map[string]any{
			"capture_content_events": map[string]any{
				"type":        "boolean",
				"label":       "Capture Content Lifecycle Events",
				"description": "Subscribe to content create, update, delete, and publish events on the core EventBus.",
				"default":     true,
			},
			"redact_sensitive_metadata": map[string]any{
				"type":        "boolean",
				"label":       "Redact Sensitive Metadata",
				"description": "Automatically redact credentials and tokens before persisting metadata.",
				"default":     true,
			},
			"retention_days": map[string]any{
				"type":        "number",
				"label":       "Audit Retention Target (Days)",
				"description": "Operational retention guideline in days.",
				"default":     90,
				"min":         7,
				"max":         3650,
			},
		},
		Routes: []pluginsdk.RouteDef{
			{Method: http.MethodGet, Path: "/stats", Permission: "read:system:audit_logs"},
		},
	}
}

// Init sets up event subscriptions and routes.
func (p *Plugin) Init(ctx context.Context, host pluginsdk.Host) error {
	p.Host = host

	// 1. Subscribe to Content Lifecycle Hooks
	events := []string{
		string(content.EventAfterCreate),
		string(content.EventAfterUpdate),
		string(content.EventAfterDelete),
		string(content.EventAfterPublish),
	}

	for _, evt := range events {
		eventCopy := evt
		err := host.SubscribeEvent(eventCopy, func(ctx context.Context, name string, payload any) error {
			if rawEnabled, exists := host.GetSetting("capture_content_events"); exists {
				if enabled, ok := rawEnabled.(bool); ok && !enabled {
					return nil
				}
			}
			atomic.AddInt64(&p.eventCount, 1)

			entry, ok := payload.(*content.ContentEntry)
			if !ok {
				return nil
			}

			meta := map[string]any{
				"entry_id":     entry.ID,
				"slug":         entry.Slug,
				"content_type": entry.ContentTypeID,
				"version":      entry.DraftVersion,
				"status":       entry.Status,
			}

			host.RecordAuditLog(ctx, name, "content_entry", entry.ID, meta)
			host.Logger().Info("Audit event captured", "event", name, "entry_id", entry.ID)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to subscribe to %s: %w", evt, err)
		}
	}

	// 2. Mount API Route /stats
	host.MountRoute(http.MethodGet, "/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plugin_id":    "audit-log",
			"active":       true,
			"total_events": atomic.LoadInt64(&p.eventCount),
		})
	})

	return nil
}

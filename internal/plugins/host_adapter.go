package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/go-cms/cms/internal/fields"
	pluginsdk "github.com/go-cms/cms/pkg/plugin-sdk"
	"github.com/google/uuid"
)

// HostAdapter implements pluginsdk.Host by bridging into CMS core subsystems.
type HostAdapter struct {
	mu         sync.RWMutex
	pluginID   string
	pluginName string
	logger     *slog.Logger
	eventBus   *content.EventBus
	fieldReg   *fields.Registry
	router     chi.Router
	db         interfaces.Database
	settings   map[string]any
	onSettings func(key string, val any)
	onAudit    func(ctx context.Context, action, resourceType, resourceID string, metadata map[string]any)
}

// SetAuditRecorder configures the unified host audit recorder callback.
func (h *HostAdapter) SetAuditRecorder(fn func(ctx context.Context, action, resourceType, resourceID string, metadata map[string]any)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAudit = fn
}

// SetPluginName records the display name used when registering plugin metadata.
func (h *HostAdapter) SetPluginName(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pluginName = name
}

// NewHostAdapter creates a new HostAdapter for a specific plugin.
func NewHostAdapter(
	pluginID string,
	logger *slog.Logger,
	eventBus *content.EventBus,
	fieldReg *fields.Registry,
	router chi.Router,
	db interfaces.Database,
	settings map[string]any,
	onSettings func(key string, val any),
) *HostAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	if settings == nil {
		settings = make(map[string]any)
	}

	return &HostAdapter{
		pluginID:   pluginID,
		logger:     logger.With("plugin_id", pluginID),
		eventBus:   eventBus,
		fieldReg:   fieldReg,
		router:     router,
		db:         db,
		settings:   settings,
		onSettings: onSettings,
	}
}

// Logger returns the plugin-scoped logger.
func (h *HostAdapter) Logger() *slog.Logger {
	return h.logger
}

// SubscribeEvent binds a plugin handler to a core content lifecycle event.
func (h *HostAdapter) SubscribeEvent(eventName string, handler pluginsdk.EventHandler) error {
	if h.eventBus == nil {
		return fmt.Errorf("event bus not available")
	}

	h.eventBus.Subscribe(content.LifecycleEvent(eventName), func(ctx context.Context, event content.LifecycleEvent, entry *content.ContentEntry) error {
		return handler(ctx, string(event), entry)
	})
	return nil
}

// RegisterFieldType registers a custom data field type into the fields registry.
func (h *HostAdapter) RegisterFieldType(fieldType string, validator pluginsdk.FieldValidator) error {
	if h.fieldReg == nil {
		return fmt.Errorf("field registry not available")
	}

	h.mu.RLock()
	pluginName := h.pluginName
	h.mu.RUnlock()

	var fn fields.FieldValidatorFunc
	if validator != nil {
		fn = fields.FieldValidatorFunc(validator)
	}
	desc := fields.CustomFieldDescriptor{
		Type:       fields.FieldType(fieldType),
		PluginSlug: h.pluginID,
		PluginName: pluginName,
	}
	if err := h.fieldReg.RegisterCustom(desc, fn); err != nil {
		return err
	}
	h.logger.Info("Plugin registered custom field type", "type", fieldType, "plugin", h.pluginID)
	return nil
}

// MountRoute adds a scoped HTTP route to the plugin's router.
func (h *HostAdapter) MountRoute(method, path string, handler http.HandlerFunc) {
	if h.router == nil {
		return
	}

	switch method {
	case http.MethodGet:
		h.router.Get(path, handler)
	case http.MethodPost:
		h.router.Post(path, handler)
	case http.MethodPut:
		h.router.Put(path, handler)
	case http.MethodPatch:
		h.router.Patch(path, handler)
	case http.MethodDelete:
		h.router.Delete(path, handler)
	default:
		h.router.Method(method, path, handler)
	}

	h.logger.Debug("Mounted plugin route", "method", method, "path", path)
}

// GetSetting retrieves a configuration value.
func (h *HostAdapter) GetSetting(key string) (any, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	v, ok := h.settings[key]
	return v, ok
}

// SetSetting updates a configuration setting.
func (h *HostAdapter) SetSetting(key string, val any) error {
	h.mu.Lock()
	h.settings[key] = val
	h.mu.Unlock()

	if h.onSettings != nil {
		h.onSettings(key, val)
	}
	return nil
}

// RecordAuditLog routes plugin audit events through the core audit pipeline when configured,
// falling back to direct SQL persistence when no host recorder is attached.
func (h *HostAdapter) RecordAuditLog(ctx context.Context, action, resourceType, resourceID string, metadata map[string]any) {
	h.mu.RLock()
	recorder := h.onAudit
	h.mu.RUnlock()
	if recorder != nil {
		recorder(ctx, action, resourceType, resourceID, metadata)
		return
	}

	sqlDB, ok := h.db.(interfaces.SQLDatabase)
	if !ok || sqlDB == nil {
		h.logger.Info("Audit Log Recorded (no SQL DB)", "action", action, "resource_type", resourceType, "resource_id", resourceID)
		return
	}

	metaJSON, _ := json.Marshal(metadata)
	query := "INSERT INTO cms_audit_logs (id, user_id, actor_type, action, resource_type, resource_id, metadata, ip_address, result, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	if sqlDB.DriverName() == "postgres" {
		query = "INSERT INTO cms_audit_logs (id, user_id, actor_type, action, resource_type, resource_id, metadata, ip_address, result, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)"
	}

	logID := uuid.New().String()
	now := time.Now().UTC()
	_, err := sqlDB.ExecContext(ctx, query, logID, "system", "plugin", action, resourceType, resourceID, string(metaJSON), "127.0.0.1", "success", now)
	if err != nil {
		h.logger.Warn("Failed to persist audit log", "error", err)
	}
}

package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/go-cms/cms/internal/fields"
	pluginsdk "github.com/go-cms/cms/pkg/plugin-sdk"
)

var (
	ErrPluginNotFound    = errors.New("plugin not found")
	ErrPluginAlreadyRuns = errors.New("plugin is already enabled")
	ErrPluginNotEnabled  = errors.New("plugin is not enabled")
)

// AuditRecorderFunc bridges plugin audit events into the core audit logging service.
type AuditRecorderFunc func(ctx context.Context, action, resourceType, resourceID string, metadata map[string]any)

// Manager coordinates plugin discovery, registration, lifecycle transitions, and API routing.
type Manager struct {
	mu            sync.RWMutex
	pluginsDir    string
	logger        *slog.Logger
	eventBus      *content.EventBus
	fieldReg      *fields.Registry
	db            interfaces.Database
	pluginsRouter chi.Router
	auditFn       AuditRecorderFunc

	entities   map[string]*PluginEntity
	instances  map[string]pluginsdk.Plugin
	adapters   map[string]*HostAdapter
	subrouters map[string]chi.Router
}

// SetAuditRecorder installs the host audit recorder for current and future enabled plugins.
func (m *Manager) SetAuditRecorder(fn AuditRecorderFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditFn = fn
	for _, adapter := range m.adapters {
		adapter.SetAuditRecorder(fn)
	}
}

// NewManager creates a new plugin lifecycle manager.
func NewManager(
	pluginsDir string,
	eventBus *content.EventBus,
	fieldReg *fields.Registry,
	db interfaces.Database,
	logger *slog.Logger,
) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	if pluginsDir == "" {
		pluginsDir = "./plugins"
	}

	mgr := &Manager{
		pluginsDir:    pluginsDir,
		logger:        logger,
		eventBus:      eventBus,
		fieldReg:      fieldReg,
		db:            db,
		pluginsRouter: chi.NewRouter(),
		entities:      make(map[string]*PluginEntity),
		instances:     make(map[string]pluginsdk.Plugin),
		adapters:      make(map[string]*HostAdapter),
		subrouters:    make(map[string]chi.Router),
	}

	mgr.pluginsRouter.HandleFunc("/{pluginID}", mgr.dispatchRoute)
	mgr.pluginsRouter.HandleFunc("/{pluginID}/*", mgr.dispatchRoute)
	return mgr
}

func (m *Manager) dispatchRoute(w http.ResponseWriter, r *http.Request) {
	pluginID := chi.URLParam(r, "pluginID")
	if pluginID == "" {
		http.NotFound(w, r)
		return
	}

	m.mu.RLock()
	entity, ok := m.entities[pluginID]
	subrouter, hasSub := m.subrouters[pluginID]
	m.mu.RUnlock()

	if !ok || entity.Status != StatusEnabled || !hasSub {
		http.NotFound(w, r)
		return
	}

	subPath := "/" + strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if perm := findRoutePermission(entity.Manifest.Routes, r.Method, subPath); perm != "" {
		if auth.GetUserFromContext(r.Context()) == nil {
			http.Error(w, `{"error":"Authentication required"}`, http.StatusUnauthorized)
			return
		}
		action, resource := splitPluginPermission(perm)
		if !auth.RequestHasPermission(r.Context(), action, resource) {
			http.Error(w, `{"error":"Forbidden: insufficient permissions"}`, http.StatusForbidden)
			return
		}
	}

	subReq := r.Clone(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	subReq.URL.Path = subPath
	subrouter.ServeHTTP(w, subReq)
}

func findRoutePermission(routes []pluginsdk.RouteDef, method, subPath string) string {
	cleanSub := "/" + strings.Trim(subPath, "/")
	for _, rt := range routes {
		if !strings.EqualFold(strings.TrimSpace(rt.Method), method) {
			continue
		}
		cleanRoute := "/" + strings.Trim(rt.Path, "/")
		if cleanRoute == cleanSub || (strings.HasSuffix(cleanRoute, "/*") && strings.HasPrefix(cleanSub, strings.TrimSuffix(cleanRoute, "/*"))) {
			return strings.TrimSpace(rt.Permission)
		}
	}
	return ""
}

func splitPluginPermission(perm string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(perm), ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "read", perm
}

// Router returns the parent Chi router that routes requests to individual plugin subrouters.
func (m *Manager) Router() chi.Router {
	return m.pluginsRouter
}

// Scan inspects the plugins directory on disk and registers discovered plugin manifests.
func (m *Manager) Scan() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(m.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			_ = os.MkdirAll(m.pluginsDir, 0755)
			return nil
		}
		return fmt.Errorf("failed to read plugins directory %q: %w", m.pluginsDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginDir := filepath.Join(m.pluginsDir, entry.Name())
		manifestFile := filepath.Join(pluginDir, "plugin.json")

		data, err := os.ReadFile(manifestFile)
		if err != nil {
			continue // No manifest, skip
		}

		manifest, err := pluginsdk.ParseManifest(data)
		if err != nil {
			m.logger.Warn("Failed to parse plugin manifest", "dir", entry.Name(), "error", err)
			continue
		}

		// Keep existing settings if already registered
		existing, ok := m.entities[manifest.ID]
		settings := make(map[string]any)
		status := StatusDiscovered
		if ok {
			settings = existing.Settings
			status = existing.Status
		}

		m.entities[manifest.ID] = &PluginEntity{
			Manifest: *manifest,
			Status:   status,
			Settings: settings,
			Dir:      pluginDir,
		}
	}

	return nil
}

// RegisterNative registers an in-process Go-native plugin instance with the manager.
func (m *Manager) RegisterNative(p pluginsdk.Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest := p.Manifest()
	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("invalid native plugin manifest: %w", err)
	}

	m.instances[manifest.ID] = p

	existing, ok := m.entities[manifest.ID]
	settings := applySchemaDefaults(manifest.SettingsSchema, nil)
	status := StatusInstalled
	if ok {
		settings = applySchemaDefaults(manifest.SettingsSchema, existing.Settings)
		status = existing.Status
	}

	m.entities[manifest.ID] = &PluginEntity{
		Manifest: manifest,
		Status:   status,
		Settings: settings,
	}

	m.logger.Info("Registered native plugin", "id", manifest.ID, "name", manifest.Name, "version", manifest.Version)
	return nil
}

// Enable transitions a plugin from disabled/installed to enabled and starts its runtime.
func (m *Manager) Enable(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, ok := m.entities[id]
	if !ok {
		return ErrPluginNotFound
	}

	if entity.Status == StatusEnabled {
		return ErrPluginAlreadyRuns
	}

	instance, ok := m.instances[id]
	if !ok {
		return fmt.Errorf("no executable runtime found for plugin %q (ensure plugin is registered)", id)
	}

	// Create isolated subrouter for plugin
	sub := chi.NewRouter()
	m.subrouters[id] = sub

	adapter := NewHostAdapter(
		id,
		m.logger,
		m.eventBus,
		m.fieldReg,
		sub,
		m.db,
		entity.Settings,
		func(k string, v any) {
			entity.Settings[k] = v
		},
	)
	adapter.SetPluginName(entity.Manifest.Name)
	if m.auditFn != nil {
		adapter.SetAuditRecorder(m.auditFn)
	}
	m.adapters[id] = adapter

	// Initialize and start plugin
	if err := instance.Init(ctx, adapter); err != nil {
		if m.fieldReg != nil {
			m.fieldReg.UnregisterPluginFields(id)
		}
		delete(m.adapters, id)
		delete(m.subrouters, id)
		entity.Status = StatusError
		entity.ErrorMsg = fmt.Sprintf("init failure: %v", err)
		return fmt.Errorf("failed to initialize plugin %q: %w", id, err)
	}

	if err := instance.Start(ctx); err != nil {
		if m.fieldReg != nil {
			m.fieldReg.UnregisterPluginFields(id)
		}
		delete(m.adapters, id)
		delete(m.subrouters, id)
		entity.Status = StatusError
		entity.ErrorMsg = fmt.Sprintf("start failure: %v", err)
		return fmt.Errorf("failed to start plugin %q: %w", id, err)
	}

	now := time.Now().UTC()
	entity.Status = StatusEnabled
	entity.EnabledAt = &now
	entity.ErrorMsg = ""
	m.persistPluginStateLocked(ctx, entity)

	m.logger.Info("Plugin enabled successfully", "id", id, "name", entity.Manifest.Name)
	return nil
}

// Disable stops an active plugin and detaches its extension points.
func (m *Manager) Disable(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, ok := m.entities[id]
	if !ok {
		return ErrPluginNotFound
	}

	if entity.Status != StatusEnabled {
		return ErrPluginNotEnabled
	}

	if instance, ok := m.instances[id]; ok {
		if err := instance.Stop(ctx); err != nil {
			m.logger.Warn("Error stopping plugin instance", "id", id, "error", err)
		}
	}
	if m.fieldReg != nil {
		m.fieldReg.UnregisterPluginFields(id)
	}

	delete(m.adapters, id)
	delete(m.subrouters, id)

	entity.Status = StatusDisabled
	entity.EnabledAt = nil
	m.persistPluginStateLocked(ctx, entity)

	m.logger.Info("Plugin disabled", "id", id)
	return nil
}

// Get returns the plugin entity by ID.
func (m *Manager) Get(id string) (*PluginEntity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entity, ok := m.entities[id]
	if !ok {
		return nil, ErrPluginNotFound
	}

	clone := *entity
	return &clone, nil
}

// List returns all discovered and installed plugins.
func (m *Manager) List() []*PluginEntity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []*PluginEntity
	for _, e := range m.entities {
		clone := *e
		list = append(list, &clone)
	}
	return list
}

// UpdateSettings updates configuration options for a plugin.
func (m *Manager) UpdateSettings(ctx context.Context, id string, settings map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entity, ok := m.entities[id]
	if !ok {
		return ErrPluginNotFound
	}

	if entity.Settings == nil {
		entity.Settings = make(map[string]any)
	}

	for k, v := range settings {
		entity.Settings[k] = v
	}

	if adapter, ok := m.adapters[id]; ok {
		for k, v := range settings {
			_ = adapter.SetSetting(k, v)
		}
	}
	m.persistPluginStateLocked(ctx, entity)

	m.logger.Info("Updated plugin settings", "id", id)
	return nil
}

// BootstrapPlugins reconciles registered plugins with persisted database state and activates defaults on first boot.
func (m *Manager) BootstrapPlugins(ctx context.Context, defaultEnabled ...string) error {
	states := m.loadAllPersistedStates(ctx)
	defaultsMap := make(map[string]bool, len(defaultEnabled))
	for _, id := range defaultEnabled {
		defaultsMap[id] = true
	}

	m.mu.Lock()
	var toEnable []string
	for id, entity := range m.entities {
		if st, exists := states[id]; exists {
			entity.Settings = applySchemaDefaults(entity.Manifest.SettingsSchema, st.Settings)
			if st.Enabled {
				if entity.Status != StatusEnabled {
					toEnable = append(toEnable, id)
				}
			} else {
				entity.Status = StatusDisabled
				entity.EnabledAt = nil
			}
		} else {
			entity.Settings = applySchemaDefaults(entity.Manifest.SettingsSchema, entity.Settings)
			if defaultsMap[id] && entity.Status != StatusEnabled {
				toEnable = append(toEnable, id)
			}
		}
	}
	m.mu.Unlock()

	for _, id := range toEnable {
		if err := m.Enable(ctx, id); err != nil {
			m.logger.Warn("Failed to activate plugin during bootstrap", "plugin", id, "error", err)
		}
	}
	return nil
}

func applySchemaDefaults(schema map[string]any, current map[string]any) map[string]any {
	out := make(map[string]any, len(current)+len(schema))
	for k, rawDef := range schema {
		if defMap, ok := rawDef.(map[string]any); ok {
			if defVal, hasDefault := defMap["default"]; hasDefault {
				out[k] = defVal
			}
		}
	}
	for k, v := range current {
		out[k] = v
	}
	return out
}

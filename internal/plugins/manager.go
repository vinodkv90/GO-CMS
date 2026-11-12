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

// Manager coordinates plugin discovery, registration, lifecycle transitions, and API routing.
type Manager struct {
	mu            sync.RWMutex
	pluginsDir    string
	logger        *slog.Logger
	eventBus      *content.EventBus
	fieldReg      *fields.Registry
	db            interfaces.Database
	pluginsRouter chi.Router

	entities   map[string]*PluginEntity
	instances  map[string]pluginsdk.Plugin
	adapters   map[string]*HostAdapter
	subrouters map[string]chi.Router
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
	subReq := r.Clone(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	subReq.URL.Path = subPath
	subrouter.ServeHTTP(w, subReq)
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
	settings := make(map[string]any)
	status := StatusInstalled
	if ok {
		settings = existing.Settings
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

	m.logger.Info("Updated plugin settings", "id", id)
	return nil
}

package plugins

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
)

// Handler handles administrative plugin management endpoints.
type Handler struct {
	mgr *Manager
}

// NewHandler creates a new plugin management HTTP handler.
func NewHandler(mgr *Manager) *Handler {
	return &Handler{mgr: mgr}
}

// RegisterRoutes registers the plugin administrative REST API endpoints.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// 1. Mount plugin custom subroutes under /api/v1/plugins/ext/*
	r.Mount("/plugins/ext", h.mgr.Router())

	// 2. Admin plugin management API
	r.Route("/plugins", func(pr chi.Router) {
		pr.Use(auth.RequireAuth)

		pr.With(auth.RequirePermission("read", "plugins")).Get("/", h.List)
		pr.With(auth.RequirePermission("read", "plugins")).Get("/{id}", h.Get)
		pr.With(auth.RequirePermission("update", "plugins")).Post("/{id}/enable", h.Enable)
		pr.With(auth.RequirePermission("update", "plugins")).Post("/{id}/disable", h.Disable)
		pr.With(auth.RequirePermission("update", "plugins")).Patch("/{id}/settings", h.UpdateSettings)
	})
}

// List returns all discovered and installed plugins.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	plugins := h.mgr.List()
	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": plugins,
	})
}

// Get returns details for a single plugin.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	entity, err := h.mgr.Get(id)
	if err != nil {
		if errors.Is(err, ErrPluginNotFound) {
			h.writeError(w, http.StatusNotFound, "Plugin not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": entity,
	})
}

// Enable activates a plugin.
func (h *Handler) Enable(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.mgr.Enable(r.Context(), id); err != nil {
		if errors.Is(err, ErrPluginNotFound) {
			h.writeError(w, http.StatusNotFound, "Plugin not found")
			return
		}
		if errors.Is(err, ErrPluginAlreadyRuns) {
			h.writeError(w, http.StatusConflict, "Plugin is already enabled")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	entity, _ := h.mgr.Get(id)
	h.writeJSON(w, http.StatusOK, map[string]any{
		"message": "Plugin enabled successfully",
		"data":    entity,
	})
}

// Disable deactivates a plugin.
func (h *Handler) Disable(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.mgr.Disable(r.Context(), id); err != nil {
		if errors.Is(err, ErrPluginNotFound) {
			h.writeError(w, http.StatusNotFound, "Plugin not found")
			return
		}
		if errors.Is(err, ErrPluginNotEnabled) {
			h.writeError(w, http.StatusBadRequest, "Plugin is not enabled")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	entity, _ := h.mgr.Get(id)
	h.writeJSON(w, http.StatusOK, map[string]any{
		"message": "Plugin disabled successfully",
		"data":    entity,
	})
}

// UpdateSettings updates administrative settings for a plugin.
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var settings map[string]any
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid JSON settings payload")
		return
	}

	if err := h.mgr.UpdateSettings(r.Context(), id, settings); err != nil {
		if errors.Is(err, ErrPluginNotFound) {
			h.writeError(w, http.StatusNotFound, "Plugin not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	entity, _ := h.mgr.Get(id)
	h.writeJSON(w, http.StatusOK, map[string]any{
		"message": "Settings updated successfully",
		"data":    entity,
	})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{
		"error": message,
	})
}

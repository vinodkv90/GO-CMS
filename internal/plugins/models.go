package plugins

import (
	"time"

	pluginsdk "github.com/go-cms/cms/pkg/plugin-sdk"
)

// PluginStatus represents the operational state of a plugin.
type PluginStatus string

const (
	StatusDiscovered PluginStatus = "discovered"
	StatusInstalled  PluginStatus = "installed"
	StatusEnabled    PluginStatus = "enabled"
	StatusDisabled   PluginStatus = "disabled"
	StatusError      PluginStatus = "error"
)

// PluginEntity holds the runtime registration state and administrative configuration of a plugin.
type PluginEntity struct {
	Manifest  pluginsdk.Manifest `json:"manifest"`
	Status    PluginStatus       `json:"status"`
	Settings  map[string]any     `json:"settings"`
	Dir       string             `json:"dir,omitempty"`
	EnabledAt *time.Time         `json:"enabled_at,omitempty"`
	ErrorMsg  string             `json:"error_msg,omitempty"`
}


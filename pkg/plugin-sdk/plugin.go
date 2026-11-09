package pluginsdk

import (
	"context"
)

// Plugin is the contract that every Go-native CMS plugin must implement.
type Plugin interface {
	// Manifest returns the plugin manifest describing identity and permissions.
	Manifest() Manifest

	// Init is called when the plugin is loaded into the host environment.
	Init(ctx context.Context, host Host) error

	// Start activates plugin background workers, event subscriptions, and routes.
	Start(ctx context.Context) error

	// Stop cleanly terminates plugin background tasks and releases resources.
	Stop(ctx context.Context) error
}

// BasePlugin provides a sensible default implementation of optional Plugin methods.
type BasePlugin struct {
	Host Host
}

// Init saves the host instance.
func (b *BasePlugin) Init(ctx context.Context, host Host) error {
	b.Host = host
	return nil
}

// Start default no-op.
func (b *BasePlugin) Start(ctx context.Context) error {
	return nil
}

// Stop default no-op.
func (b *BasePlugin) Stop(ctx context.Context) error {
	return nil
}

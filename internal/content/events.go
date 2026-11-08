package content

import (
	"context"
	"fmt"
	"sync"
)

// LifecycleEvent defines the content mutation lifecycle points.
type LifecycleEvent string

const (
	EventBeforeCreate  LifecycleEvent = "before_create"
	EventAfterCreate   LifecycleEvent = "after_create"
	EventBeforeUpdate  LifecycleEvent = "before_update"
	EventAfterUpdate   LifecycleEvent = "after_update"
	EventBeforeDelete  LifecycleEvent = "before_delete"
	EventAfterDelete   LifecycleEvent = "after_delete"
	EventBeforePublish LifecycleEvent = "before_publish"
	EventAfterPublish  LifecycleEvent = "after_publish"
)

// HookFunc is a callback invoked when a lifecycle event fires.
type HookFunc func(ctx context.Context, event LifecycleEvent, entry *ContentEntry) error

// EventBus manages registration and synchronous execution of content lifecycle hooks.
type EventBus struct {
	mu    sync.RWMutex
	hooks map[LifecycleEvent][]HookFunc
}

// NewEventBus creates an initialized EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		hooks: make(map[LifecycleEvent][]HookFunc),
	}
}

// Subscribe registers a callback for a specific lifecycle event.
func (b *EventBus) Subscribe(event LifecycleEvent, hook HookFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hooks[event] = append(b.hooks[event], hook)
}

// Emit executes all registered hooks for an event in sequence.
// If any "Before" hook returns an error, the operation is aborted.
func (b *EventBus) Emit(ctx context.Context, event LifecycleEvent, entry *ContentEntry) error {
	b.mu.RLock()
	hooks := b.hooks[event]
	b.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook(ctx, event, entry); err != nil {
			return fmt.Errorf("lifecycle hook %s failed: %w", event, err)
		}
	}
	return nil
}


package webhooks

import "time"

// SupportedEvents enumerates the CMS lifecycle and administrative events that can trigger webhooks.
var SupportedEvents = []string{
	"entry.create",
	"entry.update",
	"entry.delete",
	"entry.publish",
	"media.create",
	"media.delete",
	"content_type.create",
	"content_type.update",
	"content_type.delete",
}

// EventDescription provides metadata for the webhook configuration UI.
type EventDescription struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// EventCatalog describes all supported webhook events.
func EventCatalog() []EventDescription {
	return []EventDescription{
		{Name: "entry.create", Label: "Entry Created", Category: "Content", Description: "Triggered when a new content entry is created."},
		{Name: "entry.update", Label: "Entry Updated", Category: "Content", Description: "Triggered when a content entry draft or metadata is updated."},
		{Name: "entry.delete", Label: "Entry Deleted", Category: "Content", Description: "Triggered when a content entry is deleted."},
		{Name: "entry.publish", Label: "Entry Published", Category: "Content", Description: "Triggered when a content entry is published."},
		{Name: "media.create", Label: "Media Uploaded", Category: "Media", Description: "Triggered when a media asset is uploaded."},
		{Name: "media.delete", Label: "Media Deleted", Category: "Media", Description: "Triggered when a media asset is deleted."},
		{Name: "content_type.create", Label: "Content-Type Created", Category: "Schema", Description: "Triggered when a collection or single type schema is created."},
		{Name: "content_type.update", Label: "Content-Type Updated", Category: "Schema", Description: "Triggered when a schema definition is modified."},
		{Name: "content_type.delete", Label: "Content-Type Deleted", Category: "Schema", Description: "Triggered when a schema definition is removed."},
	}
}

// Webhook represents a persisted outgoing webhook subscription.
type Webhook struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	URL              string            `json:"url"`
	Headers          map[string]string `json:"headers"`
	Events           []string          `json:"events"`
	Secret           string            `json:"secret,omitempty"`
	SecretConfigured bool              `json:"secret_configured"`
	SecretEnc        string            `json:"-"`
	IsEnabled        bool              `json:"is_enabled"`
	MaxRetries       int               `json:"max_retries"`
	CreatedBy        string            `json:"created_by,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// WebhookDelivery records a single webhook delivery attempt or test execution.
type WebhookDelivery struct {
	ID              string    `json:"id"`
	WebhookID       string    `json:"webhook_id"`
	Event           string    `json:"event"`
	RequestURL      string    `json:"request_url"`
	StatusCode      int       `json:"status_code"`
	DurationMs      int64     `json:"duration_ms"`
	Attempt         int       `json:"attempt"`
	Success         bool      `json:"success"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	ResponseSnippet string    `json:"response_snippet,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateWebhookRequest creates a new webhook subscription.
type CreateWebhookRequest struct {
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers"`
	Events     []string          `json:"events"`
	Secret     string            `json:"secret"`
	IsEnabled  *bool             `json:"is_enabled"`
	MaxRetries *int              `json:"max_retries"`
}

// UpdateWebhookRequest modifies an existing webhook subscription.
type UpdateWebhookRequest struct {
	Name       *string           `json:"name"`
	URL        *string           `json:"url"`
	Headers    map[string]string `json:"headers"`
	Events     []string          `json:"events"`
	Secret     *string           `json:"secret"`
	IsEnabled  *bool             `json:"is_enabled"`
	MaxRetries *int              `json:"max_retries"`
}


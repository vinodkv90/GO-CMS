package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-cms/cms/internal/content"
)

// Indexer listens to content lifecycle hooks and synchronizes the search index.
type Indexer struct {
	engine *Engine
}

// NewIndexer creates an Indexer wrapping the search Engine.
func NewIndexer(engine *Engine) *Indexer {
	return &Indexer{engine: engine}
}

// AttachLifecycleHooks subscribes the indexer to content mutations on the EventBus.
func (idx *Indexer) AttachLifecycleHooks(bus *content.EventBus) {
	if bus == nil {
		return
	}

	// Index published content
	bus.Subscribe(content.EventAfterPublish, func(ctx context.Context, event content.LifecycleEvent, entry *content.ContentEntry) error {
		idx.IndexEntry(entry)
		return nil
	})

	// Remove from index if deleted
	bus.Subscribe(content.EventAfterDelete, func(ctx context.Context, event content.LifecycleEvent, entry *content.ContentEntry) error {
		if entry != nil {
			idx.engine.Delete(entry.ID)
		}
		return nil
	})
}

// IndexEntry converts a ContentEntry into a searchable Document and adds it to the index.
func (idx *Indexer) IndexEntry(entry *content.ContentEntry) {
	if entry == nil {
		return
	}

	data := entry.PublishedData
	if data == nil || len(data) == 0 {
		data = entry.DraftData
	}
	if data == nil {
		return
	}

	title := extractString(data, "title", "name", "headline")
	if title == "" && entry.Slug != "" {
		title = entry.Slug
	}

	bodyParts := make([]string, 0)
	for k, v := range data {
		if k == "title" || k == "name" || k == "headline" {
			continue
		}
		if s, ok := v.(string); ok && len(s) > 0 {
			bodyParts = append(bodyParts, s)
		}
	}
	body := strings.Join(bodyParts, " ")

	docType := entry.ContentTypeUID
	if docType == "" {
		docType = entry.ContentTypeID
	}

	doc := Document{
		ID:    entry.ID,
		Type:  docType,
		Title: title,
		Body:  body,
		Metadata: map[string]any{
			"slug":             entry.Slug,
			"status":           string(entry.Status),
			"content_type_id":  entry.ContentTypeID,
			"content_type_uid": entry.ContentTypeUID,
		},
	}

	idx.engine.Index(doc)
}

func extractString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, isStr := v.(string); isStr && strings.TrimSpace(s) != "" {
				return s
			}
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

// Engine returns the underlying search engine instance.
func (idx *Indexer) Engine() *Engine {
	return idx.engine
}


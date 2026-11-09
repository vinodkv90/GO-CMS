package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_IndexAndSearch(t *testing.T) {
	engine := NewEngine()

	// 1. Index sample documents
	doc1 := Document{
		ID:    "art-1",
		Type:  "articles",
		Title: "Building Scalable Systems in Go",
		Body:  "Go concurrency primitives like goroutines and channels make building high performance backends straightforward.",
		Tags:  []string{"golang", "systems"},
	}

	doc2 := Document{
		ID:    "art-2",
		Type:  "articles",
		Title: "Introduction to MongoDB and Document Stores",
		Body:  "MongoDB offers flexible schemaless data modeling and high availability with replica sets.",
		Tags:  []string{"database", "nosql"},
	}

	doc3 := Document{
		ID:    "prod-1",
		Type:  "products",
		Title: "Go Programming Mechanical Keyboard",
		Body:  "High quality tactile mechanical keyboard designed for Go software engineers.",
		Tags:  []string{"hardware", "golang"},
	}

	engine.Index(doc1)
	engine.Index(doc2)
	engine.Index(doc3)

	// 2. Search for "concurrency" -> should match doc1
	res := engine.Search("concurrency", "", 10)
	require.Len(t, res, 1)
	assert.Equal(t, "art-1", res[0].Document.ID)
	assert.NotEmpty(t, res[0].Highlights)

	// 3. Search for "golang" with type filter "products" -> should only return doc3
	res = engine.Search("golang", "products", 10)
	require.Len(t, res, 1)
	assert.Equal(t, "prod-1", res[0].Document.ID)

	// 4. Search for "Go" without filter -> doc1 and doc3 (doc3 has Go in title, doc1 has Go in title)
	res = engine.Search("go", "", 10)
	assert.GreaterOrEqual(t, len(res), 2)

	// 5. Delete doc1 and verify it is removed
	engine.Delete("art-1")
	res = engine.Search("concurrency", "", 10)
	assert.Empty(t, res)

	// 6. Clear index
	engine.Clear()
	res = engine.Search("database", "", 10)
	assert.Empty(t, res)
}

func TestIndexer_LifecycleHookIntegration(t *testing.T) {
	engine := NewEngine()
	indexer := NewIndexer(engine)
	bus := content.NewEventBus()
	indexer.AttachLifecycleHooks(bus)

	entry := &content.ContentEntry{
		ID:            "entry-100",
		ContentTypeID: "articles",
		Slug:          "hello-world",
		Status:        content.StatusPublished,
		PublishedData: map[string]any{
			"title":   "Hello World from Go CMS",
			"content": "This is an automated content article published via event bus.",
		},
	}

	// 1. Emit AfterPublish event
	ctx := context.Background()
	err := bus.Emit(ctx, content.EventAfterPublish, entry)
	require.NoError(t, err)

	// Search for "automated"
	res := engine.Search("automated", "", 10)
	require.Len(t, res, 1)
	assert.Equal(t, "entry-100", res[0].Document.ID)

	// 2. Emit AfterDelete event
	err = bus.Emit(ctx, content.EventAfterDelete, entry)
	require.NoError(t, err)

	res = engine.Search("automated", "", 10)
	assert.Empty(t, res)
}

func TestHTTPHandler_SearchEndpoint(t *testing.T) {
	engine := NewEngine()
	engine.Index(Document{
		ID:    "doc-1",
		Type:  "articles",
		Title: "Enterprise Go Architectures",
		Body:  "Modular monolith architectures provide superior maintainability.",
	})

	handler := NewHTTPHandler(engine)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/search?q=architectures&type=articles", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data := resp["data"].([]any)
	assert.Len(t, data, 1)

	meta := resp["meta"].(map[string]any)
	assert.Equal(t, "architectures", meta["query"])
	assert.Equal(t, float64(1), meta["total"])
}


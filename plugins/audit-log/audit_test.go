package auditlog_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/fields"
	"github.com/go-cms/cms/internal/plugins"
	auditlog "github.com/go-cms/cms/plugins/audit-log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditLogPlugin_EventCaptureAndStats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eventBus := content.NewEventBus()
	fieldReg := fields.NewRegistry()
	subrouter := chi.NewRouter()

	adapter := plugins.NewHostAdapter(
		"audit-log",
		logger,
		eventBus,
		fieldReg,
		subrouter,
		nil,
		make(map[string]any),
		nil,
	)

	plugin := auditlog.New()
	manifest := plugin.Manifest()
	assert.Equal(t, "audit-log", manifest.ID)

	ctx := context.Background()
	require.NoError(t, plugin.Init(ctx, adapter))
	require.NoError(t, plugin.Start(ctx))

	// 1. Emit AfterCreate hook
	entry1 := &content.ContentEntry{
		ID:            "entry-101",
		ContentTypeID: "articles",
		Slug:          "first-post",
		Status:        content.StatusDraft,
		DraftVersion:  1,
	}
	err := eventBus.Emit(ctx, content.EventAfterCreate, entry1)
	require.NoError(t, err)

	// 2. Emit AfterPublish hook
	entry1.Status = content.StatusPublished
	err = eventBus.Emit(ctx, content.EventAfterPublish, entry1)
	require.NoError(t, err)

	// 3. Test /stats endpoint
	req := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()
	subrouter.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var stats map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &stats)
	require.NoError(t, err)

	assert.Equal(t, "audit-log", stats["plugin_id"])
	assert.Equal(t, true, stats["active"])
	assert.Equal(t, float64(2), stats["total_events"])

	require.NoError(t, plugin.Stop(ctx))
}

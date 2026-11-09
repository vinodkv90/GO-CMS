package seoanalyzer_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/fields"
	"github.com/go-cms/cms/internal/plugins"
	seoanalyzer "github.com/go-cms/cms/plugins/seo-analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSEOAnalyzerPlugin_AnalyzeEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eventBus := content.NewEventBus()
	fieldReg := fields.NewRegistry()
	subrouter := chi.NewRouter()

	adapter := plugins.NewHostAdapter(
		"seo-analyzer",
		logger,
		eventBus,
		fieldReg,
		subrouter,
		nil,
		make(map[string]any),
		nil,
	)

	plugin := seoanalyzer.New()
	ctx := context.Background()
	require.NoError(t, plugin.Init(ctx, adapter))
	require.NoError(t, plugin.Start(ctx))

	// 1. Valid high-scoring post
	goodContent := strings.Repeat("content word is important for ranking. ", 15)
	payload := map[string]string{
		"title":   "The Complete Comprehensive Guide to Go CMS Platform",
		"content": goodContent,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/analyze", bytes.NewReader(body))
	w := httptest.NewRecorder()
	subrouter.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, float64(100), resp["score"])
	assert.Greater(t, resp["word_count"].(float64), float64(50))

	// 2. Low-scoring post with recommendations
	badPayload := map[string]string{
		"title":   "Short",
		"content": "Too short.",
	}
	body, _ = json.Marshal(badPayload)

	req = httptest.NewRequest("POST", "/analyze", bytes.NewReader(body))
	w = httptest.NewRecorder()
	subrouter.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Less(t, resp["score"].(float64), float64(80))
	recs := resp["recommendations"].([]any)
	assert.NotEmpty(t, recs)

	require.NoError(t, plugin.Stop(ctx))
}

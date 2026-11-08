package seoanalyzer

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	pluginsdk "github.com/go-cms/cms/pkg/plugin-sdk"
)

// Plugin implements SEO analysis for CMS content editors.
type Plugin struct {
	pluginsdk.BasePlugin
}

// New creates a new SEO Analyzer plugin instance.
func New() *Plugin {
	return &Plugin{}
}

// Manifest returns the plugin manifest.
func (p *Plugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:                "seo-analyzer",
		Name:              "SEO Analyzer Plugin",
		Version:           "1.0.0",
		Description:       "Analyzes content title and body for SEO best practices and keyword density",
		Author:            "Go CMS Core Team",
		MinimumCMSVersion: "1.0.0",
		Runtime:           "go-native",
		Permissions: []string{
			"routes:mount",
		},
		Routes: []pluginsdk.RouteDef{
			{Method: http.MethodPost, Path: "/analyze"},
		},
	}
}

// Init mounts the /analyze endpoint.
func (p *Plugin) Init(ctx context.Context, host pluginsdk.Host) error {
	p.Host = host

	host.MountRoute(http.MethodPost, "/analyze", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
			return
		}

		score := 100
		var recommendations []string

		titleLen := len(strings.TrimSpace(input.Title))
		if titleLen < 20 {
			score -= 15
			recommendations = append(recommendations, "Title is too short (< 20 characters)")
		} else if titleLen > 70 {
			score -= 10
			recommendations = append(recommendations, "Title is too long (> 70 characters)")
		}

		words := strings.Fields(input.Content)
		if len(words) < 50 {
			score -= 25
			recommendations = append(recommendations, "Content length is low (< 50 words)")
		}

		if score < 0 {
			score = 0
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"score":           score,
			"word_count":      len(words),
			"recommendations": recommendations,
		})
	})

	return nil
}


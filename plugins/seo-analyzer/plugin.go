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
		SettingsSchema: map[string]any{
			"min_title_length": map[string]any{
				"type":        "number",
				"label":       "Minimum Title Length",
				"description": "Minimum recommended character length for page titles.",
				"default":     20,
				"min":         5,
				"max":         120,
			},
			"max_title_length": map[string]any{
				"type":        "number",
				"label":       "Maximum Title Length",
				"description": "Maximum recommended character length before search snippet truncation.",
				"default":     70,
				"min":         30,
				"max":         200,
			},
			"min_word_count": map[string]any{
				"type":        "number",
				"label":       "Minimum Content Word Count",
				"description": "Minimum recommended body word count for SEO depth scoring.",
				"default":     50,
				"min":         10,
				"max":         5000,
			},
		},
		Routes: []pluginsdk.RouteDef{
			{Method: http.MethodPost, Path: "/analyze", Permission: "read:plugins"},
		},
	}
}

func intSetting(host pluginsdk.Host, key string, fallback int) int {
	v, ok := host.GetSetting(key)
	if !ok || v == nil {
		return fallback
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return fallback
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

		minTitle := intSetting(host, "min_title_length", 20)
		maxTitle := intSetting(host, "max_title_length", 70)
		minWords := intSetting(host, "min_word_count", 50)

		score := 100
		var recommendations []string

		titleLen := len(strings.TrimSpace(input.Title))
		if titleLen < minTitle {
			score -= 15
			recommendations = append(recommendations, "Title is too short")
		} else if titleLen > maxTitle {
			score -= 10
			recommendations = append(recommendations, "Title is too long")
		}

		words := strings.Fields(input.Content)
		if len(words) < minWords {
			score -= 25
			recommendations = append(recommendations, "Content length is low")
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

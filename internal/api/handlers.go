package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/interfaces"
)

const CMSVersion = "1.0.0"

// HealthHandler returns a lightweight 200 OK liveness response.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// ReadinessHandler checks database connectivity and overall system health.
func ReadinessHandler(db interfaces.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "healthy"
		var dbErr string
		if db != nil {
			if err := db.Ping(ctx); err != nil {
				dbStatus = "unhealthy"
				dbErr = err.Error()
			}
		} else {
			dbStatus = "not_configured"
		}

		status := http.StatusOK
		overall := "ready"
		if dbStatus != "healthy" {
			status = http.StatusServiceUnavailable
			overall = "not_ready"
		}

		response := map[string]any{
			"status": overall,
			"components": map[string]any{
				"database": map[string]any{
					"status": dbStatus,
					"driver": db.DriverName(),
					"error":  dbErr,
				},
			},
			"time": time.Now().UTC().Format(time.RFC3339),
		}

		respondJSON(w, status, response)
	}
}

// InfoHandler returns system runtime metadata and database capabilities.
func InfoHandler(cfg *config.Config, db interfaces.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var caps []string
		var driverName string

		if db != nil {
			driverName = db.DriverName()
			for _, c := range db.Capabilities().All() {
				caps = append(caps, string(c))
			}
		}

		response := map[string]any{
			"name":        "Go CMS Platform",
			"version":     CMSVersion,
			"environment": cfg.Server.Env,
			"database": map[string]any{
				"driver":       driverName,
				"capabilities": caps,
			},
			"media": map[string]any{
				"provider":    cfg.Media.Provider,
				"signed_urls": cfg.Media.SignedURLs.Enabled,
			},
			"theme": map[string]any{
				"active": cfg.Themes.ActiveTheme,
			},
		}

		respondJSON(w, http.StatusOK, response)
	}
}

// PingHandler provides a simple API ping test.
func PingHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"message": "pong",
	})
}

// findOpenAPISpec locates the openapi.yaml specification file.
func findOpenAPISpec() string {
	candidates := []string{
		"docs/openapi.yaml",
		"../docs/openapi.yaml",
		"../../docs/openapi.yaml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "docs/openapi.yaml"
}

// OpenAPIHandler serves the raw OpenAPI 3.0 specification.
func OpenAPIHandler(w http.ResponseWriter, r *http.Request) {
	path := findOpenAPISpec()
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	http.ServeFile(w, r, path)
}

// DocsHandler serves interactive API documentation using Swagger UI.
func DocsHandler(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Go CMS - API Documentation</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: '/api/v1/openapi.yaml',
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIBundle.SwaggerUIStandalonePreset
        ],
        layout: "BaseLayout"
      });
    };
  </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

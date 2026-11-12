package api

import (
	"net/http"
	"slices"
	"sync"

	"github.com/go-chi/cors"
	"github.com/go-cms/cms/internal/settings"
)

// DynamicCORS wraps go-chi/cors so CORS rules update safely at runtime without restart.
type DynamicCORS struct {
	mu      sync.RWMutex
	enabled bool
	handler http.Handler
	mw      func(http.Handler) http.Handler
}

// NewDynamicCORS constructs a live-updatable CORS middleware from initial CORS settings.
func NewDynamicCORS(cfg settings.CORSSettings) *DynamicCORS {
	d := &DynamicCORS{}
	d.Update(cfg)
	return d
}

// Update atomically swaps the active CORS middleware policy.
func (d *DynamicCORS) Update(cfg settings.CORSSettings) {
	origins := cfg.AllowedOrigins
	if len(origins) == 0 {
		origins = []string{"*"}
	}
	// Safety invariant: never send wildcard '*' with Access-Control-Allow-Credentials: true.
	allowCreds := cfg.AllowCredentials && !slices.Contains(origins, "*")

	opts := cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   cfg.AllowedMethods,
		AllowedHeaders:   cfg.AllowedHeaders,
		ExposedHeaders:   cfg.ExposedHeaders,
		AllowCredentials: allowCreds,
		MaxAge:           cfg.MaxAgeSeconds,
	}
	mw := cors.Handler(opts)

	d.mu.Lock()
	d.enabled = cfg.Enabled
	d.mw = mw
	d.mu.Unlock()
}

// Middleware applies the currently configured CORS handler when CORS is enabled.
func (d *DynamicCORS) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.RLock()
		enabled := d.enabled
		mw := d.mw
		d.mu.RUnlock()

		if !enabled || mw == nil {
			next.ServeHTTP(w, r)
			return
		}
		mw(next).ServeHTTP(w, r)
	})
}


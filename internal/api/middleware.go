package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-cms/cms/internal/core/logging"
)

// SecurityHeadersMiddleware adds essential security headers to every response.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'self';")
		next.ServeHTTP(w, r)
	})
}

// StructuredLoggerMiddleware logs every HTTP request using slog with context attributes.
func StructuredLoggerMiddleware(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			reqID := middleware.GetReqID(r.Context())
			ctx := logging.WithContext(r.Context(), reqID, "", "")
			r = r.WithContext(ctx)

			defer func() {
				duration := time.Since(start)
				status := ww.Status()
				if status == 0 {
					status = http.StatusOK
				}

				attrs := []any{
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"duration_ms", duration.Milliseconds(),
					"bytes", ww.BytesWritten(),
					"remote_ip", r.RemoteAddr,
					"user_agent", r.UserAgent(),
				}
				if reqID != "" {
					attrs = append(attrs, "request_id", reqID)
				}

				if status >= 500 {
					logger.ErrorContext(ctx, "HTTP Server Error", attrs...)
				} else if status >= 400 {
					logger.WarnContext(ctx, "HTTP Client Error", attrs...)
				} else {
					logger.InfoContext(ctx, "HTTP Request", attrs...)
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}


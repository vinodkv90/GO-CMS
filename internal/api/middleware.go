package api

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-cms/cms/internal/core/logging"
)

// SecurityHeadersMiddleware adds essential security headers to every response.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return SecurityHeadersMiddlewareForEnvironment(next, false)
}

// SecurityHeadersMiddlewareForEnvironment adds response security headers and
// enables HSTS only for production deployments served behind TLS.
func SecurityHeadersMiddlewareForEnvironment(next http.Handler, production bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'self';")
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if production {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}
		next.ServeHTTP(w, r)
	})
}

// TrustedRealIPMiddleware resolves the client address only when the direct
// peer is a configured trusted proxy. It prevents clients from spoofing the
// address used by rate limiting and audit logging.
func TrustedRealIPMiddleware(trustedCIDRs []string) func(http.Handler) http.Handler {
	trusted := make([]*net.IPNet, 0, len(trustedCIDRs))
	for _, cidr := range trustedCIDRs {
		if _, network, err := net.ParseCIDR(strings.TrimSpace(cidr)); err == nil {
			trusted = append(trusted, network)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer := remoteIP(r.RemoteAddr)
			if peer != nil && isTrustedProxy(peer, trusted) {
				if client := forwardedClientIP(r.Header.Get("X-Forwarded-For")); client != nil {
					r.RemoteAddr = client.String()
				} else if client := parseIP(r.Header.Get("X-Real-IP")); client != nil {
					r.RemoteAddr = client.String()
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func remoteIP(address string) net.IP {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return parseIP(host)
	}
	return parseIP(address)
}

func parseIP(value string) net.IP {
	return net.ParseIP(strings.TrimSpace(value))
}

func isTrustedProxy(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func forwardedClientIP(value string) net.IP {
	parts := strings.Split(value, ",")
	for index := len(parts) - 1; index >= 0; index-- {
		if ip := parseIP(parts[index]); ip != nil {
			return ip
		}
	}
	return nil
}

// MaxBodySizeMiddleware rejects requests exceeding maxBytes to prevent memory exhaustion DoS.
func MaxBodySizeMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "request body entity too large",
				})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// MaxBodySizeMiddlewareByPath applies a request body limit selected from the request path.
// This allows large streaming uploads without increasing limits for unrelated endpoints.
func MaxBodySizeMiddlewareByPath(defaultMaxBytes int64, pathLimit func(string) int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			maxBytes := defaultMaxBytes
			if pathLimit != nil {
				if selected := pathLimit(r.URL.Path); selected > 0 {
					maxBytes = selected
				}
			}
			if r.ContentLength > maxBytes {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "request body entity too large",
				})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
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

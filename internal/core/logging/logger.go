package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// ContextKey is a custom type for context keys to avoid collisions.
type ContextKey string

const (
	RequestIDKey ContextKey = "request_id"
	TenantIDKey  ContextKey = "tenant_id"
	UserIDKey    ContextKey = "user_id"
)

// Config defines options for creating a logger.
type Config struct {
	Level  string `yaml:"level"`  // "debug", "info", "warn", "error"
	Format string `yaml:"format"` // "json", "text"
}

// ContextHandler wraps a slog.Handler to inject contextual fields.
type ContextHandler struct {
	slog.Handler
}

// Handle extracts context values and appends them as structured attributes.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
			r.AddAttrs(slog.String("request_id", reqID))
		}
		if tenantID, ok := ctx.Value(TenantIDKey).(string); ok && tenantID != "" {
			r.AddAttrs(slog.String("tenant_id", tenantID))
		}
		if userID, ok := ctx.Value(UserIDKey).(string); ok && userID != "" {
			r.AddAttrs(slog.String("user_id", userID))
		}
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a new ContextHandler with the given attributes added.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a new ContextHandler with the given group name.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithGroup(name)}
}

// ParseLevel converts a string representation of a log level to slog.Level.
func ParseLevel(lvl string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger creates a new configured slog.Logger.
func NewLogger(cfg Config, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level: ParseLevel(cfg.Level),
	}

	var baseHandler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		baseHandler = slog.NewJSONHandler(w, opts)
	} else {
		baseHandler = slog.NewTextHandler(w, opts)
	}

	handler := &ContextHandler{Handler: baseHandler}
	return slog.New(handler)
}

// WithContext returns a context with logger-relevant values attached.
func WithContext(ctx context.Context, requestID, tenantID, userID string) context.Context {
	if requestID != "" {
		ctx = context.WithValue(ctx, RequestIDKey, requestID)
	}
	if tenantID != "" {
		ctx = context.WithValue(ctx, TenantIDKey, tenantID)
	}
	if userID != "" {
		ctx = context.WithValue(ctx, UserIDKey, userID)
	}
	return ctx
}


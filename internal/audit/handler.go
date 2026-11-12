package audit

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
)

// HTTPHandler exposes audit log query and export endpoints over HTTP.
type HTTPHandler struct {
	svc *Service
}

// NewHTTPHandler creates a new audit HTTPHandler.
func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// RegisterRoutes registers audit endpoints on the Chi router.
func (h *HTTPHandler) RegisterRoutes(r chi.Router) {
	r.Route("/audit-logs", func(logs chi.Router) {
		logs.Use(auth.RequireAuth)
		logs.With(auth.RequirePermission("read", "system:audit_logs")).Get("/", h.handleListAuditLogs)
		logs.With(auth.RequirePermission("read", "system:audit_logs")).Get("/export", h.handleExportCSV)
	})
}

func parseAuditFilter(r *http.Request, defaultLimit, maxLimit int) Filter {
	q := r.URL.Query()
	limit := defaultLimit
	offset := 0

	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if maxLimit > 0 && limit > maxLimit {
		limit = maxLimit
	}
	if o, err := strconv.Atoi(q.Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	var fromTime, toTime *time.Time
	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		if t, ok := parseFlexibleTime(raw, false); ok {
			fromTime = &t
		}
	}
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		if t, ok := parseFlexibleTime(raw, true); ok {
			toTime = &t
		}
	}

	return Filter{
		ActorID:  strings.TrimSpace(q.Get("actor_id")),
		Action:   strings.TrimSpace(q.Get("action")),
		Resource: strings.TrimSpace(q.Get("resource")),
		Result:   strings.TrimSpace(q.Get("result")),
		From:     fromTime,
		To:       toTime,
		Limit:    limit,
		Offset:   offset,
	}
}

func parseFlexibleTime(raw string, endOfDay bool) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		if endOfDay {
			t = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second + 999*time.Millisecond)
		}
		return t.UTC(), true
	}
	return time.Time{}, false
}

func (h *HTTPHandler) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	filter := parseAuditFilter(r, 50, 200)

	logs, total, err := h.svc.List(r.Context(), filter)
	if err != nil {
		http.Error(w, `{"error":"failed to retrieve audit logs"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": logs,
		"meta": map[string]any{
			"total":  total,
			"limit":  filter.Limit,
			"offset": filter.Offset,
		},
	})
}

func (h *HTTPHandler) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	filter := parseAuditFilter(r, 5000, 5000)
	filter.Offset = 0

	logs, _, err := h.svc.List(r.Context(), filter)
	if err != nil {
		http.Error(w, `{"error":"failed to export audit logs"}`, http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"id", "timestamp", "actor_id", "actor_type", "action", "resource", "target_id", "result", "ip_address", "user_agent", "metadata"})

	for _, l := range logs {
		metaStr := ""
		if len(l.Metadata) > 0 {
			if b, err := json.Marshal(RedactMetadata(l.Metadata)); err == nil {
				metaStr = string(b)
			}
		}
		_ = cw.Write([]string{
			sanitizeCSVCell(l.ID),
			l.Timestamp.UTC().Format(time.RFC3339),
			sanitizeCSVCell(l.ActorID),
			sanitizeCSVCell(l.ActorType),
			sanitizeCSVCell(l.Action),
			sanitizeCSVCell(l.Resource),
			sanitizeCSVCell(l.TargetID),
			sanitizeCSVCell(l.Result),
			sanitizeCSVCell(l.IPAddress),
			sanitizeCSVCell(l.UserAgent),
			sanitizeCSVCell(metaStr),
		})
	}
	cw.Flush()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-logs.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// sanitizeCSVCell guards against spreadsheet formula injection (=, +, -, @, tab, cr).
func sanitizeCSVCell(s string) string {
	if s == "" {
		return ""
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	default:
		return s
	}
}

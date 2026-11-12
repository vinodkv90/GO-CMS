package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_RecordAndFilter(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()

	// 1. Record 3 events
	_, err := svc.Record(ctx, RecordParams{
		ActorID:   "user-1",
		ActorType: "user",
		Action:    "create",
		Resource:  "content:articles",
		TargetID:  "art-100",
		Metadata:  map[string]any{"title": "Go CMS 1.0"},
	})
	require.NoError(t, err)

	_, err = svc.Record(ctx, RecordParams{
		ActorID:   "user-1",
		ActorType: "user",
		Action:    "publish",
		Resource:  "content:articles",
		TargetID:  "art-100",
	})
	require.NoError(t, err)

	_, err = svc.Record(ctx, RecordParams{
		ActorID:   "user-2",
		ActorType: "user",
		Action:    "upload",
		Resource:  "media:asset",
		TargetID:  "img-200",
	})
	require.NoError(t, err)

	// 2. Query all
	all, total, err := svc.List(ctx, Filter{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, all, 3)

	// 3. Filter by Action: "publish"
	pubLogs, total, err := svc.List(ctx, Filter{Action: "publish"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "art-100", pubLogs[0].TargetID)

	// 4. Filter by Actor: "user-2"
	user2Logs, total, err := svc.List(ctx, Filter{ActorID: "user-2"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "media:asset", user2Logs[0].Resource)

	// 5. Pagination
	page1, total, err := svc.List(ctx, Filter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, page1, 2)
}

func TestHTTPHandler_AuditLogsEndpoint(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()
	_, _ = svc.Record(ctx, RecordParams{
		ActorID:   "admin",
		ActorType: "user",
		Action:    "login",
		Resource:  "auth:session",
	})

	handler := NewHTTPHandler(svc)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			adminUser := &auth.User{
				ID:       "admin-1",
				Username: "admin",
				Status:   auth.StatusActive,
				Roles: []auth.Role{
					{
						Slug: auth.RoleSuperAdmin,
					},
				},
			}
			ctx := context.WithValue(req.Context(), auth.UserContextKey, adminUser)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/audit-logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data := resp["data"].([]any)
	assert.Len(t, data, 1)

	meta := resp["meta"].(map[string]any)
	assert.Equal(t, float64(1), meta["total"])
}

func TestService_RedactMetadataAndDateResultFilters(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()

	rec, err := svc.Record(ctx, RecordParams{
		ActorID:   "admin-1",
		ActorType: "admin",
		Action:    "settings.update",
		Resource:  "settings:email",
		Result:    "success",
		Metadata: map[string]any{
			"smtp_host":     "smtp.example.com",
			"smtp_password": "super-secret-password",
			"nested": map[string]any{
				"client_secret": "oidc-secret-val",
				"client_id":     "public-client-id",
				"access_token":  "tok-123",
			},
			"items": []any{
				map[string]any{"api_key": "ak_live_999", "label": "primary"},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "[REDACTED]", rec.Metadata["smtp_password"])
	assert.Equal(t, "smtp.example.com", rec.Metadata["smtp_host"])

	nested := rec.Metadata["nested"].(map[string]any)
	assert.Equal(t, "[REDACTED]", nested["client_secret"])
	assert.Equal(t, "[REDACTED]", nested["access_token"])
	assert.Equal(t, "public-client-id", nested["client_id"])

	items := rec.Metadata["items"].([]any)
	firstItem := items[0].(map[string]any)
	assert.Equal(t, "[REDACTED]", firstItem["api_key"])
	assert.Equal(t, "primary", firstItem["label"])

	// Record a failed auth event
	_, err = svc.Record(ctx, RecordParams{
		ActorID:   "admin-2",
		ActorType: "admin",
		Action:    "auth.login",
		Resource:  "auth:session",
		Result:    "failure",
	})
	require.NoError(t, err)

	// Filter by Result
	fails, total, err := svc.List(ctx, Filter{Result: "failure"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "admin-2", fails[0].ActorID)

	// Filter by Future From returns 0
	future := time.Now().UTC().Add(time.Hour)
	_, total, err = svc.List(ctx, Filter{From: &future})
	require.NoError(t, err)
	assert.Equal(t, 0, total)

	// Filter by Past From + Future To returns 2
	past := time.Now().UTC().Add(-time.Hour)
	toFuture := time.Now().UTC().Add(time.Hour)
	_, total, err = svc.List(ctx, Filter{From: &past, To: &toFuture})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
}

func TestHTTPHandler_ExportCSVAndRBAC(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()
	_, err := svc.Record(ctx, RecordParams{
		ActorID:   "user-csv",
		ActorType: "admin",
		Action:    "=CMD()",
		Resource:  "+content:articles",
		TargetID:  "@id1",
		IPAddress: "127.0.0.1",
		Result:    "success",
		Metadata:  map[string]any{"secret": "leaked"},
	})
	require.NoError(t, err)

	handler := NewHTTPHandler(svc)

	// 1. Unauthenticated -> 401
	unauthR := chi.NewRouter()
	handler.RegisterRoutes(unauthR)
	w := httptest.NewRecorder()
	unauthR.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/audit-logs", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	unauthR.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/audit-logs/export", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 2. Non-privileged user -> 403
	editorR := chi.NewRouter()
	editorR.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			u := &auth.User{
				ID:     "ed-1",
				Status: auth.StatusActive,
				Roles:  []auth.Role{{Slug: "editor", Permissions: []auth.Permission{{Action: "read", Resource: "content:articles"}}}},
			}
			next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, u)))
		})
	})
	handler.RegisterRoutes(editorR)

	w = httptest.NewRecorder()
	editorR.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/audit-logs/export", nil))
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 3. SuperAdmin -> 200 CSV with formula-injection protection and redacted metadata
	adminR := chi.NewRouter()
	adminR.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			u := &auth.User{
				ID:     "admin-1",
				Status: auth.StatusActive,
				Roles:  []auth.Role{{Slug: auth.RoleSuperAdmin}},
			}
			next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, u)))
		})
	})
	handler.RegisterRoutes(adminR)

	w = httptest.NewRecorder()
	adminR.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/audit-logs/export?result=success", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")
	assert.Contains(t, w.Header().Get("Content-Disposition"), "audit-logs.csv")
	assert.NotContains(t, w.Body.String(), "leaked")

	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2) // Header + 1 row
	assert.Equal(t, "id", rows[0][0])
	assert.Equal(t, "'=CMD()", rows[1][4])
	assert.Equal(t, "'+content:articles", rows[1][5])
	assert.Equal(t, "'@id1", rows[1][6])
}

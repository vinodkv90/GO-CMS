package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

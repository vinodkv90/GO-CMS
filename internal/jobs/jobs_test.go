package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerPool_SuccessExecution(t *testing.T) {
	q := NewQueue()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := NewWorkerPool(q, 2, 10*time.Millisecond, logger)

	var executedCount atomic.Int32
	pool.RegisterHandler("test:simple", func(ctx context.Context, job *Job) error {
		var payload map[string]string
		_ = json.Unmarshal(job.Payload, &payload)
		assert.Equal(t, "bar", payload["foo"])
		executedCount.Add(1)
		return nil
	})

	pool.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = pool.Stop(ctx)
	}()

	job, err := NewJob("test:simple", map[string]string{"foo": "bar"}, time.Time{}, 3)
	require.NoError(t, err)

	err = q.Enqueue(job)
	require.NoError(t, err)

	// Wait for execution
	require.Eventually(t, func() bool {
		return executedCount.Load() == 1
	}, 1*time.Second, 20*time.Millisecond)

	// Check job status in queue
	stored, ok := q.Get(job.ID)
	require.True(t, ok)
	assert.Equal(t, StatusCompleted, stored.Status)
	assert.NotNil(t, stored.CompletedAt)
	assert.Equal(t, 1, stored.Attempts)
	assert.Empty(t, stored.Error)
}

func TestWorkerPool_RetryAndExhaustion(t *testing.T) {
	q := NewQueue()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Fast poll for testing
	pool := NewWorkerPool(q, 1, 10*time.Millisecond, logger)

	var attempts atomic.Int32
	pool.RegisterHandler("test:failing", func(ctx context.Context, job *Job) error {
		attempts.Add(1)
		return errors.New("simulated network failure")
	})

	pool.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = pool.Stop(ctx)
	}()

	// Max 2 retries
	job, err := NewJob("test:failing", map[string]string{"data": "xyz"}, time.Time{}, 2)
	require.NoError(t, err)

	err = q.Enqueue(job)
	require.NoError(t, err)

	// Wait until permanently failed
	require.Eventually(t, func() bool {
		stored, ok := q.Get(job.ID)
		return ok && stored.Status == StatusFailed
	}, 2*time.Second, 50*time.Millisecond)

	stored, ok := q.Get(job.ID)
	require.True(t, ok)
	assert.Equal(t, StatusFailed, stored.Status)
	assert.Equal(t, 2, stored.Attempts)
	assert.Contains(t, stored.Error, "simulated network failure")
}

func TestWorkerPool_UnregisteredHandler(t *testing.T) {
	q := NewQueue()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := NewWorkerPool(q, 1, 10*time.Millisecond, logger)

	pool.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = pool.Stop(ctx)
	}()

	job, err := NewJob("unknown:type", map[string]string{}, time.Time{}, 1)
	require.NoError(t, err)

	err = q.Enqueue(job)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		stored, ok := q.Get(job.ID)
		return ok && stored.Status == StatusFailed
	}, 1*time.Second, 20*time.Millisecond)

	stored, _ := q.Get(job.ID)
	assert.Contains(t, stored.Error, "unregistered job handler")
}

func TestHTTPHandler_Endpoints(t *testing.T) {
	q := NewQueue()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := NewWorkerPool(q, 1, 100*time.Millisecond, logger)
	handler := NewHTTPHandler(pool)

	r := chi.NewRouter()
	// Mock authentication context with super admin
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			adminUser := &auth.User{
				ID:       "admin-1",
				Username: "admin",
				Status:   auth.StatusActive,
				Roles: []auth.Role{
					{
						Slug: auth.RoleSuperAdmin,
						Permissions: []auth.Permission{
							{Action: "*", Resource: "*"},
						},
					},
				},
			}
			ctx := context.WithValue(req.Context(), auth.UserContextKey, adminUser)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	handler.RegisterRoutes(r)

	// 1. Enqueue job
	job, err := NewJob("test:job", map[string]string{"foo": "bar"}, time.Time{}, 3)
	require.NoError(t, err)
	_ = q.Enqueue(job)

	// 2. Test GET /jobs
	req := httptest.NewRequest("GET", "/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &listResp)
	require.NoError(t, err)
	data := listResp["data"].([]any)
	assert.Len(t, data, 1)

	// 3. Test GET /jobs/stats
	req = httptest.NewRequest("GET", "/jobs/stats", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 4. Test GET /jobs/{id}
	req = httptest.NewRequest("GET", "/jobs/"+job.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 5. Test POST /jobs/{id}/retry
	req = httptest.NewRequest("POST", "/jobs/"+job.ID+"/retry", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

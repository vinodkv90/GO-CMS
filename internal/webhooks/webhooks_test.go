package webhooks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/settings"
	"github.com/go-cms/cms/internal/webhooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestWebhookService() *webhooks.Service {
	svc := webhooks.NewService(
		webhooks.NewMemoryRepository(),
		func() string { return "unit-test-encryption-key-material-32+" },
		nil,
		nil,
	)
	return svc
}

func TestValidateWebhookURL_SSRFProtection(t *testing.T) {
	badURLs := []string{
		"http://127.0.0.1:8080/hook",
		"http://localhost:9000/hook",
		"http://169.254.169.254/latest/meta-data/",
		"https://10.0.0.5/internal",
		"https://172.16.0.1/hook",
		"https://192.168.1.50/hook",
		"https://100.64.0.1/cgnat",
		"https://[::1]/loopback",
		"https://metadata.internal/secret",
		"ftp://example.com/hook",
		"not-a-url",
	}
	for _, raw := range badURLs {
		err := webhooks.ValidateWebhookURL(raw)
		assert.ErrorIs(t, err, webhooks.ErrUnsafeWebhookURL, "expected SSRF rejection for %s", raw)
	}

	assert.NoError(t, webhooks.ValidateWebhookURL("https://hooks.example.com/cms-events"))
}

func TestService_CRUD_SecretPreservationAndTriggerHMAC(t *testing.T) {
	ctx := context.Background()
	var gotSig, gotEvent, gotCustomHeader string
	var gotBody []byte

	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-CMS-Webhook-Signature")
		gotEvent = r.Header.Get("X-CMS-Webhook-Event")
		gotCustomHeader = r.Header.Get("X-Custom-Token")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	defer targetSrv.Close()

	svc := newTestWebhookService()
	svc.SetAllowUnsafeTestURLs(true)

	// 1. Create with secret
	created, err := svc.Create(ctx, webhooks.CreateWebhookRequest{
		Name:    "Build Trigger",
		URL:     targetSrv.URL,
		Headers: map[string]string{"X-Custom-Token": "abc-123"},
		Events:  []string{"entry.publish", "entry.update"},
		Secret:  "super-hmac-secret",
	}, "admin-1")
	require.NoError(t, err)
	assert.True(t, created.SecretConfigured)
	assert.Equal(t, settings.SecretPreserveSentinel, created.Secret)
	assert.NotContains(t, created.Secret, "super-hmac-secret")

	// 2. Preserve secret on update using sentinel
	newName := "Build Trigger Renamed"
	sentinel := settings.SecretPreserveSentinel
	updated, err := svc.Update(ctx, created.ID, webhooks.UpdateWebhookRequest{
		Name:   &newName,
		Secret: &sentinel,
	}, "admin-1")
	require.NoError(t, err)
	assert.Equal(t, "Build Trigger Renamed", updated.Name)
	assert.True(t, updated.SecretConfigured)
	assert.Equal(t, settings.SecretPreserveSentinel, updated.Secret)

	// 3. TriggerTest validates HMAC signature and persists delivery
	delivery, err := svc.TriggerTest(ctx, created.ID, "admin-1")
	require.NoError(t, err)
	assert.True(t, delivery.Success)
	assert.Equal(t, http.StatusAccepted, delivery.StatusCode)
	assert.Equal(t, `{"received":true}`, delivery.ResponseSnippet)
	assert.Equal(t, "webhook.test", gotEvent)
	assert.Equal(t, "abc-123", gotCustomHeader)
	assert.Equal(t, webhooks.ComputeHMACSignature("super-hmac-secret", gotBody), gotSig)

	deliveries, total, err := svc.ListDeliveries(ctx, created.ID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, deliveries, 1)
	assert.Equal(t, delivery.ID, deliveries[0].ID)

	// 4. Clearing secret explicitly removes it
	empty := ""
	cleared, err := svc.Update(ctx, created.ID, webhooks.UpdateWebhookRequest{Secret: &empty}, "admin-1")
	require.NoError(t, err)
	assert.False(t, cleared.SecretConfigured)
	assert.Equal(t, "", cleared.Secret)
}

func TestService_EventBusDispatchWithRetries(t *testing.T) {
	ctx := context.Background()
	var attempts int32

	flakySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("temporary outage"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer flakySrv.Close()

	svc := newTestWebhookService()
	svc.SetAllowUnsafeTestURLs(true)

	maxRetries := 3
	wh, err := svc.Create(ctx, webhooks.CreateWebhookRequest{
		Name:       "Retry Subscriber",
		URL:        flakySrv.URL,
		Events:     []string{"entry.publish"},
		MaxRetries: &maxRetries,
	}, "admin-1")
	require.NoError(t, err)

	bus := content.NewEventBus()
	svc.RegisterContentBus(bus)

	require.NoError(t, bus.Emit(ctx, content.EventAfterPublish, &content.ContentEntry{
		ID:            "ent-42",
		ContentTypeID: "articles",
		Slug:          "hello-world",
		Status:        content.StatusPublished,
		DraftVersion:  2,
	}))

	svc.Wait()
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))

	logs, total, err := svc.ListDeliveries(ctx, wh.ID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, logs, 3)
	// Newest delivery (attempt 3) succeeded
	assert.True(t, logs[0].Success)
	assert.Equal(t, http.StatusOK, logs[0].StatusCode)
	assert.Equal(t, 3, logs[0].Attempt)
}

func TestHandler_RBACEnforcementAndEndpoints(t *testing.T) {
	svc := newTestWebhookService()
	handler := webhooks.NewHandler(svc)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// 1. Reader without permissions -> 403
	viewer := &auth.User{
		ID:     "usr-viewer",
		Status: auth.StatusActive,
		Roles: []auth.Role{
			{Slug: auth.RoleViewer, Permissions: []auth.Permission{{Action: "read", Resource: "content"}}},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, viewer))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 2. Admin with webhooks permissions -> create & list & SSRF block
	admin := &auth.User{
		ID:     "usr-admin",
		Status: auth.StatusActive,
		Roles:  []auth.Role{{Slug: auth.RoleSuperAdmin}},
	}

	ssrfPayload := []byte(`{"name":"Local","url":"http://127.0.0.1:8080/internal","events":["entry.create"]}`)
	req = httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(ssrfPayload))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "unsafe webhook")

	validPayload := []byte(`{"name":"Prod Hook","url":"https://hooks.example.com/events","events":["entry.create"],"secret":"mysecret"}`)
	req = httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(validPayload))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	assert.NotContains(t, w.Body.String(), "mysecret")
	assert.Contains(t, w.Body.String(), settings.SecretPreserveSentinel)

	var resp struct {
		Data webhooks.Webhook `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Data.ID)
	assert.True(t, resp.Data.SecretConfigured)

	// Events catalog
	req = httptest.NewRequest(http.MethodGet, "/webhooks/events", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "entry.publish")
}


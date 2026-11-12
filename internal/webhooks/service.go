package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-cms/cms/internal/audit"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/settings"
	"github.com/google/uuid"
)

var (
	// ErrInvalidWebhookInput indicates a bad webhook create/update payload.
	ErrInvalidWebhookInput = errors.New("invalid webhook configuration")
)

var supportedEventsMap = func() map[string]bool {
	m := make(map[string]bool, len(SupportedEvents)+1)
	for _, e := range SupportedEvents {
		m[e] = true
	}
	m["*"] = true
	return m
}()

// Service coordinates webhook subscriptions, HMAC signing, EventBus hooks, and delivery retries.
type Service struct {
	repo                Repository
	encKeyFn            func() string
	auditSvc            *audit.Service
	logger              *slog.Logger
	httpClient          *http.Client
	allowUnsafeTestURLs bool
	retryBaseDelay      time.Duration
	wg                  sync.WaitGroup
}

// NewService constructs a webhook service.
func NewService(
	repo Repository,
	encKeyFn func() string,
	auditSvc *audit.Service,
	logger *slog.Logger,
) *Service {
	if repo == nil {
		repo = NewMemoryRepository()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:           repo,
		encKeyFn:       encKeyFn,
		auditSvc:       auditSvc,
		logger:         logger,
		httpClient:     newSSRFSafeHTTPClient(false),
		retryBaseDelay: 250 * time.Millisecond,
	}
}

// SetAllowUnsafeTestURLs relaxes loopback SSRF checks strictly for in-process httptest unit tests.
func (s *Service) SetAllowUnsafeTestURLs(allow bool) {
	s.allowUnsafeTestURLs = allow
	s.httpClient = newSSRFSafeHTTPClient(allow)
	if allow {
		s.retryBaseDelay = 10 * time.Millisecond
	} else {
		s.retryBaseDelay = 250 * time.Millisecond
	}
}

func (s *Service) encryptionKey() string {
	if s.encKeyFn != nil {
		return s.encKeyFn()
	}
	return ""
}

func sanitizeWebhookForOutput(wh *Webhook) *Webhook {
	cp := cloneWebhook(wh)
	cp.SecretConfigured = cp.SecretEnc != ""
	cp.SecretEnc = ""
	if cp.SecretConfigured {
		cp.Secret = settings.SecretPreserveSentinel
	} else {
		cp.Secret = ""
	}
	return cp
}

func normalizeEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return nil, errors.New("at least one webhook event must be selected")
	}
	seen := make(map[string]bool, len(events))
	out := make([]string, 0, len(events))
	for _, raw := range events {
		ev := strings.TrimSpace(raw)
		if ev == "" || seen[ev] {
			continue
		}
		if !supportedEventsMap[ev] {
			return nil, errors.New("unsupported webhook event: " + ev)
		}
		seen[ev] = true
		out = append(out, ev)
	}
	if len(out) == 0 {
		return nil, errors.New("at least one valid webhook event must be selected")
	}
	return out, nil
}

func normalizeHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range headers {
		cleanKey := strings.TrimSpace(k)
		if cleanKey == "" {
			continue
		}
		out[cleanKey] = strings.TrimSpace(v)
	}
	return out
}

func clampRetries(r int) int {
	if r < 0 {
		return 0
	}
	if r > 5 {
		return 5
	}
	return r
}

func (s *Service) List(ctx context.Context) ([]*Webhook, error) {
	items, err := s.repo.ListWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Webhook, 0, len(items))
	for _, wh := range items {
		out = append(out, sanitizeWebhookForOutput(wh))
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (*Webhook, error) {
	wh, err := s.repo.GetWebhook(ctx, id)
	if err != nil {
		return nil, err
	}
	return sanitizeWebhookForOutput(wh), nil
}

func (s *Service) Create(ctx context.Context, req CreateWebhookRequest, actorID string) (*Webhook, error) {
	name := strings.TrimSpace(req.Name)
	targetURL := strings.TrimSpace(req.URL)
	if name == "" {
		return nil, errors.New("webhook name is required")
	}
	if !s.allowUnsafeTestURLs {
		if err := ValidateWebhookURL(targetURL); err != nil {
			return nil, err
		}
	}
	events, err := normalizeEvents(req.Events)
	if err != nil {
		return nil, err
	}
	isEnabled := true
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}
	maxRetries := 3
	if req.MaxRetries != nil {
		maxRetries = clampRetries(*req.MaxRetries)
	}

	var secretEnc string
	rawSecret := strings.TrimSpace(req.Secret)
	if rawSecret != "" && rawSecret != settings.SecretPreserveSentinel {
		enc, err := settings.EncryptSecret(s.encryptionKey(), rawSecret)
		if err != nil {
			return nil, err
		}
		secretEnc = enc
	}

	now := time.Now().UTC()
	wh := &Webhook{
		ID:         uuid.NewString(),
		Name:       name,
		URL:        targetURL,
		Headers:    normalizeHeaders(req.Headers),
		Events:     events,
		SecretEnc:  secretEnc,
		IsEnabled:  isEnabled,
		MaxRetries: maxRetries,
		CreatedBy:  actorID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repo.CreateWebhook(ctx, wh); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, actorID, "webhook.create", wh.ID, map[string]any{"name": wh.Name, "events": wh.Events})
	return sanitizeWebhookForOutput(wh), nil
}

func (s *Service) Update(ctx context.Context, id string, req UpdateWebhookRequest, actorID string) (*Webhook, error) {
	existing, err := s.repo.GetWebhook(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, errors.New("webhook name cannot be empty")
		}
		existing.Name = name
	}
	if req.URL != nil {
		targetURL := strings.TrimSpace(*req.URL)
		if !s.allowUnsafeTestURLs {
			if err := ValidateWebhookURL(targetURL); err != nil {
				return nil, err
			}
		}
		existing.URL = targetURL
	}
	if req.Headers != nil {
		existing.Headers = normalizeHeaders(req.Headers)
	}
	if req.Events != nil {
		events, err := normalizeEvents(req.Events)
		if err != nil {
			return nil, err
		}
		existing.Events = events
	}
	if req.IsEnabled != nil {
		existing.IsEnabled = *req.IsEnabled
	}
	if req.MaxRetries != nil {
		existing.MaxRetries = clampRetries(*req.MaxRetries)
	}
	if req.Secret != nil {
		incoming := strings.TrimSpace(*req.Secret)
		if incoming == "" {
			existing.SecretEnc = ""
		} else if incoming != settings.SecretPreserveSentinel {
			enc, err := settings.EncryptSecret(s.encryptionKey(), incoming)
			if err != nil {
				return nil, err
			}
			existing.SecretEnc = enc
		}
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateWebhook(ctx, existing); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, actorID, "webhook.update", existing.ID, map[string]any{"name": existing.Name, "enabled": existing.IsEnabled})
	return sanitizeWebhookForOutput(existing), nil
}

func (s *Service) Delete(ctx context.Context, id, actorID string) error {
	if err := s.repo.DeleteWebhook(ctx, id); err != nil {
		return err
	}
	s.recordAudit(ctx, actorID, "webhook.delete", id, nil)
	return nil
}

func (s *Service) ListDeliveries(ctx context.Context, webhookID string, limit, offset int) ([]*WebhookDelivery, int, error) {
	if _, err := s.repo.GetWebhook(ctx, webhookID); err != nil {
		return nil, 0, err
	}
	return s.repo.ListDeliveries(ctx, webhookID, limit, offset)
}

// TriggerTest sends a single synchronous test payload to verify the webhook endpoint and signing configuration.
func (s *Service) TriggerTest(ctx context.Context, webhookID, actorID string) (*WebhookDelivery, error) {
	wh, err := s.repo.GetWebhook(ctx, webhookID)
	if err != nil {
		return nil, err
	}
	if !s.allowUnsafeTestURLs {
		if err := ValidateWebhookURL(wh.URL); err != nil {
			return nil, err
		}
	}
	payloadObj := map[string]any{
		"event":     "webhook.test",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"webhook": map[string]any{
			"id":   wh.ID,
			"name": wh.Name,
		},
		"data": map[string]any{
			"message": "Test delivery triggered from Go CMS administration panel.",
		},
	}
	delivery, err := s.deliverOnce(ctx, wh, "webhook.test", payloadObj, 1)
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, actorID, "webhook.trigger", wh.ID, map[string]any{"status_code": delivery.StatusCode, "success": delivery.Success})
	return delivery, nil
}

// RegisterContentBus subscribes the webhook dispatcher to core content lifecycle events.
func (s *Service) RegisterContentBus(bus *content.EventBus) {
	if bus == nil {
		return
	}
	eventMap := map[content.LifecycleEvent]string{
		content.EventAfterCreate:  "entry.create",
		content.EventAfterUpdate:  "entry.update",
		content.EventAfterDelete:  "entry.delete",
		content.EventAfterPublish: "entry.publish",
	}
	for lifecycleEv, webhookEv := range eventMap {
		evName := webhookEv
		bus.Subscribe(lifecycleEv, func(ctx context.Context, _ content.LifecycleEvent, entry *content.ContentEntry) error {
			if entry == nil {
				return nil
			}
			s.EmitEvent(ctx, evName, map[string]any{
				"id":              entry.ID,
				"content_type_id": entry.ContentTypeID,
				"slug":            entry.Slug,
				"status":          entry.Status,
				"draft_version":   entry.DraftVersion,
				"updated_at":      entry.UpdatedAt,
			})
			return nil
		})
	}
}

// EmitEvent dispatches matching enabled webhooks asynchronously with retry/backoff.
func (s *Service) EmitEvent(_ context.Context, eventName string, data any) {
	whs, err := s.repo.ListWebhooks(context.Background())
	if err != nil {
		s.logger.Warn("Failed to query webhooks for event dispatch", "event", eventName, "error", err)
		return
	}
	for _, wh := range whs {
		if !wh.IsEnabled || !webhookMatchesEvent(wh, eventName) {
			continue
		}
		target := cloneWebhook(wh)
		payloadObj := map[string]any{
			"event":     eventName,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"data":      data,
		}
		s.wg.Add(1)
		go func(w *Webhook, p map[string]any) {
			defer s.wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			s.deliverWithRetries(ctx, w, eventName, p)
		}(target, payloadObj)
	}
}

// Wait blocks until all in-flight asynchronous webhook deliveries complete (used in tests and shutdown).
func (s *Service) Wait() {
	s.wg.Wait()
}

func webhookMatchesEvent(wh *Webhook, eventName string) bool {
	for _, ev := range wh.Events {
		if ev == "*" || ev == eventName {
			return true
		}
	}
	return false
}

func (s *Service) deliverWithRetries(ctx context.Context, wh *Webhook, eventName string, payloadObj map[string]any) {
	if !s.allowUnsafeTestURLs {
		if err := ValidateWebhookURL(wh.URL); err != nil {
			_ = s.repo.CreateDelivery(ctx, &WebhookDelivery{
				ID:           uuid.NewString(),
				WebhookID:    wh.ID,
				Event:        eventName,
				RequestURL:   wh.URL,
				Attempt:      1,
				Success:      false,
				ErrorMessage: err.Error(),
				CreatedAt:    time.Now().UTC(),
			})
			return
		}
	}

	attempts := 1 + clampRetries(wh.MaxRetries)
	for attempt := 1; attempt <= attempts; attempt++ {
		d, err := s.deliverOnce(ctx, wh, eventName, payloadObj, attempt)
		if err == nil && d.Success {
			return
		}
		if attempt < attempts {
			backoff := s.retryBaseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
	}
}

func (s *Service) deliverOnce(ctx context.Context, wh *Webhook, eventName string, payloadObj map[string]any, attempt int) (*WebhookDelivery, error) {
	payloadBytes, err := json.Marshal(payloadObj)
	if err != nil {
		return nil, err
	}

	var signingSecret string
	if wh.SecretEnc != "" {
		dec, err := settings.DecryptSecret(s.encryptionKey(), wh.SecretEnc)
		if err != nil {
			s.logger.Warn("Failed to decrypt webhook signing secret", "webhook_id", wh.ID, "error", err)
		} else {
			signingSecret = dec
		}
	}

	deliveryID := uuid.NewString()
	statusCode, durationMs, success, errMsg, snippet := executeDeliveryAttempt(
		ctx, s.httpClient, wh, deliveryID, eventName, payloadBytes, signingSecret, attempt,
	)

	d := &WebhookDelivery{
		ID:              deliveryID,
		WebhookID:       wh.ID,
		Event:           eventName,
		RequestURL:      wh.URL,
		StatusCode:      statusCode,
		DurationMs:      durationMs,
		Attempt:         attempt,
		Success:         success,
		ErrorMessage:    errMsg,
		ResponseSnippet: snippet,
		CreatedAt:       time.Now().UTC(),
	}
	if saveErr := s.repo.CreateDelivery(ctx, d); saveErr != nil {
		s.logger.Warn("Failed to persist webhook delivery log", "webhook_id", wh.ID, "error", saveErr)
	}
	return d, nil
}

func (s *Service) recordAudit(ctx context.Context, actorID, action, targetID string, meta map[string]any) {
	if s.auditSvc == nil {
		return
	}
	_, _ = s.auditSvc.Record(ctx, audit.RecordParams{
		ActorID:   actorID,
		ActorType: "admin",
		Action:    action,
		Resource:  "webhooks",
		TargetID:  targetID,
		Metadata:  meta,
	})
}


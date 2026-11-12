package email_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/email"
	"github.com/go-cms/cms/internal/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSMTPServer struct {
	ln       net.Listener
	mu       sync.Mutex
	authLine string
	mailFrom string
	rcptTo   string
	data     string
	done     chan struct{}
}

func startMockSMTPServer(t *testing.T) *mockSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &mockSMTPServer{
		ln:   ln,
		done: make(chan struct{}),
	}

	go func() {
		defer close(srv.done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.handleConn(conn)
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		<-srv.done
	})
	return srv
}

func (s *mockSMTPServer) Port() int {
	return s.ln.Addr().(*net.TCPAddr).Port
}

func (s *mockSMTPServer) handleConn(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := func(line string) {
		_, _ = io.WriteString(conn, line+"\r\n")
	}

	w("220 mock-smtp.local ESMTP Ready")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			w("250-mock-smtp.local")
			w("250 AUTH PLAIN")
		case strings.HasPrefix(upper, "AUTH PLAIN "):
			s.mu.Lock()
			s.authLine = strings.TrimPrefix(line[len("AUTH PLAIN "):], "")
			s.mu.Unlock()
			w("235 2.7.0 Authentication successful")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.mu.Lock()
			s.mailFrom = strings.TrimSpace(line[len("MAIL FROM:"):])
			s.mu.Unlock()
			w("250 2.1.0 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.mu.Lock()
			s.rcptTo = strings.TrimSpace(line[len("RCPT TO:"):])
			s.mu.Unlock()
			w("250 2.1.5 OK")
		case upper == "DATA":
			w("354 Start mail input; end with <CRLF>.<CRLF>")
			var body strings.Builder
			for {
				dLine, err := r.ReadString('\n')
				if err != nil {
					return
				}
				trimmed := strings.TrimRight(dLine, "\r\n")
				if trimmed == "." {
					break
				}
				body.WriteString(trimmed)
				body.WriteString("\n")
			}
			s.mu.Lock()
			s.data = body.String()
			s.mu.Unlock()
			w("250 2.0.0 Queued")
		case upper == "QUIT":
			w("221 2.0.0 Bye")
			return
		default:
			w("250 OK")
		}
	}
}

func setupEmailTestEnv(t *testing.T) (*email.Service, *settings.Service) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Auth.SessionSecret = "email-test-encryption-key-012345"
	settingsSvc := settings.NewService(settings.NewMemoryRepository(), cfg, nil, settings.RuntimeStatusProvider{})
	emailSvc := email.NewService(settingsSvc, email.NewMemoryTemplateRepository(), nil)
	return emailSvc, settingsSvc
}

func TestEmailService_SettingsValidationAndSecretPreservation(t *testing.T) {
	svc, _ := setupEmailTestEnv(t)
	ctx := context.Background()

	// Seeded default templates exist
	list, err := svc.ListTemplates(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 5)

	// Header injection in FromName rejected
	_, err = svc.UpdateSettings(ctx, email.EmailSettings{
		Provider:  email.ProviderLog,
		FromName:  "Admin\r\nBcc: evil@attacker.com",
		FromEmail: "cms@example.org",
	}, settings.AuditMeta{})
	assert.Error(t, err)

	// Save SMTP settings and check secret encryption + masking
	saved, err := svc.UpdateSettings(ctx, email.EmailSettings{
		Provider:     email.ProviderSMTP,
		FromName:     "Go CMS",
		FromEmail:    "cms@example.org",
		SMTPHost:     "smtp.example.org",
		SMTPPort:     587,
		SMTPUsername: "smtp-user",
		SMTPPassword: "very-secret-smtp-password",
		Encryption:   email.EncryptionSTARTTLS,
	}, settings.AuditMeta{})
	require.NoError(t, err)
	assert.True(t, saved.PasswordConfigured)
	assert.Equal(t, settings.SecretPreserveSentinel, saved.SMTPPassword)
	assert.NotContains(t, saved.SMTPPassword, "very-secret-smtp-password")

	// Save again with sentinel keeps encrypted password intact
	updated, err := svc.UpdateSettings(ctx, email.EmailSettings{
		Provider:     email.ProviderSMTP,
		FromName:     "Go CMS Updated",
		FromEmail:    "cms@example.org",
		SMTPHost:     "smtp.example.org",
		SMTPPort:     587,
		SMTPUsername: "smtp-user",
		SMTPPassword: settings.SecretPreserveSentinel,
		Encryption:   email.EncryptionSTARTTLS,
	}, settings.AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, settings.SecretPreserveSentinel, updated.SMTPPassword)

	_, decPass, err := svc.ResolveRuntimeConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "very-secret-smtp-password", decPass)
}

func TestEmailService_RealSMTPMockHandshakeAndDelivery(t *testing.T) {
	smtpSrv := startMockSMTPServer(t)
	svc, _ := setupEmailTestEnv(t)
	ctx := context.Background()

	_, err := svc.UpdateSettings(ctx, email.EmailSettings{
		Provider:       email.ProviderSMTP,
		FromName:       "Go CMS Mailer",
		FromEmail:      "noreply@example.org",
		DefaultReplyTo: "support@example.org",
		SMTPHost:       "127.0.0.1",
		SMTPPort:       smtpSrv.Port(),
		SMTPUsername:   "cms-bot",
		SMTPPassword:   "bot-secret-pass",
		Encryption:     email.EncryptionNone,
		TimeoutSeconds: 5,
	}, settings.AuditMeta{})
	require.NoError(t, err)

	// 1. Test Connection performs real TCP handshake & PLAIN auth
	connRes, err := svc.TestConnection(ctx, nil)
	require.NoError(t, err)
	assert.True(t, connRes.OK, connRes.Message)

	smtpSrv.mu.Lock()
	expectedAuth := base64.StdEncoding.EncodeToString([]byte("\x00cms-bot\x00bot-secret-pass"))
	assert.Equal(t, expectedAuth, smtpSrv.authLine)
	smtpSrv.mu.Unlock()

	// 2. Send Admin Invitation email via real SMTP pipeline
	err = svc.SendAdminInvitation(ctx, "newadmin@example.org", "raw-invite-tok-123", "Vinod", 48)
	require.NoError(t, err)

	smtpSrv.mu.Lock()
	mailFrom := smtpSrv.mailFrom
	rcptTo := smtpSrv.rcptTo
	data := smtpSrv.data
	smtpSrv.mu.Unlock()

	assert.Equal(t, "<noreply@example.org>", mailFrom)
	assert.Equal(t, "<newadmin@example.org>", rcptTo)
	assert.Contains(t, data, "raw-invite-tok-123")
	assert.Contains(t, data, "invite_token=raw-invite-tok-123")
	assert.Contains(t, data, "Reply-To: support@example.org")
}

func TestEmailService_TemplateXSSSanitizationAndHeaderSafety(t *testing.T) {
	svc, _ := setupEmailTestEnv(t)
	ctx := context.Background()

	updated, err := svc.UpdateTemplate(ctx, "welcome", email.UpdateTemplateInput{
		Name:      "Welcome",
		Subject:   "Welcome to {{site_name}}",
		BodyHTML:  `<div><h1>Hi {{username}}</h1><script>alert("xss")</script><p onclick="steal()">Safe text</p><a href="javascript:evil()">bad</a></div>`,
		BodyText:  "Hi {{username}}",
		Enabled:   true,
	}, settings.AuditMeta{})
	require.NoError(t, err)

	assert.NotContains(t, strings.ToLower(updated.BodyHTML), "<script")
	assert.NotContains(t, strings.ToLower(updated.BodyHTML), "onclick")
	assert.NotContains(t, strings.ToLower(updated.BodyHTML), "javascript:")
	assert.Contains(t, updated.BodyHTML, "Safe text")

	// Render preview escapes HTML variables and strips CRLF in subject
	preview, err := svc.PreviewTemplate(ctx, "welcome", nil, map[string]string{
		"site_name": "Demo\r\nBcc: evil@example.com",
		"username":  `<img src=x onerror=alert(1)>`,
	})
	require.NoError(t, err)
	assert.NotContains(t, preview.Subject, "\r")
	assert.NotContains(t, preview.Subject, "\n")
	assert.NotContains(t, preview.BodyHTML, "<img")
	assert.Contains(t, preview.BodyHTML, "&lt;img src=x onerror=alert(1)&gt;")
}

func TestEmailHTTP_RBACAndEndpoints(t *testing.T) {
	svc, _ := setupEmailTestEnv(t)
	handler := email.NewHTTPHandler(svc)

	withUser := func(u *auth.User) chi.Router {
		r := chi.NewRouter()
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, u))
				next.ServeHTTP(w, req)
			})
		})
		handler.RegisterProtectedRoutes(r)
		return r
	}

	viewer := &auth.User{ID: "v1", Status: auth.StatusActive, Roles: []auth.Role{{Slug: auth.RoleViewer}}}
	admin := &auth.User{ID: "a1", Status: auth.StatusActive, Roles: []auth.Role{{Slug: auth.RoleSuperAdmin}}}

	viewerRouter := withUser(viewer)
	adminRouter := withUser(admin)

	// Viewer cannot access email settings or templates
	req := httptest.NewRequest(http.MethodGet, "/settings/email", nil)
	rec := httptest.NewRecorder()
	viewerRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Admin updates email to log mode & sends test email
	body, _ := json.Marshal(map[string]any{
		"to": "qa@example.org",
		"override": map[string]any{
			"provider":   email.ProviderLog,
			"from_name":  "QA CMS",
			"from_email": "qa-sender@example.org",
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/settings/email/test-send", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok":true`)

	// Preview endpoint works
	req = httptest.NewRequest(http.MethodPost, "/settings/email-templates/admin_invite/preview", bytes.NewReader([]byte(`{}`)))
	rec = httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "invite_token=")
	assert.Equal(t, fmt.Sprintf("%d", http.StatusOK), fmt.Sprintf("%d", rec.Code))
}

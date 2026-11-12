package email

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-cms/cms/internal/settings"
)

// Service orchestrates encrypted email settings, SMTP/Log transports, and transactional templates.
type Service struct {
	settingsSvc *settings.Service
	repo        TemplateRepository
	logger      *slog.Logger

	seedOnce sync.Once
}

// NewService constructs an email Service backed by settings.Service and a TemplateRepository.
func NewService(settingsSvc *settings.Service, repo TemplateRepository, logger *slog.Logger) *Service {
	if repo == nil {
		repo = NewMemoryTemplateRepository()
	}
	if logger == nil {
		logger = slog.Default()
	}
	svc := &Service{
		settingsSvc: settingsSvc,
		repo:        repo,
		logger:      logger,
	}
	svc.ensureSeeded(context.Background())
	return svc
}

func (s *Service) ensureSeeded(ctx context.Context) {
	s.seedOnce.Do(func() {
		if err := s.repo.SeedMissing(ctx, DefaultTemplates()); err != nil {
			s.logger.Warn("Failed to seed default email templates", "error", err)
		}
	})
}

func (s *Service) loadRawSettings(ctx context.Context) (*EmailSettings, error) {
	cfg := DefaultEmailSettings()
	if s.settingsSvc == nil {
		return &cfg, nil
	}
	updatedAt, found, err := s.settingsSvc.LoadJSONSetting(ctx, settings.KeyEmail, &cfg)
	if err != nil {
		return nil, err
	}
	if found {
		cfg.UpdatedAt = updatedAt
	}
	return &cfg, nil
}

// GetSettings returns sanitized email configuration (SMTP password masked via sentinel).
func (s *Service) GetSettings(ctx context.Context) (*EmailSettings, error) {
	raw, err := s.loadRawSettings(ctx)
	if err != nil {
		return nil, err
	}
	sanitized := raw.Sanitized()
	return &sanitized, nil
}

// ResolveRuntimeConfig loads current settings and decrypts any stored SMTP password.
func (s *Service) ResolveRuntimeConfig(ctx context.Context) (EmailSettings, string, error) {
	raw, err := s.loadRawSettings(ctx)
	if err != nil {
		return EmailSettings{}, "", err
	}
	password := ""
	if raw.SMTPPasswordEnc != "" && s.settingsSvc != nil {
		dec, err := settings.DecryptSecret(s.settingsSvc.EncryptionKey(), raw.SMTPPasswordEnc)
		if err != nil {
			return EmailSettings{}, "", fmt.Errorf("failed to decrypt stored SMTP password: %w", err)
		}
		password = dec
	}
	return raw.Sanitized(), password, nil
}

// UpdateSettings validates, encrypts SMTP password, persists, and returns sanitized EmailSettings.
func (s *Service) UpdateSettings(ctx context.Context, input EmailSettings, meta settings.AuditMeta) (*EmailSettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	existing, err := s.loadRawSettings(ctx)
	if err != nil {
		return nil, err
	}

	encKey := "default-cms-settings-key"
	if s.settingsSvc != nil {
		encKey = s.settingsSvc.EncryptionKey()
	}

	newEnc := ""
	switch input.SMTPPassword {
	case settings.SecretPreserveSentinel:
		newEnc = existing.SMTPPasswordEnc
	case "":
		newEnc = ""
	default:
		newEnc, err = settings.EncryptSecret(encKey, input.SMTPPassword)
		if err != nil {
			return nil, err
		}
	}

	toSave := input
	toSave.SMTPPassword = ""
	toSave.SMTPPasswordEnc = newEnc
	toSave.PasswordConfigured = newEnc != ""

	if s.settingsSvc != nil {
		updatedAt, err := s.settingsSvc.SaveJSONSetting(ctx, settings.KeyEmail, toSave, meta, "settings:email", map[string]any{
			"provider":            toSave.Provider,
			"smtp_host":           toSave.SMTPHost,
			"smtp_port":           toSave.SMTPPort,
			"encryption":          toSave.Encryption,
			"password_configured": toSave.PasswordConfigured,
		})
		if err != nil {
			return nil, err
		}
		toSave.UpdatedAt = updatedAt
	} else {
		toSave.UpdatedAt = time.Now().UTC()
	}

	sanitized := toSave.Sanitized()
	return &sanitized, nil
}

func (s *Service) resolveOverride(ctx context.Context, override *EmailSettings) (EmailSettings, string, error) {
	stored, storedPass, err := s.ResolveRuntimeConfig(ctx)
	if err != nil {
		return EmailSettings{}, "", err
	}
	if override == nil {
		return stored, storedPass, nil
	}
	target := *override
	if err := target.NormalizeAndValidate(); err != nil {
		return EmailSettings{}, "", err
	}
	pass := ""
	switch target.SMTPPassword {
	case settings.SecretPreserveSentinel:
		pass = storedPass
	case "":
		pass = ""
	default:
		pass = target.SMTPPassword
	}
	return target, pass, nil
}

// TestConnection tests connectivity and authentication against the configured (or overridden) transport.
func (s *Service) TestConnection(ctx context.Context, override *EmailSettings) (*TestResult, error) {
	cfg, pass, err := s.resolveOverride(ctx, override)
	if err != nil {
		return nil, err
	}
	mailer := BuildMailer(cfg, pass, s.logger)
	start := time.Now()
	err = mailer.CheckConnection(ctx)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &TestResult{
			OK:        false,
			Provider:  mailer.Name(),
			LatencyMs: latency,
			Message:   err.Error(),
		}, nil
	}
	msg := fmt.Sprintf("Email provider %q is ready.", mailer.Name())
	if mailer.Name() == ProviderSMTP {
		msg = fmt.Sprintf("Connected and authenticated with SMTP server %s:%d.", cfg.SMTPHost, cfg.SMTPPort)
	}
	return &TestResult{
		OK:        true,
		Provider:  mailer.Name(),
		LatencyMs: latency,
		Message:   msg,
	}, nil
}

// SendTestEmail dispatches a real test email to recipient using active or overridden settings.
func (s *Service) SendTestEmail(ctx context.Context, recipient string, override *EmailSettings) (*TestResult, error) {
	recipient = strings.TrimSpace(recipient)
	if HasHeaderInjection(recipient) {
		return nil, fmt.Errorf("invalid recipient address")
	}
	addr, err := mail.ParseAddress(recipient)
	if err != nil || addr.Address == "" {
		return nil, fmt.Errorf("invalid recipient email address %q", recipient)
	}

	cfg, pass, err := s.resolveOverride(ctx, override)
	if err != nil {
		return nil, err
	}
	mailer := BuildMailer(cfg, pass, s.logger)
	start := time.Now()
	err = mailer.Send(ctx, Message{
		FromName:  cfg.FromName,
		FromEmail: cfg.FromEmail,
		ReplyTo:   cfg.DefaultReplyTo,
		To:        addr.Address,
		Subject:   "Go CMS Email Configuration Test",
		HTMLBody:  SanitizeHTML(`<p>This test email confirms your Go CMS outbound email configuration is working.</p>`),
		TextBody:  "This test email confirms your Go CMS outbound email configuration is working.",
	})
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &TestResult{
			OK:        false,
			Provider:  mailer.Name(),
			LatencyMs: latency,
			Message:   err.Error(),
		}, nil
	}
	return &TestResult{
		OK:        true,
		Provider:  mailer.Name(),
		LatencyMs: latency,
		Message:   fmt.Sprintf("Test email dispatched via %s to %s.", mailer.Name(), addr.Address),
	}, nil
}

// ListTemplates returns all email templates sorted by key.
func (s *Service) ListTemplates(ctx context.Context) ([]EmailTemplate, error) {
	s.ensureSeeded(ctx)
	return s.repo.ListTemplates(ctx)
}

// GetTemplate retrieves one email template by its key.
func (s *Service) GetTemplate(ctx context.Context, key string) (*EmailTemplate, error) {
	s.ensureSeeded(ctx)
	return s.repo.GetTemplateByKey(ctx, strings.TrimSpace(key))
}

// UpdateTemplate validates headers, sanitizes HTML content, and persists template edits.
func (s *Service) UpdateTemplate(ctx context.Context, key string, input UpdateTemplateInput, meta settings.AuditMeta) (*EmailTemplate, error) {
	s.ensureSeeded(ctx)
	existing, err := s.repo.GetTemplateByKey(ctx, strings.TrimSpace(key))
	if err != nil {
		return nil, err
	}

	for _, field := range []string{input.Name, input.Subject, input.FromName, input.FromEmail, input.ReplyTo} {
		if HasHeaderInjection(field) {
			return nil, fmt.Errorf("header line breaks are not allowed in template metadata")
		}
	}
	if strings.TrimSpace(input.Subject) == "" {
		return nil, fmt.Errorf("template subject is required")
	}
	if strings.TrimSpace(input.FromEmail) != "" {
		addr, err := mail.ParseAddress(strings.TrimSpace(input.FromEmail))
		if err != nil || addr.Address == "" {
			return nil, fmt.Errorf("invalid from_email address")
		}
		input.FromEmail = addr.Address
	}
	if strings.TrimSpace(input.ReplyTo) != "" {
		addr, err := mail.ParseAddress(strings.TrimSpace(input.ReplyTo))
		if err != nil || addr.Address == "" {
			return nil, fmt.Errorf("invalid reply_to address")
		}
		input.ReplyTo = addr.Address
	}

	updated := *existing
	if strings.TrimSpace(input.Name) != "" {
		updated.Name = strings.TrimSpace(input.Name)
	}
	updated.Subject = strings.TrimSpace(input.Subject)
	updated.FromName = strings.TrimSpace(input.FromName)
	updated.FromEmail = strings.TrimSpace(input.FromEmail)
	updated.ReplyTo = strings.TrimSpace(input.ReplyTo)
	updated.BodyHTML = SanitizeHTML(input.BodyHTML)
	updated.BodyText = input.BodyText
	updated.Enabled = input.Enabled

	saved, err := s.repo.UpsertTemplate(ctx, &updated)
	if err != nil {
		return nil, err
	}
	return saved, nil
}

// PreviewTemplate renders sample (or provided) variables safely into the named template.
func (s *Service) PreviewTemplate(ctx context.Context, key string, override *UpdateTemplateInput, vars map[string]string) (*PreviewResult, error) {
	s.ensureSeeded(ctx)
	tmpl, err := s.repo.GetTemplateByKey(ctx, strings.TrimSpace(key))
	if err != nil {
		return nil, err
	}
	target := *tmpl
	if override != nil {
		if override.Subject != "" {
			target.Subject = override.Subject
		}
		if override.BodyHTML != "" {
			target.BodyHTML = override.BodyHTML
		}
		if override.BodyText != "" {
			target.BodyText = override.BodyText
		}
	}
	if len(vars) == 0 {
		vars = sampleVarsForTemplate(target.TemplateKey)
	}
	res, err := RenderTemplate(target, vars)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func sampleVarsForTemplate(key string) map[string]string {
	base := map[string]string{
		"site_name":        "Go CMS",
		"username":         "alex.morgan",
		"recipient_email":  "alex@example.com",
		"inviter_name":     "Super Admin",
		"invite_url":       "https://cms.example.com/admin/?invite_token=sample-token",
		"confirm_url":      "https://cms.example.com/confirm?token=sample-token",
		"reset_url":        "https://cms.example.com/reset-password?token=sample-token",
		"login_url":        "https://cms.example.com/admin/",
		"expires_in_hours": "72",
		"expires_minutes":  "30",
	}
	return base
}

// SendTemplate renders a stored template and dispatches it via the active email provider.
func (s *Service) SendTemplate(ctx context.Context, key string, to string, vars map[string]string) error {
	s.ensureSeeded(ctx)
	if HasHeaderInjection(to) {
		return fmt.Errorf("invalid recipient email")
	}
	cfg, pass, err := s.ResolveRuntimeConfig(ctx)
	if err != nil {
		return err
	}
	mailer := BuildMailer(cfg, pass, s.logger)
	if mailer.Name() == ProviderDisabled {
		return ErrEmailDisabled
	}

	tmpl, err := s.repo.GetTemplateByKey(ctx, strings.TrimSpace(key))
	if err != nil {
		return err
	}
	if !tmpl.Enabled {
		return fmt.Errorf("email template %q is disabled", tmpl.TemplateKey)
	}

	merged := make(map[string]string, len(vars)+1)
	if s.settingsSvc != nil {
		pub := s.settingsSvc.GetPublicGeneral(ctx)
		if pub.SiteName != "" {
			merged["site_name"] = pub.SiteName
		}
	}
	if merged["site_name"] == "" {
		merged["site_name"] = "Go CMS"
	}
	for k, v := range vars {
		merged[k] = v
	}

	rendered, err := RenderTemplate(*tmpl, merged)
	if err != nil {
		return err
	}

	fromName := tmpl.FromName
	if fromName == "" {
		fromName = cfg.FromName
	}
	fromEmail := tmpl.FromEmail
	if fromEmail == "" {
		fromEmail = cfg.FromEmail
	}
	replyTo := tmpl.ReplyTo
	if replyTo == "" {
		replyTo = cfg.DefaultReplyTo
	}

	return mailer.Send(ctx, Message{
		FromName:  fromName,
		FromEmail: fromEmail,
		ReplyTo:   replyTo,
		To:        to,
		Subject:   rendered.Subject,
		HTMLBody:  rendered.BodyHTML,
		TextBody:  rendered.BodyText,
	})
}

// SendAdminInvitation delivers an admin invitation email containing the onboarding link.
func (s *Service) SendAdminInvitation(ctx context.Context, recipientEmail, rawToken, inviterName string, expiresInHours int) error {
	baseURL := "http://localhost:8080"
	adminPath := "/admin"
	if s.settingsSvc != nil {
		if gen, err := s.settingsSvc.GetGeneral(ctx); err == nil && gen != nil {
			if gen.PublicURL != "" {
				baseURL = strings.TrimRight(gen.PublicURL, "/")
			}
			if gen.AdminPath != "" {
				adminPath = gen.AdminPath
			}
		}
	}
	inviteURL := fmt.Sprintf("%s%s/?invite_token=%s", baseURL, adminPath, url.QueryEscape(rawToken))
	if inviterName == "" {
		inviterName = "An administrator"
	}
	if expiresInHours <= 0 {
		expiresInHours = 72
	}
	return s.SendTemplate(ctx, "admin_invite", recipientEmail, map[string]string{
		"recipient_email":  recipientEmail,
		"inviter_name":     inviterName,
		"invite_url":       inviteURL,
		"expires_in_hours": strconv.Itoa(expiresInHours),
	})
}

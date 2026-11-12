package email

import (
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/settings"
)

const (
	ProviderDisabled = "disabled"
	ProviderLog      = "log"
	ProviderSMTP     = "smtp"

	EncryptionTLS      = "tls"
	EncryptionSTARTTLS = "starttls"
	EncryptionNone     = "none"
)

// EmailSettings holds persistent outbound email transport and sender defaults.
type EmailSettings struct {
	Provider           string    `json:"provider"`
	FromName           string    `json:"from_name"`
	FromEmail          string    `json:"from_email"`
	DefaultReplyTo     string    `json:"default_reply_to"`
	SMTPHost           string    `json:"smtp_host"`
	SMTPPort           int       `json:"smtp_port"`
	SMTPUsername       string    `json:"smtp_username"`
	SMTPPassword       string    `json:"smtp_password,omitempty"`
	SMTPPasswordEnc    string    `json:"smtp_password_enc,omitempty"`
	PasswordConfigured bool      `json:"password_configured"`
	Encryption         string    `json:"encryption"`
	SkipTLSVerify      bool      `json:"skip_tls_verify"`
	TimeoutSeconds     int       `json:"timeout_seconds"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
}

// DefaultEmailSettings returns baseline development-safe email settings.
func DefaultEmailSettings() EmailSettings {
	return EmailSettings{
		Provider:       ProviderLog,
		FromName:       "Go CMS",
		FromEmail:      "no-reply@localhost.localdomain",
		SMTPPort:       587,
		Encryption:     EncryptionSTARTTLS,
		TimeoutSeconds: 10,
	}
}

// Sanitized returns a copy safe for HTTP JSON serialization (replaces encrypted password with sentinel).
func (e EmailSettings) Sanitized() EmailSettings {
	out := e
	configured := out.SMTPPasswordEnc != "" || out.PasswordConfigured
	out.PasswordConfigured = configured
	out.SMTPPasswordEnc = ""
	if configured {
		out.SMTPPassword = settings.SecretPreserveSentinel
	} else {
		out.SMTPPassword = ""
	}
	return out
}

// NormalizeAndValidate trims fields, rejects header injection, and checks provider configuration.
func (e *EmailSettings) NormalizeAndValidate() error {
	e.Provider = strings.ToLower(strings.TrimSpace(e.Provider))
	if e.Provider == "" {
		e.Provider = ProviderLog
	}
	switch e.Provider {
	case ProviderDisabled, ProviderLog, ProviderSMTP:
	default:
		return fmt.Errorf("unsupported email provider %q (must be smtp, log, or disabled)", e.Provider)
	}

	e.FromName = strings.TrimSpace(e.FromName)
	e.FromEmail = strings.TrimSpace(e.FromEmail)
	e.DefaultReplyTo = strings.TrimSpace(e.DefaultReplyTo)
	e.SMTPHost = strings.TrimSpace(e.SMTPHost)
	e.SMTPUsername = strings.TrimSpace(e.SMTPUsername)
	e.Encryption = strings.ToLower(strings.TrimSpace(e.Encryption))
	if e.Encryption == "" {
		e.Encryption = EncryptionSTARTTLS
	}
	if e.TimeoutSeconds <= 0 {
		e.TimeoutSeconds = 10
	}
	if e.TimeoutSeconds > 120 {
		return fmt.Errorf("timeout_seconds must be between 1 and 120")
	}

	for _, field := range []string{e.FromName, e.FromEmail, e.DefaultReplyTo, e.SMTPHost, e.SMTPUsername} {
		if HasHeaderInjection(field) {
			return fmt.Errorf("header line breaks are not allowed in email settings fields")
		}
	}

	if e.FromEmail != "" {
		addr, err := mail.ParseAddress(e.FromEmail)
		if err != nil || addr.Address == "" {
			return fmt.Errorf("invalid from_email address %q", e.FromEmail)
		}
		e.FromEmail = addr.Address
	}
	if e.DefaultReplyTo != "" {
		addr, err := mail.ParseAddress(e.DefaultReplyTo)
		if err != nil || addr.Address == "" {
			return fmt.Errorf("invalid default_reply_to address %q", e.DefaultReplyTo)
		}
		e.DefaultReplyTo = addr.Address
	}

	switch e.Encryption {
	case EncryptionTLS, EncryptionSTARTTLS, EncryptionNone:
	default:
		return fmt.Errorf("unsupported SMTP encryption mode %q (must be tls, starttls, or none)", e.Encryption)
	}

	if e.Provider == ProviderSMTP {
		if e.FromEmail == "" {
			return fmt.Errorf("from_email is required when provider is smtp")
		}
		if e.SMTPHost == "" {
			return fmt.Errorf("smtp_host is required when provider is smtp")
		}
		if e.SMTPPort < 1 || e.SMTPPort > 65535 {
			return fmt.Errorf("smtp_port must be between 1 and 65535")
		}
	} else if e.SMTPPort == 0 {
		e.SMTPPort = 587
	}

	return nil
}

// EmailTemplate represents a customizable transactional email template stored in cms_email_templates.
type EmailTemplate struct {
	ID          string    `json:"id"`
	TemplateKey string    `json:"template_key"`
	Name        string    `json:"name"`
	Subject     string    `json:"subject"`
	FromName    string    `json:"from_name"`
	FromEmail   string    `json:"from_email"`
	ReplyTo     string    `json:"reply_to"`
	BodyHTML    string    `json:"body_html"`
	BodyText    string    `json:"body_text"`
	Variables   []string  `json:"variables"`
	Enabled     bool      `json:"enabled"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UpdateTemplateInput captures editable fields for updating an email template.
type UpdateTemplateInput struct {
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	FromName  string `json:"from_name"`
	FromEmail string `json:"from_email"`
	ReplyTo   string `json:"reply_to"`
	BodyHTML  string `json:"body_html"`
	BodyText  string `json:"body_text"`
	Enabled   bool   `json:"enabled"`
}

// Message describes a single rendered outbound email.
type Message struct {
	FromName  string
	FromEmail string
	ReplyTo   string
	To        string
	Subject   string
	HTMLBody  string
	TextBody  string
}

// TestResult reports connection check or test email delivery results.
type TestResult struct {
	OK        bool   `json:"ok"`
	Provider  string `json:"provider"`
	LatencyMs int64  `json:"latency_ms"`
	Message   string `json:"message"`
}

// PreviewResult returns rendered HTML, text, and subject for a template preview.
type PreviewResult struct {
	Subject  string `json:"subject"`
	BodyHTML string `json:"body_html"`
	BodyText string `json:"body_text"`
}


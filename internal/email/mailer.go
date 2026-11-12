package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// ErrEmailDisabled is returned when email delivery is attempted while provider is disabled.
var ErrEmailDisabled = errors.New("email delivery is disabled")

// Mailer abstracts outbound email delivery and transport health checks.
type Mailer interface {
	Name() string
	Send(ctx context.Context, msg Message) error
	CheckConnection(ctx context.Context) error
}

// BuildMailer constructs the concrete Mailer implementation for EmailSettings and decrypted SMTP password.
func BuildMailer(cfg EmailSettings, smtpPassword string, logger *slog.Logger) Mailer {
	if logger == nil {
		logger = slog.Default()
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case ProviderDisabled:
		return &DisabledMailer{}
	case ProviderSMTP:
		return &SMTPMailer{
			cfg:      cfg,
			password: smtpPassword,
		}
	default:
		return &LogMailer{logger: logger}
	}
}

// DisabledMailer explicitly rejects email delivery when disabled in settings.
type DisabledMailer struct{}

func (m *DisabledMailer) Name() string { return ProviderDisabled }

func (m *DisabledMailer) Send(context.Context, Message) error {
	return ErrEmailDisabled
}

func (m *DisabledMailer) CheckConnection(context.Context) error {
	return ErrEmailDisabled
}

// LogMailer records sanitized message metadata to application logs for development and previews.
type LogMailer struct {
	logger *slog.Logger
}

func (m *LogMailer) Name() string { return ProviderLog }

func (m *LogMailer) Send(_ context.Context, msg Message) error {
	m.logger.Info("Email delivery (log transport)",
		"to", msg.To,
		"subject", msg.Subject,
		"from", msg.FromEmail,
		"html_bytes", len(msg.HTMLBody),
		"text_bytes", len(msg.TextBody),
	)
	return nil
}

func (m *LogMailer) CheckConnection(context.Context) error {
	return nil
}

// SMTPMailer delivers multipart MIME emails via net/smtp over TLS, STARTTLS, or plain TCP.
type SMTPMailer struct {
	cfg      EmailSettings
	password string
}

func (m *SMTPMailer) Name() string { return ProviderSMTP }

func (m *SMTPMailer) timeout() time.Duration {
	if m.cfg.TimeoutSeconds > 0 {
		return time.Duration(m.cfg.TimeoutSeconds) * time.Second
	}
	return 10 * time.Second
}

func (m *SMTPMailer) openClient(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(m.cfg.SMTPHost, fmt.Sprintf("%d", m.cfg.SMTPPort))
	timeout := m.timeout()
	dialer := &net.Dialer{Timeout: timeout}
	tlsCfg := &tls.Config{
		ServerName:         m.cfg.SMTPHost,
		InsecureSkipVerify: m.cfg.SkipTLSVerify,
		MinVersion:         tls.VersionTLS12,
	}

	var conn net.Conn
	var err error
	if m.cfg.Encryption == EncryptionTLS {
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: tlsCfg}
		conn, err = tlsDialer.DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SMTP server %s: %w", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))

	client, err := smtp.NewClient(conn, m.cfg.SMTPHost)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("SMTP handshake failed: %w", err)
	}

	if m.cfg.Encryption == EncryptionSTARTTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP server %s does not advertise STARTTLS", addr)
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP STARTTLS upgrade failed: %w", err)
		}
	}

	if m.cfg.SMTPUsername != "" {
		auth := smtp.PlainAuth("", m.cfg.SMTPUsername, m.password, m.cfg.SMTPHost)
		if err := client.Auth(auth); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	return client, nil
}

func (m *SMTPMailer) CheckConnection(ctx context.Context) error {
	client, err := m.openClient(ctx)
	if err != nil {
		return err
	}
	return client.Quit()
}

func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	if HasHeaderInjection(msg.To) || HasHeaderInjection(msg.Subject) || HasHeaderInjection(msg.FromName) || HasHeaderInjection(msg.FromEmail) || HasHeaderInjection(msg.ReplyTo) {
		return fmt.Errorf("email message headers contain invalid newline characters")
	}

	toAddr, err := mail.ParseAddress(msg.To)
	if err != nil || toAddr.Address == "" {
		return fmt.Errorf("invalid recipient email address %q", msg.To)
	}
	fromEmail := strings.TrimSpace(msg.FromEmail)
	if fromEmail == "" {
		fromEmail = m.cfg.FromEmail
	}
	fromAddr, err := mail.ParseAddress(fromEmail)
	if err != nil || fromAddr.Address == "" {
		return fmt.Errorf("invalid sender email address %q", fromEmail)
	}

	rawMsg := buildMIMEMessage(Message{
		FromName:  msg.FromName,
		FromEmail: fromAddr.Address,
		ReplyTo:   msg.ReplyTo,
		To:        toAddr.Address,
		Subject:   msg.Subject,
		HTMLBody:  msg.HTMLBody,
		TextBody:  msg.TextBody,
	})

	client, err := m.openClient(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err := client.Mail(fromAddr.Address); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	if err := client.Rcpt(toAddr.Address); err != nil {
		return fmt.Errorf("SMTP RCPT TO failed: %w", err)
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}
	if _, err := wc.Write(rawMsg); err != nil {
		_ = wc.Close()
		return fmt.Errorf("SMTP write payload failed: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("SMTP commit failed: %w", err)
	}
	return client.Quit()
}

func buildMIMEMessage(msg Message) []byte {
	fromHeader := (&mail.Address{Name: msg.FromName, Address: msg.FromEmail}).String()
	boundary := fmt.Sprintf("gocms-alt-%d", time.Now().UnixNano())

	var sb strings.Builder
	sb.WriteString("From: " + fromHeader + "\r\n")
	sb.WriteString("To: " + msg.To + "\r\n")
	if msg.ReplyTo != "" {
		sb.WriteString("Reply-To: " + msg.ReplyTo + "\r\n")
	}
	sb.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", msg.Subject) + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")

	textBody := msg.TextBody
	if textBody == "" {
		textBody = msg.Subject
	}
	sb.WriteString("--" + boundary + "\r\n")
	sb.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	sb.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	sb.WriteString(strings.ReplaceAll(textBody, "\n", "\r\n") + "\r\n\r\n")

	if msg.HTMLBody != "" {
		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		sb.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		sb.WriteString(strings.ReplaceAll(msg.HTMLBody, "\n", "\r\n") + "\r\n\r\n")
	}

	sb.WriteString("--" + boundary + "--\r\n")
	return []byte(sb.String())
}


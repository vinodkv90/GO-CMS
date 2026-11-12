package email

import (
	"time"

	"github.com/google/uuid"
)

// DefaultTemplates returns the standard transactional templates seeded on first startup.
func DefaultTemplates() []EmailTemplate {
	now := time.Now().UTC()
	return []EmailTemplate{
		{
			ID:          uuid.New().String(),
			TemplateKey: "admin_invite",
			Name:        "Admin Panel Invitation",
			Subject:     "You've been invited to {{site_name}} Administration",
			BodyHTML: `<div><h2>Welcome to {{site_name}}</h2><p>Hello {{recipient_email}},</p><p>{{inviter_name}} has invited you to collaborate as an administrator.</p><p><a href="{{invite_url}}">Accept your invitation</a> (expires in {{expires_in_hours}} hours).</p></div>`,
			BodyText:    "Hello {{recipient_email}},\n\n{{inviter_name}} invited you to {{site_name}}.\nAccept your invitation within {{expires_in_hours}} hours:\n{{invite_url}}\n",
			Variables:   []string{"site_name", "recipient_email", "inviter_name", "invite_url", "expires_in_hours"},
			Enabled:     true,
			UpdatedAt:   now,
		},
		{
			ID:          uuid.New().String(),
			TemplateKey: "email_confirmation",
			Name:        "Email Confirmation",
			Subject:     "Confirm your email address for {{site_name}}",
			BodyHTML: `<div><h2>Confirm your account</h2><p>Hello {{username}},</p><p>Please confirm your email address by visiting the link below:</p><p><a href="{{confirm_url}}">Confirm Email Address</a></p></div>`,
			BodyText:    "Hello {{username}},\n\nConfirm your email address for {{site_name}}:\n{{confirm_url}}\n",
			Variables:   []string{"site_name", "username", "confirm_url"},
			Enabled:     true,
			UpdatedAt:   now,
		},
		{
			ID:          uuid.New().String(),
			TemplateKey: "password_reset",
			Name:        "Password Reset",
			Subject:     "Reset your {{site_name}} password",
			BodyHTML: `<div><h2>Password Reset Request</h2><p>Hello {{username}},</p><p>We received a request to reset your password. Use the link below within {{expires_minutes}} minutes:</p><p><a href="{{reset_url}}">Reset Password</a></p></div>`,
			BodyText:    "Hello {{username}},\n\nReset your {{site_name}} password (expires in {{expires_minutes}} minutes):\n{{reset_url}}\n",
			Variables:   []string{"site_name", "username", "reset_url", "expires_minutes"},
			Enabled:     true,
			UpdatedAt:   now,
		},
		{
			ID:          uuid.New().String(),
			TemplateKey: "welcome",
			Name:        "Welcome Email",
			Subject:     "Welcome to {{site_name}}",
			BodyHTML: `<div><h2>Welcome aboard, {{username}}!</h2><p>Your account on {{site_name}} is ready.</p><p><a href="{{login_url}}">Sign in</a></p></div>`,
			BodyText:    "Welcome aboard, {{username}}!\n\nSign in at {{login_url}}\n",
			Variables:   []string{"site_name", "username", "login_url"},
			Enabled:     true,
			UpdatedAt:   now,
		},
		{
			ID:          uuid.New().String(),
			TemplateKey: "account_activation",
			Name:        "Account Activation",
			Subject:     "Your {{site_name}} account has been activated",
			BodyHTML: `<div><h2>Account Activated</h2><p>Hello {{username}},</p><p>Your account has been activated and you can now sign in.</p><p><a href="{{login_url}}">Go to Sign In</a></p></div>`,
			BodyText:    "Hello {{username}},\n\nYour {{site_name}} account is now active:\n{{login_url}}\n",
			Variables:   []string{"site_name", "username", "login_url"},
			Enabled:     true,
			UpdatedAt:   now,
		},
	}
}


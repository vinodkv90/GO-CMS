package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrAccountLocked   = errors.New("account temporarily locked due to failed login attempts")
	ErrMFARequired     = errors.New("multi-factor authentication verification required")
	ErrInvalidMFACode  = errors.New("invalid multi-factor verification code")
	ErrPasswordExpired = errors.New("password has expired and must be updated")
	ErrSSOOnlyAccount  = errors.New("password login is disabled for accounts using enforced SSO")
	ErrIPNotAllowed    = errors.New("client IP address is not permitted to access the admin panel")
)

// SecurityRuntimeConfig captures live security, lockout, password policy, and SSO enforcement settings.
type SecurityRuntimeConfig struct {
	SessionTimeoutMinutes  int
	MaxFailedLogins        int
	LockoutDurationMinutes int
	EnforceAdminMFA        bool
	AllowedAdminIPCIDRs    []string
	PasswordPolicy         PasswordPolicy
	SSOEnabled             bool
	SSOEnforcedDomains     []string
}

// DefaultSecurityRuntimeConfig returns baseline runtime security parameters.
func DefaultSecurityRuntimeConfig() SecurityRuntimeConfig {
	return SecurityRuntimeConfig{
		SessionTimeoutMinutes:  1440,
		MaxFailedLogins:        5,
		LockoutDurationMinutes: 15,
		EnforceAdminMFA:        false,
		AllowedAdminIPCIDRs:    nil,
		PasswordPolicy:         DefaultPasswordPolicy(),
	}
}

// SecurityPolicyProvider supplies dynamic security and password policy settings at runtime.
type SecurityPolicyProvider func(ctx context.Context) SecurityRuntimeConfig

// SetSecurityPolicyProvider wires a live callback for dynamic security, lockout, MFA, IP, and password policies.
func (s *Service) SetSecurityPolicyProvider(fn SecurityPolicyProvider) {
	s.securityPolicyFn = fn
}

func (s *Service) securityPolicy(ctx context.Context) SecurityRuntimeConfig {
	defaultMins := 1440
	if s.cfg != nil && s.cfg.Auth.TokenExpiration > 0 {
		defaultMins = int(s.cfg.Auth.TokenExpiration.Minutes())
		if defaultMins <= 0 {
			defaultMins = 1440
		}
	}
	if s.securityPolicyFn != nil {
		cfg := s.securityPolicyFn(ctx)
		if cfg.SessionTimeoutMinutes <= 0 {
			cfg.SessionTimeoutMinutes = defaultMins
		}
		if cfg.MaxFailedLogins <= 0 {
			cfg.MaxFailedLogins = 5
		}
		if cfg.LockoutDurationMinutes <= 0 {
			cfg.LockoutDurationMinutes = 15
		}
		if cfg.PasswordPolicy.MinLength < 8 {
			cfg.PasswordPolicy.MinLength = 8
		}
		return cfg
	}
	def := DefaultSecurityRuntimeConfig()
	def.SessionTimeoutMinutes = defaultMins
	return def
}

func (s *Service) sessionTTL(policy SecurityRuntimeConfig) time.Duration {
	if policy.SessionTimeoutMinutes > 0 {
		return time.Duration(policy.SessionTimeoutMinutes) * time.Minute
	}
	if s.cfg != nil && s.cfg.Auth.TokenExpiration > 0 {
		return s.cfg.Auth.TokenExpiration
	}
	return 24 * time.Hour
}

func (s *Service) mfaEncryptionKey() string {
	if s.cfg != nil {
		if k := strings.TrimSpace(s.cfg.Auth.SessionSecret); k != "" {
			return k
		}
		if k := strings.TrimSpace(s.cfg.Auth.JWTSecret); k != "" {
			return k
		}
	}
	return "go-cms-dev-mfa-secret"
}

func (s *Service) validatePasswordForPolicy(ctx context.Context, password string) error {
	policy := s.securityPolicy(ctx).PasswordPolicy
	return ValidatePasswordWithPolicy(password, policy)
}

// ExtractClientIP extracts the normalized IP string from RemoteAddr or a bare IP.
func ExtractClientIP(remoteAddr string) string {
	trimmed := strings.TrimSpace(remoteAddr)
	if trimmed == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		return strings.TrimSpace(host)
	}
	return strings.Trim(trimmed, "[]")
}

// IsIPAllowed returns true when allowedCIDRs is empty or when clientAddr falls inside any CIDR block.
func IsIPAllowed(clientAddr string, allowedCIDRs []string) bool {
	if len(allowedCIDRs) == 0 {
		return true
	}
	ipStr := ExtractClientIP(clientAddr)
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, raw := range allowedCIDRs {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			if network.Contains(ip) {
				return true
			}
			continue
		}
		if single := net.ParseIP(entry); single != nil && single.Equal(ip) {
			return true
		}
	}
	return false
}

func emailDomain(email string) string {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(email)), "@", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func isDomainListed(domain string, list []string) bool {
	if domain == "" {
		return false
	}
	for _, d := range list {
		if strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(d), "@"), domain) {
			return true
		}
	}
	return false
}

// LoginResult represents the outcome of an authentication attempt, including optional MFA or password expiry steps.
type LoginResult struct {
	User             *User  `json:"user,omitempty"`
	Token            string `json:"token,omitempty"`
	MFARequired      bool   `json:"mfa_required,omitempty"`
	MFASetupRequired bool   `json:"mfa_setup_required,omitempty"`
	MFAToken         string `json:"mfa_token,omitempty"`
	PasswordExpired  bool   `json:"password_expired,omitempty"`
}

// MFAEnrollResult contains the secret and authenticator URI returned when starting TOTP enrollment.
type MFAEnrollResult struct {
	Secret     string `json:"secret"`
	OTPAuthURI string `json:"otpauth_uri"`
	Issuer     string `json:"issuer"`
	Account    string `json:"account"`
}

// MFAConfirmResult returns recovery codes and an active session token once TOTP enrollment is confirmed.
type MFAConfirmResult struct {
	User          *User    `json:"user"`
	RecoveryCodes []string `json:"recovery_codes"`
	Token         string   `json:"token,omitempty"`
}

// SSOLoginRequest represents verified enterprise OIDC identity attributes used to complete Admin SSO login.
type SSOLoginRequest struct {
	Provider      string
	Subject       string
	Email         string
	FirstName     string
	LastName      string
	AutoProvision bool
	DefaultRoleID string
	ClientIP      string
}

// AuthenticateWithOptions validates credentials and applies lockout, IP allowlist, SSO enforcement, password age, and MFA rules.
func (s *Service) AuthenticateWithOptions(ctx context.Context, identifier, password, clientIP string) (*LoginResult, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	policy := s.securityPolicy(ctx)
	if !IsIPAllowed(clientIP, policy.AllowedAdminIPCIDRs) {
		s.recordAudit(ctx, AuditMeta{ActorEmail: identifier, IP: clientIP}, "admin_login_blocked_ip", "user", "", map[string]any{"ip": ExtractClientIP(clientIP)}, "failure")
		return nil, ErrIPNotAllowed
	}

	var user *User
	var err error
	if strings.Contains(identifier, "@") {
		user, err = s.userRepo.GetUserByEmail(ctx, strings.ToLower(identifier))
	} else {
		user, err = s.userRepo.GetUserByUsername(ctx, identifier)
	}
	if err != nil {
		s.recordAudit(ctx, AuditMeta{ActorEmail: identifier, IP: clientIP}, "admin_login_failed", "auth:session", "", map[string]any{"reason": "invalid_credentials"}, "failure")
		return nil, ErrInvalidCredentials
	}

	if policy.SSOEnabled && isDomainListed(emailDomain(user.Email), policy.SSOEnforcedDomains) {
		return nil, ErrSSOOnlyAccount
	}

	now := time.Now().UTC()
	if user.LockedUntil != nil && now.Before(*user.LockedUntil) {
		return nil, fmt.Errorf("%w: try again after %s", ErrAccountLocked, user.LockedUntil.UTC().Format(time.RFC3339))
	}

	ok, err := ComparePassword(password, user.PasswordHash)
	if err != nil || !ok {
		user.FailedLoginAttempts++
		if policy.MaxFailedLogins > 0 && user.FailedLoginAttempts >= policy.MaxFailedLogins {
			lockedUntil := now.Add(time.Duration(policy.LockoutDurationMinutes) * time.Minute)
			user.LockedUntil = &lockedUntil
			s.recordAudit(ctx, AuditMeta{ActorID: user.ID, ActorEmail: user.Email, IP: clientIP}, "admin_account_locked", "user", user.ID, map[string]any{
				"failed_attempts": user.FailedLoginAttempts,
				"locked_until":    lockedUntil.Format(time.RFC3339),
			}, "failure")
		}
		user.UpdatedAt = now
		_ = s.userRepo.UpdateUser(ctx, user)
		s.recordAudit(ctx, AuditMeta{ActorID: user.ID, ActorEmail: user.Email, IP: clientIP}, "admin_login_failed", "auth:session", user.ID, map[string]any{"reason": "invalid_credentials", "failed_attempts": user.FailedLoginAttempts}, "failure")
		return nil, ErrInvalidCredentials
	}

	if user.Status != StatusActive {
		return nil, ErrUserSuspended
	}

	// Password is verified; clear any prior failed attempts or expired lock.
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil

	// Check password rotation expiry before granting a session or MFA step.
	if policy.PasswordPolicy.MaxAgeDays > 0 {
		refTime := user.CreatedAt
		if user.PasswordChangedAt != nil && !user.PasswordChangedAt.IsZero() {
			refTime = *user.PasswordChangedAt
		}
		if now.Sub(refTime) > time.Duration(policy.PasswordPolicy.MaxAgeDays)*24*time.Hour {
			user.UpdatedAt = now
			_ = s.userRepo.UpdateUser(ctx, user)
			return &LoginResult{User: user, PasswordExpired: true}, ErrPasswordExpired
		}
	}

	// MFA step if user has TOTP enabled or Admin MFA enforcement requires onboarding.
	if user.MFAEnabled {
		user.UpdatedAt = now
		_ = s.userRepo.UpdateUser(ctx, user)
		challengeToken, err := GenerateMFAChallengeToken(user.ID, user.Email, "mfa_challenge", s.cfg.Auth.JWTSecret, 5*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("failed to issue MFA challenge: %w", err)
		}
		return &LoginResult{
			User:        user,
			MFARequired: true,
			MFAToken:    challengeToken,
		}, nil
	}

	if policy.EnforceAdminMFA {
		user.UpdatedAt = now
		_ = s.userRepo.UpdateUser(ctx, user)
		setupToken, err := GenerateMFAChallengeToken(user.ID, user.Email, "mfa_setup_challenge", s.cfg.Auth.JWTSecret, 10*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("failed to issue MFA setup challenge: %w", err)
		}
		return &LoginResult{
			User:             user,
			MFASetupRequired: true,
			MFAToken:         setupToken,
		}, nil
	}

	user.LastLoginAt = &now
	user.UpdatedAt = now
	_ = s.userRepo.UpdateUser(ctx, user)

	token, err := GenerateAccessToken(user, s.cfg.Auth.JWTSecret, s.sessionTTL(policy))
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}
	s.recordAudit(ctx, AuditMeta{ActorID: user.ID, ActorEmail: user.Email, IP: clientIP}, "admin_login", "auth:session", user.ID, nil, "success")
	return &LoginResult{User: user, Token: token}, nil
}

// EnrollMFA starts TOTP enrollment for an admin user without yet activating MFA until a valid code is confirmed.
func (s *Service) EnrollMFA(ctx context.Context, userID string) (*MFAEnrollResult, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.MFAEnabled {
		return nil, errors.New("MFA is already enabled on this account")
	}

	secret, err := GenerateTOTPSecret()
	if err != nil {
		return nil, err
	}
	enc, err := EncryptMFASecret(s.mfaEncryptionKey(), secret)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt MFA secret: %w", err)
	}

	user.MFASecretEnc = enc
	user.UpdatedAt = time.Now().UTC()
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	issuer := "Go CMS"
	return &MFAEnrollResult{
		Secret:     secret,
		OTPAuthURI: BuildOTPAuthURI(issuer, user.Email, secret),
		Issuer:     issuer,
		Account:    user.Email,
	}, nil
}

// ConfirmMFA verifies the user's first TOTP code, enables MFA, generates single-use recovery codes, and returns a session token.
func (s *Service) ConfirmMFA(ctx context.Context, userID, code string) (*MFAConfirmResult, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.MFAEnabled {
		return nil, errors.New("MFA is already enabled on this account")
	}
	if strings.TrimSpace(user.MFASecretEnc) == "" {
		return nil, errors.New("no pending MFA enrollment found; start enrollment first")
	}

	secret, err := DecryptMFASecret(s.mfaEncryptionKey(), user.MFASecretEnc)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt pending MFA secret: %w", err)
	}
	now := time.Now().UTC()
	if !VerifyTOTPCode(secret, code, now) {
		return nil, ErrInvalidMFACode
	}

	plainCodes, hashedJSON, err := GenerateRecoveryCodes(8)
	if err != nil {
		return nil, err
	}

	user.MFAEnabled = true
	user.MFABackupCodesHash = hashedJSON
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	fullUser, err := s.userRepo.GetUserByID(ctx, user.ID)
	if err != nil {
		fullUser = user
	}
	token, err := GenerateAccessToken(fullUser, s.cfg.Auth.JWTSecret, s.sessionTTL(s.securityPolicy(ctx)))
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	s.recordAudit(ctx, AuditMeta{ActorID: user.ID, ActorEmail: user.Email}, "admin_mfa_enabled", "user", user.ID, nil, "success")
	return &MFAConfirmResult{
		User:          fullUser,
		RecoveryCodes: plainCodes,
		Token:         token,
	}, nil
}

// VerifyMFALogin completes a two-step login challenge using either a TOTP code or a single-use recovery code.
func (s *Service) VerifyMFALogin(ctx context.Context, mfaToken, code, clientIP string) (*LoginResult, error) {
	claims, err := ValidateMFAChallengeToken(mfaToken, s.cfg.Auth.JWTSecret)
	if err != nil || claims.Purpose != "mfa_challenge" {
		return nil, ErrUnauthorized
	}

	policy := s.securityPolicy(ctx)
	if !IsIPAllowed(clientIP, policy.AllowedAdminIPCIDRs) {
		return nil, ErrIPNotAllowed
	}

	user, err := s.userRepo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if user.Status != StatusActive {
		return nil, ErrUserSuspended
	}

	now := time.Now().UTC()
	if user.LockedUntil != nil && now.Before(*user.LockedUntil) {
		return nil, fmt.Errorf("%w: try again after %s", ErrAccountLocked, user.LockedUntil.UTC().Format(time.RFC3339))
	}
	if !user.MFAEnabled || strings.TrimSpace(user.MFASecretEnc) == "" {
		return nil, errors.New("MFA is not enabled for this account")
	}

	secret, err := DecryptMFASecret(s.mfaEncryptionKey(), user.MFASecretEnc)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt MFA secret: %w", err)
	}

	matched := VerifyTOTPCode(secret, code, now)
	usedRecovery := false
	if !matched {
		ok, updatedHashes, rErr := VerifyAndConsumeRecoveryCode(user.MFABackupCodesHash, code)
		if rErr == nil && ok {
			matched = true
			usedRecovery = true
			user.MFABackupCodesHash = updatedHashes
		}
	}

	if !matched {
		user.FailedLoginAttempts++
		if policy.MaxFailedLogins > 0 && user.FailedLoginAttempts >= policy.MaxFailedLogins {
			lockedUntil := now.Add(time.Duration(policy.LockoutDurationMinutes) * time.Minute)
			user.LockedUntil = &lockedUntil
		}
		user.UpdatedAt = now
		_ = s.userRepo.UpdateUser(ctx, user)
		return nil, ErrInvalidMFACode
	}

	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	token, err := GenerateAccessToken(user, s.cfg.Auth.JWTSecret, s.sessionTTL(policy))
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}
	s.recordAudit(ctx, AuditMeta{ActorID: user.ID, ActorEmail: user.Email, IP: clientIP}, "admin_mfa_verified", "user", user.ID, map[string]any{
		"used_recovery_code": usedRecovery,
	}, "success")
	return &LoginResult{User: user, Token: token}, nil
}

// DisableMFA disables TOTP multi-factor authentication after verifying the user's password and TOTP/recovery code.
func (s *Service) DisableMFA(ctx context.Context, userID, password, code string) (*User, error) {
	if policy := s.securityPolicy(ctx); policy.EnforceAdminMFA {
		return nil, errors.New("MFA cannot be disabled while Enforce Admin MFA policy is enabled")
	}

	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.MFAEnabled {
		return nil, errors.New("MFA is not enabled on this account")
	}

	if user.SSOProvider == "" {
		ok, cmpErr := ComparePassword(password, user.PasswordHash)
		if cmpErr != nil || !ok {
			return nil, ErrInvalidCredentials
		}
	}

	secret, err := DecryptMFASecret(s.mfaEncryptionKey(), user.MFASecretEnc)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	matched := VerifyTOTPCode(secret, code, now)
	if !matched {
		ok, _, rErr := VerifyAndConsumeRecoveryCode(user.MFABackupCodesHash, code)
		if rErr == nil && ok {
			matched = true
		}
	}
	if !matched {
		return nil, ErrInvalidMFACode
	}

	user.MFAEnabled = false
	user.MFASecretEnc = ""
	user.MFABackupCodesHash = ""
	user.UpdatedAt = now
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, AuditMeta{ActorID: user.ID, ActorEmail: user.Email}, "admin_mfa_disabled", "user", user.ID, nil, "success")
	return s.userRepo.GetUserByID(ctx, user.ID)
}

// RegenerateRecoveryCodes issues a new set of single-use backup recovery codes after validating a current TOTP code.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID, totpCode string) ([]string, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.MFAEnabled || strings.TrimSpace(user.MFASecretEnc) == "" {
		return nil, errors.New("MFA is not enabled on this account")
	}
	secret, err := DecryptMFASecret(s.mfaEncryptionKey(), user.MFASecretEnc)
	if err != nil {
		return nil, err
	}
	if !VerifyTOTPCode(secret, totpCode, time.Now().UTC()) {
		return nil, ErrInvalidMFACode
	}

	plainCodes, hashedJSON, err := GenerateRecoveryCodes(8)
	if err != nil {
		return nil, err
	}
	user.MFABackupCodesHash = hashedJSON
	user.UpdatedAt = time.Now().UTC()
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, AuditMeta{ActorID: user.ID, ActorEmail: user.Email}, "admin_mfa_recovery_regenerated", "user", user.ID, nil, "success")
	return plainCodes, nil
}

// ResetExpiredPassword allows a user whose password exceeded MaxAgeDays to rotate their password and log in.
func (s *Service) ResetExpiredPassword(ctx context.Context, identifier, currentPassword, newPassword, clientIP string) (*LoginResult, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || currentPassword == "" || newPassword == "" {
		return nil, errors.New("identifier, current password, and new password are required")
	}
	if currentPassword == newPassword {
		return nil, errors.New("new password must differ from current password")
	}

	var user *User
	var err error
	if strings.Contains(identifier, "@") {
		user, err = s.userRepo.GetUserByEmail(ctx, strings.ToLower(identifier))
	} else {
		user, err = s.userRepo.GetUserByUsername(ctx, identifier)
	}
	if err != nil {
		s.recordAudit(ctx, AuditMeta{ActorEmail: identifier, IP: clientIP}, "admin_expired_password_reset_failed", "user", "", map[string]any{"reason": "invalid_credentials"}, "failure")
		return nil, ErrInvalidCredentials
	}
	ok, err := ComparePassword(currentPassword, user.PasswordHash)
	if err != nil || !ok {
		return nil, ErrInvalidCredentials
	}
	if user.Status != StatusActive {
		return nil, ErrUserSuspended
	}
	if err := s.validatePasswordForPolicy(ctx, newPassword); err != nil {
		return nil, err
	}

	newHash, err := HashPassword(newPassword, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	now := time.Now().UTC()
	user.PasswordHash = newHash
	user.PasswordChangedAt = &now
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	user.UpdatedAt = now
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, AuditMeta{ActorID: user.ID, ActorEmail: user.Email, IP: clientIP}, "admin_expired_password_reset", "user", user.ID, nil, "success")
	return s.AuthenticateWithOptions(ctx, user.Email, newPassword, clientIP)
}

// UnlockUser clears brute-force lockout counters for an administrator account.
func (s *Service) UnlockUser(ctx context.Context, userID string, meta AuditMeta) (*User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	user.UpdatedAt = time.Now().UTC()
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, meta, "unlock_user", "user", user.ID, map[string]any{"email": user.Email}, "success")
	return s.userRepo.GetUserByID(ctx, user.ID)
}

// CompleteSSOLogin authenticates or optionally auto-provisions an admin user from a verified enterprise OIDC identity.
func (s *Service) CompleteSSOLogin(ctx context.Context, req SSOLoginRequest) (*LoginResult, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Subject == "" || req.Provider == "" {
		return nil, errors.New("SSO email, provider, and subject are required")
	}

	policy := s.securityPolicy(ctx)
	if !IsIPAllowed(req.ClientIP, policy.AllowedAdminIPCIDRs) {
		return nil, ErrIPNotAllowed
	}

	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
		if !req.AutoProvision {
			return nil, ErrForbidden
		}

		randomBytes := make([]byte, 24)
		_, _ = rand.Read(randomBytes)
		pwHash, hashErr := HashPassword("Sso!"+hex.EncodeToString(randomBytes), nil)
		if hashErr != nil {
			return nil, hashErr
		}

		roleID := strings.TrimSpace(req.DefaultRoleID)
		if roleID == "" {
			if viewerRole, rErr := s.roleRepo.GetRoleBySlug(ctx, RoleViewer); rErr == nil {
				roleID = viewerRole.ID
			}
		} else {
			if _, rErr := s.roleRepo.GetRoleByID(ctx, roleID); rErr != nil {
				return nil, rErr
			}
		}

		localPart := strings.SplitN(email, "@", 2)[0]
		username := strings.ReplaceAll(localPart, "+", "_")
		if _, existsErr := s.userRepo.GetUserByUsername(ctx, username); existsErr == nil {
			username = username + "_" + uuid.NewString()[:6]
		}

		now := time.Now().UTC()
		newUser := &User{
			ID:          uuid.New().String(),
			Username:    username,
			Email:       email,
			PasswordHash: pwHash,
			Status:      StatusActive,
			FirstName:   strings.TrimSpace(req.FirstName),
			LastName:    strings.TrimSpace(req.LastName),
			SSOProvider: req.Provider,
			SSOSubject:  req.Subject,
			LastLoginAt: &now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.userRepo.CreateUser(ctx, newUser); err != nil {
			return nil, err
		}
		if roleID != "" {
			if err := s.roleRepo.AssignRoleToUser(ctx, newUser.ID, roleID); err != nil {
				_ = s.userRepo.DeleteUser(ctx, newUser.ID)
				return nil, err
			}
		}
		user, err = s.userRepo.GetUserByID(ctx, newUser.ID)
		if err != nil {
			return nil, err
		}
		s.recordAudit(ctx, AuditMeta{ActorID: user.ID, ActorEmail: user.Email, IP: req.ClientIP}, "admin_sso_provisioned", "user", user.ID, map[string]any{"provider": req.Provider}, "success")
	} else {
		if user.Status != StatusActive {
			return nil, ErrUserSuspended
		}
		now := time.Now().UTC()
		user.SSOProvider = req.Provider
		user.SSOSubject = req.Subject
		user.FailedLoginAttempts = 0
		user.LockedUntil = nil
		user.LastLoginAt = &now
		user.UpdatedAt = now
		_ = s.userRepo.UpdateUser(ctx, user)
		user, _ = s.userRepo.GetUserByID(ctx, user.ID)
	}

	token, err := GenerateAccessToken(user, s.cfg.Auth.JWTSecret, s.sessionTTL(policy))
	if err != nil {
		return nil, fmt.Errorf("failed to generate SSO access token: %w", err)
	}
	s.recordAudit(ctx, AuditMeta{ActorID: user.ID, ActorEmail: user.Email, IP: req.ClientIP}, "admin_sso_login", "user", user.ID, map[string]any{"provider": req.Provider}, "success")
	return &LoginResult{User: user, Token: token}, nil
}

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newM5AuthHarness(t *testing.T, initialPolicy SecurityRuntimeConfig) (*Service, http.Handler, *SecurityRuntimeConfig, *sync.RWMutex) {
	t.Helper()
	repo := NewMemoryAuthRepository()
	cfg := config.DefaultConfig()
	cfg.Auth.JWTSecret = "test-jwt-secret-key-at-least-32-chars!"
	cfg.Auth.SessionSecret = "test-session-secret-key-at-least-32-chars!"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, repo, repo, repo, cfg, logger)
	_, err := svc.SeedDefaultRolesAndAdmin(context.Background(), "superadmin", "superadmin@example.com", "Admin!CMS2026")
	require.NoError(t, err)

	var mu sync.RWMutex
	policyCopy := initialPolicy
	svc.SetSecurityPolicyProvider(func(ctx context.Context) SecurityRuntimeConfig {
		mu.RLock()
		defer mu.RUnlock()
		return policyCopy
	})

	h := NewHandler(svc)
	r := chi.NewRouter()
	r.Use(CSRFMiddleware)
	r.Use(AuthMiddleware(svc))
	h.RegisterRoutes(r)

	return svc, r, &policyCopy, &mu
}

func TestM5Auth_LockoutPasswordPolicyExpirationAndIPAllowlist(t *testing.T) {
	policy := DefaultSecurityRuntimeConfig()
	policy.MaxFailedLogins = 3
	policy.LockoutDurationMinutes = 15
	policy.SessionTimeoutMinutes = 30
	policy.PasswordPolicy = PasswordPolicy{
		MinLength:        10,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireNumber:    true,
		RequireSpecial:   true,
		MaxAgeDays:       30,
	}

	svc, router, policyPtr, mu := newM5AuthHarness(t, policy)
	ctx := context.Background()

	// Dynamic password policy rejects weak password without special character or < 10 chars
	_, err := svc.Register(ctx, RegisterRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "WeakPassword1", // no special char
	})
	assert.ErrorIs(t, err, ErrWeakPassword)

	alice, err := svc.Register(ctx, RegisterRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "Strong!User2026",
	})
	require.NoError(t, err)

	// Brute-force lockout after 3 failed attempts -> 429 on subsequent login (even with RIGHT password!)
	for i := 0; i < 3; i++ {
		badBody := `{"identifier":"alice@example.com","password":"WrongPassword!1"}`
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(badBody))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	}

	goodBody := `{"identifier":"alice@example.com","password":"Strong!User2026"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(goodBody))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), "ACCOUNT_LOCKED")

	// Unlock clears lock immediately
	_, err = svc.UnlockUser(ctx, alice.ID, AuditMeta{ActorID: "admin"})
	require.NoError(t, err)

	// Simulate password older than 30 days -> PASSWORD_EXPIRED (403) + reset-expired-password succeeds
	u, err := svc.userRepo.GetUserByID(ctx, alice.ID)
	require.NoError(t, err)
	expiredAt := time.Now().UTC().Add(-45 * 24 * time.Hour)
	u.PasswordChangedAt = &expiredAt
	require.NoError(t, svc.userRepo.UpdateUser(ctx, u))

	req = httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(goodBody))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "PASSWORD_EXPIRED")

	resetPayload := `{"identifier":"alice@example.com","current_password":"Strong!User2026","new_password":"Next!Strong2026"}`
	req = httptest.NewRequest(http.MethodPost, "/auth/reset-expired-password", strings.NewReader(resetPayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var loginResp struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &loginResp))
	require.NotEmpty(t, loginResp.Token)

	// Admin IP Allowlist blocks disallowed IP on both /auth/me middleware and /auth/login
	mu.Lock()
	policyPtr.AllowedAdminIPCIDRs = []string{"10.10.0.0/16"}
	mu.Unlock()

	meReq := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	meReq.RemoteAddr = "192.0.2.99:5555"
	meReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)
	assert.Equal(t, http.StatusForbidden, meRec.Code)

	meReqAllowed := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	meReqAllowed.RemoteAddr = "10.10.5.42:5555"
	meReqAllowed.Header.Set("Authorization", "Bearer "+loginResp.Token)
	meRecAllowed := httptest.NewRecorder()
	router.ServeHTTP(meRecAllowed, meReqAllowed)
	assert.Equal(t, http.StatusOK, meRecAllowed.Code)
}

func TestM5Auth_TOTPMFAEnrollmentChallengeAndForcedPolicy(t *testing.T) {
	policy := DefaultSecurityRuntimeConfig()
	svc, router, policyPtr, mu := newM5AuthHarness(t, policy)
	ctx := context.Background()

	user, err := svc.Register(ctx, RegisterRequest{
		Username: "bob_mfa",
		Email:    "bob@example.com",
		Password: "Strong!Bob2026",
	})
	require.NoError(t, err)

	// Step 1: Login initially succeeds with JWT
	res, err := svc.AuthenticateWithOptions(ctx, "bob@example.com", "Strong!Bob2026", "127.0.0.1:1234")
	require.NoError(t, err)
	require.NotEmpty(t, res.Token)

	// Step 2: Enroll MFA via HTTP
	req := httptest.NewRequest(http.MethodPost, "/auth/mfa/enroll", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+res.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var enrollBody struct {
		Data MFAEnrollResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &enrollBody))
	require.NotEmpty(t, enrollBody.Data.Secret)
	assert.Contains(t, enrollBody.Data.OTPAuthURI, "otpauth://totp/")

	// Step 3: Confirm MFA with valid TOTP code
	totpCode, err := ComputeTOTPCode(enrollBody.Data.Secret, time.Now().UTC())
	require.NoError(t, err)

	confirmBytes, _ := json.Marshal(map[string]string{"code": totpCode})
	req = httptest.NewRequest(http.MethodPost, "/auth/mfa/confirm", bytes.NewReader(confirmBytes))
	req.Header.Set("Authorization", "Bearer "+res.Token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var confirmBody struct {
		Data MFAConfirmResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &confirmBody))
	assert.True(t, confirmBody.Data.User.MFAEnabled)
	require.Len(t, confirmBody.Data.RecoveryCodes, 8)

	// Step 4: Subsequent login returns mfa_required=true and short-lived mfa_token (NOT usable on /auth/me!)
	loginBytes, _ := json.Marshal(map[string]string{
		"identifier": "bob@example.com",
		"password":   "Strong!Bob2026",
	})
	req = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBytes))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var step1 struct {
		MFARequired bool   `json:"mfa_required"`
		MFAToken    string `json:"mfa_token"`
		Token       string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &step1))
	assert.True(t, step1.MFARequired)
	assert.Empty(t, step1.Token)
	require.NotEmpty(t, step1.MFAToken)

	// MFA challenge token MUST be rejected by normal API routes!
	badMeReq := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	badMeReq.Header.Set("Authorization", "Bearer "+step1.MFAToken)
	badMeRec := httptest.NewRecorder()
	router.ServeHTTP(badMeRec, badMeReq)
	assert.Equal(t, http.StatusUnauthorized, badMeRec.Code)

	// Step 5: Complete MFA login using a single-use backup recovery code
	verifyBytes, _ := json.Marshal(map[string]string{
		"mfa_token": step1.MFAToken,
		"code":      confirmBody.Data.RecoveryCodes[0],
	})
	req = httptest.NewRequest(http.MethodPost, "/auth/mfa/verify", bytes.NewReader(verifyBytes))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var step2 struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &step2))
	require.NotEmpty(t, step2.Token)

	// Re-using the same recovery code on another login challenge must fail
	res2, err := svc.AuthenticateWithOptions(ctx, "bob@example.com", "Strong!Bob2026", "127.0.0.1:1234")
	require.NoError(t, err)
	require.True(t, res2.MFARequired)
	_, err = svc.VerifyMFALogin(ctx, res2.MFAToken, confirmBody.Data.RecoveryCodes[0], "127.0.0.1:1234")
	assert.ErrorIs(t, err, ErrInvalidMFACode)

	// Step 6: EnforceAdminMFA requires un-enrolled admins to complete inline MFA enrollment
	carol, err := svc.Register(ctx, RegisterRequest{
		Username: "carol_admin",
		Email:    "carol@example.com",
		Password: "Strong!Carol2026",
	})
	require.NoError(t, err)

	mu.Lock()
	policyPtr.EnforceAdminMFA = true
	mu.Unlock()

	carolLoginBytes, _ := json.Marshal(map[string]string{
		"identifier": "carol@example.com",
		"password":   "Strong!Carol2026",
	})
	req = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(carolLoginBytes))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var forcedResp struct {
		MFASetupRequired bool   `json:"mfa_setup_required"`
		MFAToken         string `json:"mfa_token"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &forcedResp))
	assert.True(t, forcedResp.MFASetupRequired)
	require.NotEmpty(t, forcedResp.MFAToken)

	// Disabling MFA on bob fails while EnforceAdminMFA is true
	_, err = svc.DisableMFA(ctx, user.ID, "Strong!Bob2026", totpCode)
	assert.Error(t, err)

	// Carol enrolls and confirms using forced mfa_token -> receives full session JWT
	enrollPayload, _ := json.Marshal(map[string]string{"mfa_token": forcedResp.MFAToken})
	req = httptest.NewRequest(http.MethodPost, "/auth/mfa/enroll", bytes.NewReader(enrollPayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var carolEnroll struct {
		Data MFAEnrollResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &carolEnroll))

	carolOTP, err := ComputeTOTPCode(carolEnroll.Data.Secret, time.Now().UTC())
	require.NoError(t, err)
	confirmPayload, _ := json.Marshal(map[string]string{
		"mfa_token": forcedResp.MFAToken,
		"code":      carolOTP,
	})
	req = httptest.NewRequest(http.MethodPost, "/auth/mfa/confirm", bytes.NewReader(confirmPayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var carolConfirm struct {
		Data MFAConfirmResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &carolConfirm))
	assert.True(t, carolConfirm.Data.User.MFAEnabled)
	assert.Equal(t, carol.ID, carolConfirm.Data.User.ID)
	assert.NotEmpty(t, carolConfirm.Data.Token)

	// Step 7: Regenerate recovery codes requires current TOTP code and replaces old codes
	regenBytes, _ := json.Marshal(map[string]string{"code": carolOTP})
	req = httptest.NewRequest(http.MethodPost, "/auth/mfa/regenerate-recovery-codes", bytes.NewReader(regenBytes))
	req.Header.Set("Authorization", "Bearer "+carolConfirm.Data.Token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var regenResp struct {
		Data struct {
			RecoveryCodes []string `json:"recovery_codes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &regenResp))
	require.Len(t, regenResp.Data.RecoveryCodes, 8)
	assert.NotEqual(t, carolConfirm.Data.RecoveryCodes, regenResp.Data.RecoveryCodes)
}

func TestM5Auth_SSOEnforcementAndAutoProvision(t *testing.T) {
	policy := DefaultSecurityRuntimeConfig()
	policy.SSOEnabled = true
	policy.SSOEnforcedDomains = []string{"corp.acme.io"}
	svc, router, _, _ := newM5AuthHarness(t, policy)
	ctx := context.Background()

	// Create password user on SSO-enforced domain -> password login must be blocked
	_, err := svc.Register(ctx, RegisterRequest{
		Username: "sso_user",
		Email:    "dave@corp.acme.io",
		Password: "Strong!Dave2026",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"identifier":"dave@corp.acme.io","password":"Strong!Dave2026"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "SSO_REQUIRED")

	// CompleteSSOLogin succeeds for existing user and also auto-provisions new SSO admin when allowed
	res, err := svc.CompleteSSOLogin(ctx, SSOLoginRequest{
		Provider:      "oidc",
		Subject:       "sub-new-99",
		Email:         "eva@corp.acme.io",
		FirstName:     "Eva",
		LastName:      "SSO",
		AutoProvision: true,
		ClientIP:      "127.0.0.1:4444",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.Token)
	assert.Equal(t, "oidc", res.User.SSOProvider)
	assert.Equal(t, "sub-new-99", res.User.SSOSubject)
	assert.Equal(t, "eva@corp.acme.io", res.User.Email)
}

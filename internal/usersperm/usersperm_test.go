package usersperm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/audit"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedEmail struct {
	TemplateKey string
	To          string
	Vars        map[string]string
}

func setupUsersPermHarness(t *testing.T) (*Service, *auth.Service, *settings.Service, http.Handler, *[]recordedEmail) {
	t.Helper()
	ctx := context.Background()
	cfg := config.DefaultConfig()
	cfg.Auth.JWTSecret = "shared-jwt-secret-key-at-least-32-chars-long"
	cfg.Auth.SessionSecret = "shared-session-secret-key-at-least-32-chars"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	auditSvc := audit.NewService(nil)
	settingsRepo := settings.NewMemoryRepository()
	settingsSvc := settings.NewService(settingsRepo, cfg, auditSvc, settings.RuntimeStatusProvider{CMSVersion: "1.0.0"})

	authRepo := auth.NewMemoryAuthRepository()
	authSvc := auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, logger)
	_, err := authSvc.SeedDefaultRolesAndAdmin(ctx, "superadmin", "admin@cms.test", "Admin!CMS2026")
	require.NoError(t, err)

	var emailMu sync.Mutex
	var sentEmails []recordedEmail

	upRepo := NewMemoryRepository()
	upSvc := NewService(upRepo, settingsSvc, auditSvc, cfg, logger)
	require.NoError(t, upSvc.SeedDefaults(ctx))
	upSvc.SetEmailSender(func(ctx context.Context, templateKey, to string, vars map[string]string) error {
		emailMu.Lock()
		defer emailMu.Unlock()
		sentEmails = append(sentEmails, recordedEmail{TemplateKey: templateKey, To: to, Vars: vars})
		return nil
	})
	upSvc.SetAdminSSOLoginFunc(authSvc.CompleteSSOLogin)

	r := chi.NewRouter()
	r.Use(auth.CSRFMiddleware)
	r.Use(auth.AuthMiddleware(authSvc))
	auth.NewHandler(authSvc).RegisterRoutes(r)
	NewHandler(upSvc).RegisterRoutes(r)

	return upSvc, authSvc, settingsSvc, r, &sentEmails
}

func TestUsersPerm_StrictTokenAndAccountIsolation(t *testing.T) {
	upSvc, authSvc, _, router, _ := setupUsersPermHarness(t)
	ctx := context.Background()

	// Register an App User
	appRes, err := upSvc.Register(ctx, RegisterAppUserRequest{
		Username: "enduser1",
		Email:    "enduser1@client.test",
		Password: "AppUser!Pass2026",
	})
	require.NoError(t, err)
	require.NotEmpty(t, appRes.Token)

	// Get Admin JWT
	_, adminToken, err := authSvc.Authenticate(ctx, "admin@cms.test", "Admin!CMS2026")
	require.NoError(t, err)
	require.NotEmpty(t, adminToken)

	// 1. App User JWT works on /app-auth/me
	req := httptest.NewRequest(http.MethodGet, "/app-auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+appRes.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "enduser1@client.test")

	// 2. App User JWT MUST be rejected by Admin endpoints (/auth/me & /admin/users-permissions/roles)
	for _, adminPath := range []string{"/auth/me", "/admin/users-permissions/roles"} {
		req = httptest.NewRequest(http.MethodGet, adminPath, nil)
		req.Header.Set("Authorization", "Bearer "+appRes.Token)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "expected App JWT rejected on %s", adminPath)
	}

	// 3. Admin JWT MUST be rejected by /app-auth/me (app & admin JWT scopes never cross!)
	req = httptest.NewRequest(http.MethodGet, "/app-auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// 4. App User password cannot authenticate via Admin /auth/login
	req = httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"identifier":"enduser1@client.test","password":"AppUser!Pass2026"}`))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// 5. Admin password cannot authenticate via App /app-auth/login
	req = httptest.NewRequest(http.MethodPost, "/app-auth/login", strings.NewReader(`{"identifier":"admin@cms.test","password":"Admin!CMS2026"}`))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUsersPerm_ConfirmationForgotResetAndRegistrationToggle(t *testing.T) {
	upSvc, _, _, router, sentEmails := setupUsersPermHarness(t)
	ctx := context.Background()

	// Enable email confirmation in AppAuthSettings
	_, err := upSvc.UpdateSettings(ctx, AppAuthSettings{
		AllowRegister:            true,
		EmailConfirmation:        true,
		SessionExpirationMinutes: 60,
	}, settings.AuditMeta{ActorID: "admin"})
	require.NoError(t, err)

	// 1. Register -> unconfirmed, confirmation email recorded, login denied
	regBody := `{"username":"kate_app","email":"kate@example.com","password":"Kate!Pass2026"}`
	req := httptest.NewRequest(http.MethodPost, "/app-auth/register", strings.NewReader(regBody))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var regResp AppAuthResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &regResp))
	assert.True(t, regResp.RequiresConfirmation)
	assert.Empty(t, regResp.Token)
	assert.False(t, regResp.User.Confirmed)
	require.Len(t, *sentEmails, 1)
	assert.Equal(t, "email_confirmation", (*sentEmails)[0].TemplateKey)

	// Extract raw token from confirm_url
	confirmURL, err := url.Parse((*sentEmails)[0].Vars["confirm_url"])
	require.NoError(t, err)
	rawConfirmToken := confirmURL.Query().Get("token")
	require.NotEmpty(t, rawConfirmToken)

	// Login blocked before confirmation
	req = httptest.NewRequest(http.MethodPost, "/app-auth/login", strings.NewReader(`{"identifier":"kate@example.com","password":"Kate!Pass2026"}`))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Confirm email -> activates & returns JWT
	confirmPayload, _ := json.Marshal(map[string]string{"token": rawConfirmToken})
	req = httptest.NewRequest(http.MethodPost, "/app-auth/confirm-email", bytes.NewReader(confirmPayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var confirmResp AppAuthResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &confirmResp))
	assert.True(t, confirmResp.User.Confirmed)
	assert.NotEmpty(t, confirmResp.Token)

	// 2. Forgot password -> reset email dispatched -> reset password updates hash
	req = httptest.NewRequest(http.MethodPost, "/app-auth/forgot-password", strings.NewReader(`{"email":"kate@example.com"}`))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, *sentEmails, 2)
	assert.Equal(t, "password_reset", (*sentEmails)[1].TemplateKey)

	resetURL, err := url.Parse((*sentEmails)[1].Vars["reset_url"])
	require.NoError(t, err)
	rawResetTok := resetURL.Query().Get("token")
	require.NotEmpty(t, rawResetTok)

	resetPayload, _ := json.Marshal(map[string]string{
		"token":        rawResetTok,
		"new_password": "Kate!NewPass2027",
	})
	req = httptest.NewRequest(http.MethodPost, "/app-auth/reset-password", bytes.NewReader(resetPayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Token is single-use: re-submitting fails
	req = httptest.NewRequest(http.MethodPost, "/app-auth/reset-password", bytes.NewReader(resetPayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// New password logs in cleanly
	req = httptest.NewRequest(http.MethodPost, "/app-auth/login", strings.NewReader(`{"identifier":"kate@example.com","password":"Kate!NewPass2027"}`))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// 3. Disable registration -> registration rejected (403)
	_, err = upSvc.UpdateSettings(ctx, AppAuthSettings{AllowRegister: false, SessionExpirationMinutes: 60}, settings.AuditMeta{})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/app-auth/register", strings.NewReader(`{"username":"blocked_reg","email":"b@example.com","password":"Strong!Pass2026"}`))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestUsersPerm_RolesProvidersSSRFAndOIDCPKCEFlow(t *testing.T) {
	upSvc, authSvc, settingsSvc, router, _ := setupUsersPermHarness(t)
	ctx := context.Background()

	_, adminToken, err := authSvc.Authenticate(ctx, "admin@cms.test", "Admin!CMS2026")
	require.NoError(t, err)

	// Built-in roles seeded & protected from deletion
	roles, err := upSvc.ListRoles(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(roles), 2)
	authRole, err := upSvc.repo.GetRoleBySlug(ctx, "authenticated")
	require.NoError(t, err)
	assert.ErrorIs(t, upSvc.DeleteRole(ctx, authRole.ID, settings.AuditMeta{}), ErrSystemAppRole)

	// Production HTTP client blocks SSRF metadata/private URLs on OIDC discovery & provider upsert
	assert.ErrorIs(t, ValidateExternalURL("http://169.254.169.254/latest/meta-data"), ErrUnsafeURL)
	assert.ErrorIs(t, ValidateExternalURL("http://127.0.0.1:8080/internal"), ErrUnsafeURL)
	_, err = upSvc.DiscoverOIDC(ctx, "http://169.254.169.254")
	assert.ErrorIs(t, err, ErrUnsafeURL)

	// Mock OIDC Identity Provider validating PKCE S256 & issuing userinfo
	var expectedChallenge string
	var oidcSrv *httptest.Server
	oidcSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 oidcSrv.URL,
				"authorization_endpoint": oidcSrv.URL + "/authorize",
				"token_endpoint":         oidcSrv.URL + "/token",
				"userinfo_endpoint":      oidcSrv.URL + "/userinfo",
			})
		case "/token":
			require.NoError(t, r.ParseForm())
			verifier := r.FormValue("code_verifier")
			require.NotEmpty(t, verifier)
			sum := sha256.Sum256([]byte(verifier))
			gotChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
			assert.Equal(t, expectedChallenge, gotChallenge)
			assert.Equal(t, "mock-client-secret", r.FormValue("client_secret"))
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "mock-access-tok",
			})
		case "/userinfo":
			assert.Equal(t, "Bearer mock-access-tok", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":        "oidc-sub-77",
				"email":      "sso.admin@corp.acme.io",
				"given_name": "Sam",
				"family_name": "SSO",
			})
		}
	}))
	defer oidcSrv.Close()

	// Allow httptest local server for this service instance ONLY inside test
	upSvc.SetHTTPClient(oidcSrv.Client(), true)

	// Test OIDC discovery endpoint via admin API
	discReq := httptest.NewRequest(http.MethodPost, "/admin/users-permissions/providers/discover", strings.NewReader(`{"issuer_url":"`+oidcSrv.URL+`"}`))
	discReq.Header.Set("Authorization", "Bearer "+adminToken)
	discRec := httptest.NewRecorder()
	router.ServeHTTP(discRec, discReq)
	require.Equal(t, http.StatusOK, discRec.Code)
	assert.Contains(t, discRec.Body.String(), oidcSrv.URL+"/token")

	// Enable Admin OIDC SSO provider via Admin API and verify secret is masked on read
	provPayload, _ := json.Marshal(UpsertOAuthProviderRequest{
		DisplayName:  "Acme Corp OIDC",
		ProviderType: "oidc",
		Scope:        "admin",
		Enabled:      true,
		ClientID:     "acme-client-id",
		ClientSecret: "mock-client-secret",
		AuthURL:      oidcSrv.URL + "/authorize",
		TokenURL:     oidcSrv.URL + "/token",
		UserinfoURL:  oidcSrv.URL + "/userinfo",
		IssuerURL:    oidcSrv.URL,
		Scopes:       "openid email profile",
	})
	putReq := httptest.NewRequest(http.MethodPut, "/admin/users-permissions/providers/admin_oidc", bytes.NewReader(provPayload))
	putReq.Header.Set("Authorization", "Bearer "+adminToken)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)
	require.Equal(t, http.StatusOK, putRec.Code)
	assert.NotContains(t, putRec.Body.String(), "mock-client-secret")
	assert.Contains(t, putRec.Body.String(), settings.SecretPreserveSentinel)

	// Re-save preserving sentinel keeps encrypted secret working
	provPayloadPreserve, _ := json.Marshal(UpsertOAuthProviderRequest{
		DisplayName:  "Acme Corp OIDC v2",
		ProviderType: "oidc",
		Scope:        "admin",
		Enabled:      true,
		ClientID:     "acme-client-id",
		ClientSecret: settings.SecretPreserveSentinel,
		AuthURL:      oidcSrv.URL + "/authorize",
		TokenURL:     oidcSrv.URL + "/token",
		UserinfoURL:  oidcSrv.URL + "/userinfo",
		IssuerURL:    oidcSrv.URL,
	})
	putReq2 := httptest.NewRequest(http.MethodPut, "/admin/users-permissions/providers/admin_oidc", bytes.NewReader(provPayloadPreserve))
	putReq2.Header.Set("Authorization", "Bearer "+adminToken)
	putRec2 := httptest.NewRecorder()
	router.ServeHTTP(putRec2, putReq2)
	require.Equal(t, http.StatusOK, putRec2.Code)

	// Enable Enterprise Admin SSO with domain allowlist + auto-provisioning
	_, err = settingsSvc.UpdateSSOSettings(ctx, settings.SSOSettings{
		Enabled:            true,
		AutoProvisionAdmin: true,
		AllowedDomains:     []string{"corp.acme.io"},
	}, settings.AuditMeta{ActorID: "admin"})
	require.NoError(t, err)

	// Start Admin SSO Authorize
	authzReq := httptest.NewRequest(http.MethodGet, "/auth/sso/admin_oidc/authorize?format=json", nil)
	authzRec := httptest.NewRecorder()
	router.ServeHTTP(authzRec, authzReq)
	require.Equal(t, http.StatusOK, authzRec.Code)

	var authzBody struct {
		Data OAuthAuthorizeResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(authzRec.Body.Bytes(), &authzBody))
	parsedAuthURL, err := url.Parse(authzBody.Data.AuthorizationURL)
	require.NoError(t, err)
	expectedChallenge = parsedAuthURL.Query().Get("code_challenge")
	require.NotEmpty(t, expectedChallenge)
	assert.Equal(t, "S256", parsedAuthURL.Query().Get("code_challenge_method"))

	// Complete Admin SSO Callback -> auto-provisions admin user & returns Admin JWT!
	cbReq := httptest.NewRequest(http.MethodGet, "/auth/sso/admin_oidc/callback?code=auth-code-123&state="+url.QueryEscape(authzBody.Data.State), nil)
	cbRec := httptest.NewRecorder()
	router.ServeHTTP(cbRec, cbReq)
	require.Equal(t, http.StatusOK, cbRec.Code)
	assert.Contains(t, cbRec.Body.String(), "sso.admin@corp.acme.io")
	assert.Contains(t, cbRec.Body.String(), `"token"`)
}


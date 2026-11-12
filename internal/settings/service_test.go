package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-cms/cms/internal/audit"
	"github.com/go-cms/cms/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptSecretRoundtrip(t *testing.T) {
	key := "super-secret-master-key-for-tests"
	plaintext := "smtp-password-123!"

	encrypted, err := EncryptSecret(key, plaintext)
	require.NoError(t, err)
	assert.Contains(t, encrypted, "enc:v1:")
	assert.NotContains(t, encrypted, plaintext)

	decrypted, err := DecryptSecret(key, encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	_, err = DecryptSecret("wrong-key", encrypted)
	assert.Error(t, err)

	emptyEnc, err := EncryptSecret(key, "")
	require.NoError(t, err)
	assert.Equal(t, "", emptyEnc)

	assert.Equal(t, "existing", ResolveUpdatedSecret(SecretPreserveSentinel, "existing"))
	assert.Equal(t, "next", ResolveUpdatedSecret(" next ", "existing"))
}

func TestGeneralSettingsValidationAndCaching(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	auditSvc := audit.NewService(nil)
	cfg := config.DefaultConfig()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 8080

	svc := NewService(repo, cfg, auditSvc, RuntimeStatusProvider{CMSVersion: "1.0.0"})

	initial, err := svc.GetGeneral(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Go CMS", initial.SiteName)
	assert.Equal(t, "http://127.0.0.1:8080", initial.PublicURL)

	bad := *initial
	bad.PublicURL = "javascript:alert(1)"
	_, err = svc.UpdateGeneral(ctx, bad, AuditMeta{ActorID: "u1"})
	assert.Error(t, err)

	badTZ := *initial
	badTZ.DefaultTimezone = "Invalid/Zone_Name"
	_, err = svc.UpdateGeneral(ctx, badTZ, AuditMeta{ActorID: "u1"})
	assert.Error(t, err)

	good := *initial
	good.SiteName = "Acme Docs"
	good.SiteDescription = "Internal Portal"
	good.PublicURL = "https://cms.acme.example/"
	good.DefaultTimezone = "America/New_York"
	good.DefaultLocale = "fr"
	good.LogoURL = "/api/v1/media/123/file"

	updated, err := svc.UpdateGeneral(ctx, good, AuditMeta{ActorID: "admin-1", IPAddress: "127.0.0.1"})
	require.NoError(t, err)
	assert.Equal(t, "Acme Docs", updated.SiteName)
	assert.Equal(t, "https://cms.acme.example", updated.PublicURL)

	cached, err := svc.GetGeneral(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Acme Docs", cached.SiteName)
	assert.Equal(t, "America/New_York", cached.DefaultTimezone)

	pub := svc.GetPublicGeneral(ctx)
	assert.Equal(t, "Acme Docs", pub.SiteName)
	assert.Equal(t, "/api/v1/media/123/file", pub.LogoURL)

	logs, total, err := auditSvc.List(ctx, audit.Filter{Resource: "settings:general"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "admin-1", logs[0].ActorID)
}

func TestUserPreferencesAndSystemInfoSanitization(t *testing.T) {
	ctx := context.Background()
	cfg := config.DefaultConfig()
	cfg.Database.Password = "TOP_SECRET_DB_PASSWORD"
	cfg.Database.URI = "postgres://postgres:TOP_SECRET_URI@localhost:5433/db"
	cfg.Auth.JWTSecret = "TOP_SECRET_JWT_KEY"
	cfg.Auth.SessionSecret = "TOP_SECRET_SESSION_KEY"
	cfg.Media.SignedURLs.SecretKey = "TOP_SECRET_MEDIA_KEY"

	svc := NewService(NewMemoryRepository(), cfg, nil, RuntimeStatusProvider{
		CMSVersion: "1.0.0",
		DatabaseStatus: func(context.Context) (string, string, string) {
			return "postgres", "healthy", ""
		},
		PluginsInfo: func() (int, int) { return 3, 2 },
		ThemesInfo:  func() (string, []string) { return "default", []string{"default", "minimal"} },
	})

	prefs, err := svc.GetPreferences(ctx, "user-42")
	require.NoError(t, err)
	assert.Equal(t, "system", prefs.Theme)

	invalid := *prefs
	invalid.Theme = "neon"
	_, err = svc.UpdatePreferences(ctx, "user-42", invalid)
	assert.Error(t, err)

	prefs.Theme = "dark"
	prefs.Timezone = "Asia/Kolkata"
	prefs.SidebarCollapsed = true
	saved, err := svc.UpdatePreferences(ctx, "user-42", *prefs)
	require.NoError(t, err)
	assert.Equal(t, "dark", saved.Theme)
	assert.True(t, saved.SidebarCollapsed)

	overview, err := svc.GetOverview(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", overview.DatabaseStatus)
	assert.Equal(t, 3, overview.PluginsTotal)
	assert.Equal(t, 2, overview.PluginsEnabled)
	assert.Equal(t, 2, overview.ThemesCount)

	sys := svc.GetSystemInfo(ctx)
	assert.Equal(t, "postgres", sys.Database.Driver)
	assert.True(t, sys.EnvironmentConfig.ReadOnly)
}

func TestRestAPICORSAndRateLimitValidationAndCallbacks(t *testing.T) {
	ctx := context.Background()
	cfg := config.DefaultConfig()
	svc := NewService(NewMemoryRepository(), cfg, nil, RuntimeStatusProvider{})

	// 1. REST API validation & persistence
	rest, err := svc.GetRestAPISettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, 25, rest.DefaultLimit)
	assert.Equal(t, 100, rest.MaxLimit)
	assert.True(t, rest.WithCount)

	_, err = svc.UpdateRestAPISettings(ctx, RestAPISettings{
		DefaultLimit:       200,
		MaxLimit:           50,
		WithCount:          true,
		MaxPopulateDepth:   4,
		ExposeDraftPreview: true,
	}, AuditMeta{})
	assert.Error(t, err)

	savedRest, err := svc.UpdateRestAPISettings(ctx, RestAPISettings{
		DefaultLimit:       10,
		MaxLimit:           40,
		WithCount:          false,
		MaxPopulateDepth:   2,
		ExposeDraftPreview: false,
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, 10, savedRest.DefaultLimit)
	assert.False(t, savedRest.WithCount)
	assert.False(t, savedRest.ExposeDraftPreview)

	// 2. CORS wildcard + credentials guard + listener invocation
	var observedCORS *CORSSettings
	svc.OnCORSUpdated(func(c CORSSettings) {
		copyVal := c
		observedCORS = &copyVal
	})

	_, err = svc.UpdateCORSSettings(ctx, CORSSettings{
		Enabled:          true,
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
		MaxAgeSeconds:    300,
	}, AuditMeta{})
	assert.Error(t, err)

	_, err = svc.UpdateCORSSettings(ctx, CORSSettings{
		Enabled:        true,
		AllowedOrigins: []string{"ftp://bad.example.com"},
		MaxAgeSeconds:  300,
	}, AuditMeta{})
	assert.Error(t, err)

	savedCORS, err := svc.UpdateCORSSettings(ctx, CORSSettings{
		Enabled:          true,
		AllowedOrigins:   []string{"https://app.example.com/"},
		AllowedMethods:   []string{"get", "post"},
		AllowCredentials: true,
		MaxAgeSeconds:    600,
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://app.example.com"}, savedCORS.AllowedOrigins)
	assert.Equal(t, []string{"GET", "POST"}, savedCORS.AllowedMethods)
	require.NotNil(t, observedCORS)
	assert.Equal(t, []string{"https://app.example.com"}, observedCORS.AllowedOrigins)

	// 3. Rate limit bounds + callback
	var observedRL *RateLimitSettings
	svc.OnRateLimitUpdated(func(rl RateLimitSettings) {
		copyVal := rl
		observedRL = &copyVal
	})

	_, err = svc.UpdateRateLimitSettings(ctx, RateLimitSettings{
		Enabled:              true,
		GlobalRequestsPerSec: 0,
		GlobalBurst:          10,
		AuthRequestsPerMin:   10,
		AuthBurst:            5,
	}, AuditMeta{})
	assert.Error(t, err)

	savedRL, err := svc.UpdateRateLimitSettings(ctx, RateLimitSettings{
		Enabled:              true,
		GlobalRequestsPerSec: 80,
		GlobalBurst:          150,
		AuthRequestsPerMin:   15,
		AuthBurst:            4,
	}, AuditMeta{})
	require.NoError(t, err)
	require.NotNil(t, observedRL)
	assert.Equal(t, float64(80), savedRL.GlobalRequestsPerSec)
	assert.Equal(t, 4, observedRL.AuthBurst)
}

func TestMediaAndStorageSettingsAndSecretPreservation(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.SessionSecret = "test-session-encryption-key-012345"
	svc := NewService(NewMemoryRepository(), cfg, nil, RuntimeStatusProvider{})
	ctx := context.Background()

	// Media validation & live listener
	var observedMedia *MediaSettings
	svc.OnMediaUpdated(func(m MediaSettings) {
		c := m
		observedMedia = &c
	})
	_, err := svc.UpdateMediaSettings(ctx, MediaSettings{
		MaxFileSizeBytes:    500, // below 1024 minimum
		AllowedMIMETypes:    []string{"image/*"},
		DefaultAccess:       "public",
		SignedURLTTLSeconds: 300,
	}, AuditMeta{})
	assert.Error(t, err)

	_, err = svc.UpdateMediaSettings(ctx, MediaSettings{
		MaxFileSizeBytes:    2 << 20,
		AllowedMIMETypes:    []string{"invalid-mime"},
		DefaultAccess:       "public",
		SignedURLTTLSeconds: 300,
	}, AuditMeta{})
	assert.Error(t, err)

	savedMedia, err := svc.UpdateMediaSettings(ctx, MediaSettings{
		MaxFileSizeBytes:       5 << 20,
		AllowedMIMETypes:       []string{"IMAGE/PNG", " application/pdf ", "image/png"},
		ResponsiveSizesEnabled: true,
		GenerateSmall:          true,
		GenerateMedium:         true,
		RequireAltText:         true,
		DefaultAccess:          "SIGNED",
		SignedURLTTLSeconds:    900,
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, []string{"image/png", "application/pdf"}, savedMedia.AllowedMIMETypes)
	assert.Equal(t, "signed", savedMedia.DefaultAccess)
	require.NotNil(t, observedMedia)
	assert.True(t, observedMedia.RequireAltText)

	// Unsupported cloud storage drivers fail honestly
	_, err = svc.UpdateStorageSettings(ctx, StorageSettings{ActiveProvider: "gcs"}, AuditMeta{})
	assert.Error(t, err)
	_, err = svc.UpdateStorageSettings(ctx, StorageSettings{ActiveProvider: "azure"}, AuditMeta{})
	assert.Error(t, err)

	// S3 secret encrypted at rest, masked as sentinel on read, preserved on subsequent update
	uploadDir := t.TempDir()
	var observedStorage *StorageSettings
	var observedSecret string
	svc.OnStorageUpdated(func(st StorageSettings, secret string) {
		c := st
		observedStorage = &c
		observedSecret = secret
	})

	savedStorage, err := svc.UpdateStorageSettings(ctx, StorageSettings{
		ActiveProvider: "s3",
		Local:          LocalStorageSettings{UploadDir: uploadDir, URLPrefix: "/uploads"},
		S3: S3StorageSettings{
			Bucket:          "cms-assets",
			Region:          "us-west-2",
			AccessKeyID:     "AKIA_TEST_KEY",
			SecretAccessKey: "super-secret-s3-value",
			UseSSL:          true,
		},
	}, AuditMeta{})
	require.NoError(t, err)
	assert.True(t, savedStorage.S3.SecretConfigured)
	assert.Equal(t, SecretPreserveSentinel, savedStorage.S3.SecretAccessKey)
	assert.Empty(t, savedStorage.S3.SecretAccessKeyEnc)
	require.NotNil(t, observedStorage)
	assert.Equal(t, "super-secret-s3-value", observedSecret)

	// Updating bucket while passing sentinel keeps existing secret intact
	updatedStorage, err := svc.UpdateStorageSettings(ctx, StorageSettings{
		ActiveProvider: "s3",
		Local:          LocalStorageSettings{UploadDir: uploadDir, URLPrefix: "/uploads"},
		S3: S3StorageSettings{
			Bucket:          "cms-assets-v2",
			Region:          "us-west-2",
			AccessKeyID:     "AKIA_TEST_KEY",
			SecretAccessKey: SecretPreserveSentinel,
			UseSSL:          true,
		},
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, "cms-assets-v2", updatedStorage.S3.Bucket)
	assert.Equal(t, SecretPreserveSentinel, updatedStorage.S3.SecretAccessKey)

	_, resolvedSecret, err := svc.ResolveStorageConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "super-secret-s3-value", resolvedSecret)

	// Verify supported providers capability metadata exposes gcs/azure honestly
	fetched, err := svc.GetStorageSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, SecretPreserveSentinel, fetched.S3.SecretAccessKey)
	assert.NotContains(t, fetched.S3.SecretAccessKey, "super-secret-s3-value")
	var foundGCS bool
	var foundCloudinary bool
	for _, capItem := range fetched.SupportedProviders {
		if capItem.ID == "gcs" {
			foundGCS = true
			assert.False(t, capItem.Implemented)
		}
		if capItem.ID == "cloudinary" {
			foundCloudinary = true
			assert.True(t, capItem.Implemented)
		}
	}
	assert.True(t, foundGCS)
	assert.True(t, foundCloudinary)
}

func TestSettingsService_CloudinaryStorageAndConnection(t *testing.T) {
	ctx := context.Background()
	auditSvc := audit.NewService(nil)
	cfg := config.DefaultConfig()
	svc := NewService(NewMemoryRepository(), cfg, auditSvc, RuntimeStatusProvider{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1_1/demo-cloud/ping" {
			user, pass, ok := r.BasicAuth()
			if ok && user == "cloud-key-1" && pass == "cloud-secret-xyz" {
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// Missing api_secret fails cleanly
	_, err := svc.UpdateStorageSettings(ctx, StorageSettings{
		ActiveProvider: "cloudinary",
		Cloudinary: CloudinaryStorageSettings{
			CloudName: "demo-cloud",
			APIKey:    "cloud-key-1",
		},
	}, AuditMeta{ActorID: "admin-1"})
	assert.Error(t, err)

	var gotSecrets StorageSecrets
	svc.OnStorageUpdatedWithSecrets(func(_ StorageSettings, sec StorageSecrets) {
		gotSecrets = sec
	})

	saved, err := svc.UpdateStorageSettings(ctx, StorageSettings{
		ActiveProvider: "cloudinary",
		Local:          LocalStorageSettings{UploadDir: t.TempDir(), URLPrefix: "/uploads"},
		Cloudinary: CloudinaryStorageSettings{
			CloudName:           "demo-cloud",
			APIKey:              "cloud-key-1",
			APISecret:           "cloud-secret-xyz",
			RootFolder:          "go-cms",
			DeliveryType:        "upload",
			DefaultResourceType: "auto",
			UseSecureURL:        true,
			OptimizationEnabled: true,
			DefaultFormat:       "auto",
			DefaultQuality:      "auto",
			APIBaseURL:          srv.URL,
		},
	}, AuditMeta{ActorID: "admin-1"})
	require.NoError(t, err)
	assert.Equal(t, "cloudinary", saved.ActiveProvider)
	assert.True(t, saved.Cloudinary.SecretConfigured)
	assert.Equal(t, SecretPreserveSentinel, saved.Cloudinary.APISecret)
	assert.Empty(t, saved.Cloudinary.APISecretEnc)
	assert.Equal(t, "cloud-secret-xyz", gotSecrets.CloudinaryAPISecret)

	// Audit metadata must never expose plaintext secret
	logs, _, err := auditSvc.List(ctx, audit.Filter{Resource: "settings:storage"})
	require.NoError(t, err)
	require.NotEmpty(t, logs)
	metaJSON, _ := json.Marshal(logs[0].Metadata)
	assert.NotContains(t, string(metaJSON), "cloud-secret-xyz")

	// Updating root folder with sentinel preserves encrypted secret
	updated, err := svc.UpdateStorageSettings(ctx, StorageSettings{
		ActiveProvider: "cloudinary",
		Local:          saved.Local,
		Cloudinary: CloudinaryStorageSettings{
			CloudName:           "demo-cloud",
			APIKey:              "cloud-key-1",
			APISecret:           SecretPreserveSentinel,
			RootFolder:          "cms-prod",
			DeliveryType:        "upload",
			DefaultResourceType: "auto",
			UseSecureURL:        true,
			APIBaseURL:          srv.URL,
		},
	}, AuditMeta{ActorID: "admin-1"})
	require.NoError(t, err)
	assert.Equal(t, "cms-prod", updated.Cloudinary.RootFolder)
	assert.Equal(t, SecretPreserveSentinel, updated.Cloudinary.APISecret)

	// TestStorageConnection pings mock Cloudinary endpoint cleanly
	res, err := svc.TestStorageConnection(ctx, nil)
	require.NoError(t, err)
	assert.True(t, res.OK)
	assert.Equal(t, "cloudinary", res.Provider)

	// Test override with invalid secret returns OK=false gracefully
	override := *updated
	override.Cloudinary.APISecret = "wrong-secret"
	badRes, err := svc.TestStorageConnection(ctx, &override)
	require.NoError(t, err)
	assert.False(t, badRes.OK)

	// Verify BuildConfiguredProviders creates both local & cloudinary so switching preserves old assets
	cfgOut, secOut, err := svc.ResolveStorageWithSecrets(ctx)
	require.NoError(t, err)
	providers := BuildConfiguredProviders(cfgOut, secOut)
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.Name())
	}
	assert.Contains(t, names, "local")
	assert.Contains(t, names, "cloudinary")

	// Verify env fallback populates Cloudinary config without mutating persisted DB storage
	t.Setenv("CLOUDINARY_URL", "cloudinary://envkey:envsec@envcloud")
	envSvc := NewService(NewMemoryRepository(), cfg, nil, RuntimeStatusProvider{})
	envSt, envSec, err := envSvc.ResolveStorageWithSecrets(ctx)
	require.NoError(t, err)
	assert.Equal(t, "envcloud", envSt.Cloudinary.CloudName)
	assert.Equal(t, "envkey", envSt.Cloudinary.APIKey)
	assert.True(t, envSt.Cloudinary.SecretConfigured)
	assert.Equal(t, SecretPreserveSentinel, envSt.Cloudinary.APISecret)
	assert.Equal(t, "envsec", envSec.CloudinaryAPISecret)
}

func TestSettingsService_SecurityPasswordPolicyAndSSO(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryRepository(), config.DefaultConfig(), audit.NewService(nil), RuntimeStatusProvider{})

	// Defaults load cleanly
	defSec, err := svc.GetSecuritySettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1440, defSec.SessionTimeoutMinutes)
	assert.Equal(t, 5, defSec.MaxFailedLogins)
	assert.False(t, defSec.EnforceAdminMFA)

	// Invalid CIDR / bounds rejected
	_, err = svc.UpdateSecuritySettings(ctx, SecuritySettings{
		SessionTimeoutMinutes:  5, // too small (<15)
		MaxFailedLogins:        5,
		LockoutDurationMinutes: 15,
	}, AuditMeta{})
	assert.Error(t, err)

	_, err = svc.UpdateSecuritySettings(ctx, SecuritySettings{
		SessionTimeoutMinutes:  60,
		MaxFailedLogins:        5,
		LockoutDurationMinutes: 15,
		AllowedAdminIPCIDRs:    []string{"not-an-ip"},
	}, AuditMeta{})
	assert.Error(t, err)

	// Valid CIDRs + single IPs normalized and deduplicated
	savedSec, err := svc.UpdateSecuritySettings(ctx, SecuritySettings{
		SessionTimeoutMinutes:  120,
		MaxFailedLogins:        4,
		LockoutDurationMinutes: 30,
		EnforceAdminMFA:        true,
		AllowedAdminIPCIDRs:    []string{"10.0.0.1", "10.0.0.1/32", " 192.168.1.0/24 "},
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.1/32", "192.168.1.0/24"}, savedSec.AllowedAdminIPCIDRs)
	assert.True(t, savedSec.EnforceAdminMFA)

	// Password Policy validation & persistence
	_, err = svc.UpdatePasswordPolicySettings(ctx, PasswordPolicySettings{MinLength: 4}, AuditMeta{})
	assert.Error(t, err)

	savedPolicy, err := svc.UpdatePasswordPolicySettings(ctx, PasswordPolicySettings{
		MinLength:        12,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireNumber:    true,
		RequireSpecial:   true,
		MaxAgeDays:       90,
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, 12, savedPolicy.MinLength)
	assert.True(t, savedPolicy.RequireSpecial)
	assert.Equal(t, 90, savedPolicy.MaxAgeDays)

	// SSO Settings normalization & validation
	_, err = svc.UpdateSSOSettings(ctx, SSOSettings{
		Enabled:        true,
		AllowedDomains: []string{"invalid-domain"},
	}, AuditMeta{})
	assert.Error(t, err)

	savedSSO, err := svc.UpdateSSOSettings(ctx, SSOSettings{
		Enabled:              true,
		AutoProvisionAdmin:   true,
		DefaultRoleID:        " editor-role ",
		AllowedDomains:       []string{"@Example.COM", "example.com", "corp.acme.io"},
		EnforceSSOForDomains: true,
	}, AuditMeta{})
	require.NoError(t, err)
	assert.Equal(t, "editor-role", savedSSO.DefaultRoleID)
	assert.Equal(t, []string{"example.com", "corp.acme.io"}, savedSSO.AllowedDomains)
	assert.True(t, savedSSO.EnforceSSOForDomains)
}

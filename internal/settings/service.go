package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-cms/cms/internal/audit"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/storage"
	"github.com/go-cms/cms/internal/storage/local"
	"github.com/go-cms/cms/internal/storage/s3"
)

// AuditMeta carries request actor context for settings audit logging.
type AuditMeta struct {
	ActorID   string
	ActorType string
	IPAddress string
	UserAgent string
}

// RuntimeStatusProvider supplies live sanitized runtime metrics to Settings Overview & System Info.
type RuntimeStatusProvider struct {
	CMSVersion       string
	DatabaseStatus   func(ctx context.Context) (driver string, status string, errMsg string)
	MigrationsStatus func(ctx context.Context) []MigrationItem
	CacheStatus      func(ctx context.Context) string
	JobsStats        func() map[string]int
	ThemesInfo       func() (active string, installed []string)
	PluginsInfo      func() (total int, enabled int)
}

// Service manages typed persistent settings, caching, validation, and user preferences.
type Service struct {
	repo      Repository
	cfg       *config.Config
	auditSvc  *audit.Service
	startedAt time.Time
	runtime   RuntimeStatusProvider

	mu                 sync.RWMutex
	generalCache       *GeneralSettings
	restAPICache       *RestAPISettings
	corsCache          *CORSSettings
	rateLimitCache     *RateLimitSettings
	mediaCache         *MediaSettings
	storageCache       *StorageSettings
	securityCache      *SecuritySettings
	passwordPolicyCache *PasswordPolicySettings
	ssoCache           *SSOSettings
	corsListeners      []func(CORSSettings)
	rateLimitListeners []func(RateLimitSettings)
	mediaListeners     []func(MediaSettings)
	storageListeners   []func(StorageSettings, string)
}

// NewService constructs a settings Service.
func NewService(repo Repository, cfg *config.Config, auditSvc *audit.Service, runtime RuntimeStatusProvider) *Service {
	if repo == nil {
		repo = NewMemoryRepository()
	}
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &Service{
		repo:      repo,
		cfg:       cfg,
		auditSvc:  auditSvc,
		startedAt: time.Now().UTC(),
		runtime:   runtime,
	}
}

func (s *Service) defaultPublicURL() string {
	host := s.cfg.Server.Host
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", host, s.cfg.Server.Port)
}

// GetGeneral returns persistent GeneralSettings merged with defaults.
func (s *Service) GetGeneral(ctx context.Context) (*GeneralSettings, error) {
	s.mu.RLock()
	if s.generalCache != nil {
		copyVal := *s.generalCache
		s.mu.RUnlock()
		return &copyVal, nil
	}
	s.mu.RUnlock()

	rec, err := s.repo.GetSetting(ctx, KeyGeneral)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			def := DefaultGeneralSettings(s.defaultPublicURL())
			return &def, nil
		}
		return nil, err
	}

	out := DefaultGeneralSettings(s.defaultPublicURL())
	if len(rec.Value) > 0 {
		if err := json.Unmarshal(rec.Value, &out); err != nil {
			return nil, fmt.Errorf("invalid persisted general settings: %w", err)
		}
	}
	out.UpdatedAt = rec.UpdatedAt

	s.mu.Lock()
	cached := out
	s.generalCache = &cached
	s.mu.Unlock()

	return &out, nil
}

// GetPublicGeneral returns non-sensitive public branding information for the UI and public templates.
func (s *Service) GetPublicGeneral(ctx context.Context) PublicGeneralSettings {
	g, err := s.GetGeneral(ctx)
	if err != nil || g == nil {
		def := DefaultGeneralSettings(s.defaultPublicURL())
		g = &def
	}
	return PublicGeneralSettings{
		SiteName:        g.SiteName,
		SiteDescription: g.SiteDescription,
		PublicURL:       g.PublicURL,
		DefaultTimezone: g.DefaultTimezone,
		DefaultLocale:   g.DefaultLocale,
		DateFormat:      g.DateFormat,
		TimeFormat:      g.TimeFormat,
		LogoURL:         g.LogoURL,
		FaviconURL:      g.FaviconURL,
	}
}

// UpdateGeneral validates, persists, refreshes cache, and audits GeneralSettings updates.
func (s *Service) UpdateGeneral(ctx context.Context, input GeneralSettings, meta AuditMeta) (*GeneralSettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	rec, err := s.repo.UpsertSetting(ctx, KeyGeneral, raw, true)
	if err != nil {
		return nil, err
	}
	input.UpdatedAt = rec.UpdatedAt

	s.mu.Lock()
	cached := input
	s.generalCache = &cached
	s.mu.Unlock()

	s.recordAudit(ctx, meta, "update", "settings:general", KeyGeneral, map[string]any{
		"site_name":        input.SiteName,
		"default_timezone": input.DefaultTimezone,
		"default_locale":   input.DefaultLocale,
	})

	return &input, nil
}

// GetPreferences returns persisted per-user UI preferences or defaults if none are stored yet.
func (s *Service) GetPreferences(ctx context.Context, userID string) (*UserPreferences, error) {
	prefs, err := s.repo.GetPreferences(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			def := DefaultUserPreferences(userID)
			return &def, nil
		}
		return nil, err
	}
	return prefs, nil
}

// UpdatePreferences validates and saves per-user UI preferences.
func (s *Service) UpdatePreferences(ctx context.Context, userID string, input UserPreferences) (*UserPreferences, error) {
	input.UserID = userID
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	return s.repo.UpsertPreferences(ctx, &input)
}

// GetOverview collects real live status counts for the Settings Overview dashboard.
func (s *Service) GetOverview(ctx context.Context) (*OverviewResponse, error) {
	gen, err := s.GetGeneral(ctx)
	if err != nil {
		def := DefaultGeneralSettings(s.defaultPublicURL())
		gen = &def
	}

	dbDriver := s.cfg.Database.Driver
	dbStatus := "unknown"
	if s.runtime.DatabaseStatus != nil {
		d, st, _ := s.runtime.DatabaseStatus(ctx)
		if d != "" {
			dbDriver = d
		}
		dbStatus = st
	}

	pluginsTotal, pluginsEnabled := 0, 0
	if s.runtime.PluginsInfo != nil {
		pluginsTotal, pluginsEnabled = s.runtime.PluginsInfo()
	}

	activeTheme := s.cfg.Themes.ActiveTheme
	themesCount := 0
	if s.runtime.ThemesInfo != nil {
		act, list := s.runtime.ThemesInfo()
		if act != "" {
			activeTheme = act
		}
		themesCount = len(list)
	}

	version := s.runtime.CMSVersion
	if version == "" {
		version = "1.0.0"
	}

	return &OverviewResponse{
		SiteName:         gen.SiteName,
		PublicURL:        gen.PublicURL,
		Environment:      s.cfg.Server.Env,
		CMSVersion:       version,
		DatabaseDriver:   dbDriver,
		DatabaseStatus:   dbStatus,
		StorageProvider:  s.cfg.Media.Provider,
		CacheDriver:      s.cfg.Cache.Driver,
		JobsEnabled:      s.cfg.Jobs.Enabled,
		PluginsTotal:     pluginsTotal,
		PluginsEnabled:   pluginsEnabled,
		ThemesCount:      themesCount,
		ActiveTheme:      activeTheme,
		DefaultLocale:    gen.DefaultLocale,
		SignedURLsActive: s.cfg.Media.SignedURLs.Enabled,
	}, nil
}

// GetSystemInfo returns sanitized system diagnostics and read-only environment settings without exposing any credentials.
func (s *Service) GetSystemInfo(ctx context.Context) *SystemInfoResponse {
	resp := &SystemInfoResponse{}

	version := s.runtime.CMSVersion
	if version == "" {
		version = "1.0.0"
	}
	now := time.Now().UTC()

	resp.Application.Name = "Go CMS"
	resp.Application.Version = version
	resp.Application.Environment = s.cfg.Server.Env
	resp.Application.StartedAt = s.startedAt.Format(time.RFC3339)
	resp.Application.UptimeSec = int64(now.Sub(s.startedAt).Seconds())
	resp.Application.GoVersion = runtime.Version()
	resp.Application.OS = runtime.GOOS
	resp.Application.Arch = runtime.GOARCH
	resp.Application.Goroutines = runtime.NumGoroutine()
	resp.Application.Host = s.cfg.Server.Host
	resp.Application.Port = s.cfg.Server.Port

	resp.EnvironmentConfig.ReadOnly = true
	resp.EnvironmentConfig.LoggingLevel = s.cfg.Logging.Level
	resp.EnvironmentConfig.LoggingFormat = s.cfg.Logging.Format
	resp.EnvironmentConfig.CORSOriginsCount = len(s.cfg.Server.CORSAllowedOrigins)
	resp.EnvironmentConfig.TrustedProxies = append([]string(nil), s.cfg.Server.TrustedProxies...)
	resp.EnvironmentConfig.TokenExpiration = s.cfg.Auth.TokenExpiration.String()

	resp.Database.Driver = s.cfg.Database.Driver
	resp.Database.Host = s.cfg.Database.Host
	resp.Database.Port = s.cfg.Database.Port
	resp.Database.Database = s.cfg.Database.Database
	resp.Database.SSLMode = s.cfg.Database.SSLMode
	resp.Database.MaxOpenConns = s.cfg.Database.MaxOpenConns
	resp.Database.MaxIdleConns = s.cfg.Database.MaxIdleConns
	resp.Database.ConnectionNote = "Database credentials and connection URI are controlled by deployment configuration and hidden for security."
	if s.runtime.DatabaseStatus != nil {
		d, st, errMsg := s.runtime.DatabaseStatus(ctx)
		if d != "" {
			resp.Database.Driver = d
		}
		resp.Database.Status = st
		resp.Database.Error = errMsg
	} else {
		resp.Database.Status = "in-memory"
	}
	if s.runtime.MigrationsStatus != nil {
		items := s.runtime.MigrationsStatus(ctx)
		resp.Database.Migrations = items
		for _, item := range items {
			if item.Applied {
				resp.Database.AppliedCount++
			} else {
				resp.Database.PendingCount++
			}
		}
	}

	resp.Storage.Provider = s.cfg.Media.Provider
	resp.Storage.UploadDir = s.cfg.Media.UploadDir
	resp.Storage.SignedURLsEnabled = s.cfg.Media.SignedURLs.Enabled
	resp.Storage.SignedURLAlg = s.cfg.Media.SignedURLs.Algorithm
	resp.Storage.SignedURLExpiry = s.cfg.Media.SignedURLs.Expiration.String()

	resp.Cache.Driver = s.cfg.Cache.Driver
	resp.Cache.DefaultTTL = s.cfg.Cache.DefaultTTL.String()
	resp.Cache.Prefix = s.cfg.Cache.Prefix
	if s.runtime.CacheStatus != nil {
		resp.Cache.Status = s.runtime.CacheStatus(ctx)
	} else {
		resp.Cache.Status = "active"
	}

	resp.Jobs.Enabled = s.cfg.Jobs.Enabled
	resp.Jobs.Concurrency = s.cfg.Jobs.Concurrency
	resp.Jobs.MaxRetries = s.cfg.Jobs.MaxRetries
	resp.Jobs.PollInterval = s.cfg.Jobs.PollInterval.String()
	if s.runtime.JobsStats != nil {
		resp.Jobs.QueueStats = s.runtime.JobsStats()
	}

	resp.Themes.ActiveTheme = s.cfg.Themes.ActiveTheme
	resp.Themes.ThemesDir = s.cfg.Themes.ThemesDir
	resp.Themes.Cache = s.cfg.Themes.Cache
	if s.runtime.ThemesInfo != nil {
		act, list := s.runtime.ThemesInfo()
		if act != "" {
			resp.Themes.ActiveTheme = act
		}
		resp.Themes.Installed = list
	}

	resp.Plugins.PluginsDir = s.cfg.Plugins.PluginsDir
	if s.runtime.PluginsInfo != nil {
		total, enabled := s.runtime.PluginsInfo()
		resp.Plugins.Total = total
		resp.Plugins.Enabled = enabled
	}

	return resp
}

func (s *Service) recordAudit(ctx context.Context, meta AuditMeta, action, resource, targetID string, details map[string]any) {
	if s.auditSvc == nil {
		return
	}
	actorType := meta.ActorType
	if actorType == "" {
		actorType = "admin"
	}
	_, _ = s.auditSvc.Record(ctx, audit.RecordParams{
		ActorID:   meta.ActorID,
		ActorType: actorType,
		Action:    action,
		Resource:  resource,
		TargetID:  targetID,
		Metadata:  details,
		IPAddress: meta.IPAddress,
		UserAgent: meta.UserAgent,
	})
}

func loadTypedSetting[T any](s *Service, ctx context.Context, key string, cachePtr **T, defFn func() T, setUpdatedAt func(*T, time.Time)) (*T, error) {
	s.mu.RLock()
	if *cachePtr != nil {
		copyVal := **cachePtr
		s.mu.RUnlock()
		return &copyVal, nil
	}
	s.mu.RUnlock()

	rec, err := s.repo.GetSetting(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			def := defFn()
			return &def, nil
		}
		return nil, err
	}

	out := defFn()
	if len(rec.Value) > 0 {
		if err := json.Unmarshal(rec.Value, &out); err != nil {
			return nil, fmt.Errorf("failed to decode %s settings: %w", key, err)
		}
	}
	if setUpdatedAt != nil {
		setUpdatedAt(&out, rec.UpdatedAt)
	}

	s.mu.Lock()
	cached := out
	*cachePtr = &cached
	s.mu.Unlock()

	return &out, nil
}

func saveTypedSetting[T any](s *Service, ctx context.Context, key string, value T, setUpdatedAt func(*T, time.Time), cachePtr **T, meta AuditMeta, auditResource string, details map[string]any) (*T, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	rec, err := s.repo.UpsertSetting(ctx, key, raw, false)
	if err != nil {
		return nil, err
	}
	setUpdatedAt(&value, rec.UpdatedAt)

	s.mu.Lock()
	cached := value
	*cachePtr = &cached
	s.mu.Unlock()

	s.recordAudit(ctx, meta, "update", auditResource, key, details)
	return &value, nil
}

// LoadJSONSetting reads a typed JSON settings blob from the settings repository by key.
func (s *Service) LoadJSONSetting(ctx context.Context, key string, out any) (time.Time, bool, error) {
	rec, err := s.repo.GetSetting(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	if len(rec.Value) > 0 && out != nil {
		if err := json.Unmarshal(rec.Value, out); err != nil {
			return time.Time{}, false, fmt.Errorf("failed to decode %s settings: %w", key, err)
		}
	}
	return rec.UpdatedAt, true, nil
}

// SaveJSONSetting serializes, persists, and audits a typed JSON settings blob by key.
func (s *Service) SaveJSONSetting(ctx context.Context, key string, val any, meta AuditMeta, auditResource string, details map[string]any) (time.Time, error) {
	raw, err := json.Marshal(val)
	if err != nil {
		return time.Time{}, err
	}
	rec, err := s.repo.UpsertSetting(ctx, key, raw, false)
	if err != nil {
		return time.Time{}, err
	}
	s.recordAudit(ctx, meta, "update", auditResource, key, details)
	return rec.UpdatedAt, nil
}

// OnCORSUpdated registers a callback invoked immediately whenever CORS settings change.
func (s *Service) OnCORSUpdated(fn func(CORSSettings)) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	s.corsListeners = append(s.corsListeners, fn)
	s.mu.Unlock()
}

// OnRateLimitUpdated registers a callback invoked immediately whenever rate limit settings change.
func (s *Service) OnRateLimitUpdated(fn func(RateLimitSettings)) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	s.rateLimitListeners = append(s.rateLimitListeners, fn)
	s.mu.Unlock()
}

// GetRestAPISettings returns persisted REST API behavior settings or defaults.
func (s *Service) GetRestAPISettings(ctx context.Context) (*RestAPISettings, error) {
	return loadTypedSetting(s, ctx, KeyRestAPI, &s.restAPICache, DefaultRestAPISettings, func(v *RestAPISettings, ts time.Time) { v.UpdatedAt = ts })
}

// UpdateRestAPISettings validates, persists, refreshes cache, and audits REST API settings.
func (s *Service) UpdateRestAPISettings(ctx context.Context, input RestAPISettings, meta AuditMeta) (*RestAPISettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	return saveTypedSetting(s, ctx, KeyRestAPI, input, func(v *RestAPISettings, ts time.Time) { v.UpdatedAt = ts }, &s.restAPICache, meta, "settings:rest_api", map[string]any{
		"default_limit":        input.DefaultLimit,
		"max_limit":            input.MaxLimit,
		"with_count":           input.WithCount,
		"max_populate_depth":   input.MaxPopulateDepth,
		"expose_draft_preview": input.ExposeDraftPreview,
	})
}

// GetCORSSettings returns persisted CORS settings or deployment defaults.
func (s *Service) GetCORSSettings(ctx context.Context) (*CORSSettings, error) {
	return loadTypedSetting(s, ctx, KeyCORS, &s.corsCache, func() CORSSettings {
		return DefaultCORSSettings(s.cfg.Server.CORSAllowedOrigins)
	}, func(v *CORSSettings, ts time.Time) { v.UpdatedAt = ts })
}

// UpdateCORSSettings validates, persists, refreshes live CORS middleware, and audits changes.
func (s *Service) UpdateCORSSettings(ctx context.Context, input CORSSettings, meta AuditMeta) (*CORSSettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	saved, err := saveTypedSetting(s, ctx, KeyCORS, input, func(v *CORSSettings, ts time.Time) { v.UpdatedAt = ts }, &s.corsCache, meta, "settings:cors", map[string]any{
		"enabled":           input.Enabled,
		"allowed_origins":   input.AllowedOrigins,
		"allow_credentials": input.AllowCredentials,
	})
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	listeners := append([]func(CORSSettings){}, s.corsListeners...)
	s.mu.RUnlock()
	for _, fn := range listeners {
		fn(*saved)
	}
	return saved, nil
}

// GetRateLimitSettings returns persisted rate limiting thresholds or defaults.
func (s *Service) GetRateLimitSettings(ctx context.Context) (*RateLimitSettings, error) {
	return loadTypedSetting(s, ctx, KeyRateLimit, &s.rateLimitCache, DefaultRateLimitSettings, func(v *RateLimitSettings, ts time.Time) { v.UpdatedAt = ts })
}

// UpdateRateLimitSettings validates, persists, updates live limiters, and audits changes.
func (s *Service) UpdateRateLimitSettings(ctx context.Context, input RateLimitSettings, meta AuditMeta) (*RateLimitSettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	saved, err := saveTypedSetting(s, ctx, KeyRateLimit, input, func(v *RateLimitSettings, ts time.Time) { v.UpdatedAt = ts }, &s.rateLimitCache, meta, "settings:rate_limit", map[string]any{
		"enabled":                 input.Enabled,
		"global_requests_per_sec": input.GlobalRequestsPerSec,
		"global_burst":            input.GlobalBurst,
		"auth_requests_per_min":   input.AuthRequestsPerMin,
		"auth_burst":              input.AuthBurst,
	})
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	listeners := append([]func(RateLimitSettings){}, s.rateLimitListeners...)
	s.mu.RUnlock()
	for _, fn := range listeners {
		fn(*saved)
	}
	return saved, nil
}

// EncryptionKey returns the master secret key used to encrypt/decrypt sensitive settings values.
func (s *Service) EncryptionKey() string {
	if s.cfg != nil {
		if s.cfg.Auth.SessionSecret != "" {
			return s.cfg.Auth.SessionSecret
		}
		if s.cfg.Auth.JWTSecret != "" {
			return s.cfg.Auth.JWTSecret
		}
	}
	return "default-cms-settings-key"
}

func (s *Service) defaultMediaSettings() MediaSettings {
	var signed bool
	var expSec int
	if s.cfg != nil {
		signed = s.cfg.Media.SignedURLs.Enabled
		expSec = int(s.cfg.Media.SignedURLs.Expiration.Seconds())
	}
	return DefaultMediaSettings(signed, expSec)
}

func (s *Service) defaultStorageSettings() StorageSettings {
	uploadDir := ""
	if s.cfg != nil {
		uploadDir = s.cfg.Media.UploadDir
	}
	return DefaultStorageSettings(uploadDir)
}

// OnMediaUpdated registers a callback invoked immediately whenever media library settings change.
func (s *Service) OnMediaUpdated(fn func(MediaSettings)) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	s.mediaListeners = append(s.mediaListeners, fn)
	s.mu.Unlock()
}

// OnStorageUpdated registers a callback invoked whenever storage provider settings change.
func (s *Service) OnStorageUpdated(fn func(StorageSettings, string)) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	s.storageListeners = append(s.storageListeners, fn)
	s.mu.Unlock()
}

// GetMediaSettings returns persisted media upload and thumbnail configuration or defaults.
func (s *Service) GetMediaSettings(ctx context.Context) (*MediaSettings, error) {
	return loadTypedSetting(s, ctx, KeyMedia, &s.mediaCache, s.defaultMediaSettings, func(v *MediaSettings, ts time.Time) { v.UpdatedAt = ts })
}

// UpdateMediaSettings validates, persists, notifies listeners, and audits media policy changes.
func (s *Service) UpdateMediaSettings(ctx context.Context, input MediaSettings, meta AuditMeta) (*MediaSettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	saved, err := saveTypedSetting(s, ctx, KeyMedia, input, func(v *MediaSettings, ts time.Time) { v.UpdatedAt = ts }, &s.mediaCache, meta, "settings:media", map[string]any{
		"max_file_size_bytes":      input.MaxFileSizeBytes,
		"allowed_mime_types_count": len(input.AllowedMIMETypes),
		"responsive_sizes_enabled": input.ResponsiveSizesEnabled,
		"require_alt_text":         input.RequireAltText,
		"default_access":           input.DefaultAccess,
		"signed_url_ttl_seconds":   input.SignedURLTTLSeconds,
	})
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	listeners := append([]func(MediaSettings){}, s.mediaListeners...)
	s.mu.RUnlock()
	for _, fn := range listeners {
		fn(*saved)
	}
	return saved, nil
}

func (s *Service) loadRawStorageSettings(ctx context.Context) (*StorageSettings, error) {
	return loadTypedSetting(s, ctx, KeyStorage, &s.storageCache, s.defaultStorageSettings, func(v *StorageSettings, ts time.Time) { v.UpdatedAt = ts })
}

// GetStorageSettings returns sanitized storage settings (with S3 secrets replaced by sentinel when configured).
func (s *Service) GetStorageSettings(ctx context.Context) (*StorageSettings, error) {
	raw, err := s.loadRawStorageSettings(ctx)
	if err != nil {
		return nil, err
	}
	sanitized := raw.Sanitized()
	return &sanitized, nil
}

// ResolveStorageConfig returns persisted storage config and decrypted S3 secret access key for runtime wiring.
func (s *Service) ResolveStorageConfig(ctx context.Context) (StorageSettings, string, error) {
	raw, err := s.loadRawStorageSettings(ctx)
	if err != nil {
		return StorageSettings{}, "", err
	}
	secret := ""
	if raw.S3.SecretAccessKeyEnc != "" {
		dec, err := DecryptSecret(s.EncryptionKey(), raw.S3.SecretAccessKeyEnc)
		if err != nil {
			return StorageSettings{}, "", fmt.Errorf("failed to decrypt stored S3 secret_access_key: %w", err)
		}
		secret = dec
	}
	return raw.Sanitized(), secret, nil
}

// UpdateStorageSettings validates, encrypts S3 secret, persists, notifies listeners, and returns sanitized settings.
func (s *Service) UpdateStorageSettings(ctx context.Context, input StorageSettings, meta AuditMeta) (*StorageSettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}

	existing, err := s.loadRawStorageSettings(ctx)
	if err != nil {
		return nil, err
	}

	encKey := s.EncryptionKey()
	newEnc := ""
	decryptedSecret := ""

	switch input.S3.SecretAccessKey {
	case SecretPreserveSentinel:
		newEnc = existing.S3.SecretAccessKeyEnc
		if newEnc != "" {
			decryptedSecret, err = DecryptSecret(encKey, newEnc)
			if err != nil {
				return nil, fmt.Errorf("stored S3 secret cannot be decrypted with current key; please re-enter it")
			}
		}
	case "":
		newEnc = ""
	default:
		decryptedSecret = input.S3.SecretAccessKey
		newEnc, err = EncryptSecret(encKey, decryptedSecret)
		if err != nil {
			return nil, err
		}
	}

	if (input.ActiveProvider == "s3" || input.ActiveProvider == "minio") && newEnc == "" {
		return nil, fmt.Errorf("s3 secret_access_key is required for provider %s", input.ActiveProvider)
	}

	toSave := input
	toSave.SupportedProviders = nil
	toSave.S3.SecretAccessKey = ""
	toSave.S3.SecretAccessKeyEnc = newEnc
	toSave.S3.SecretConfigured = newEnc != ""

	savedRaw, err := saveTypedSetting(s, ctx, KeyStorage, toSave, func(v *StorageSettings, ts time.Time) { v.UpdatedAt = ts }, &s.storageCache, meta, "settings:storage", map[string]any{
		"active_provider":   toSave.ActiveProvider,
		"local_upload_dir":  toSave.Local.UploadDir,
		"s3_bucket":         toSave.S3.Bucket,
		"s3_region":         toSave.S3.Region,
		"s3_endpoint":       toSave.S3.Endpoint,
		"secret_configured": toSave.S3.SecretConfigured,
	})
	if err != nil {
		return nil, err
	}

	sanitized := savedRaw.Sanitized()
	s.mu.RLock()
	listeners := append([]func(StorageSettings, string){}, s.storageListeners...)
	s.mu.RUnlock()
	for _, fn := range listeners {
		fn(sanitized, decryptedSecret)
	}
	return &sanitized, nil
}

// BuildStorageProvider constructs a StorageProvider from validated storage settings and plaintext secret.
func BuildStorageProvider(cfg StorageSettings, decryptedSecret string) (storage.StorageProvider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.ActiveProvider)) {
	case "", "local":
		return local.New(cfg.Local.UploadDir, cfg.Local.URLPrefix)
	case "s3", "minio":
		return s3.New(s3.Config{
			ProviderName:    cfg.ActiveProvider,
			Bucket:          cfg.S3.Bucket,
			Region:          cfg.S3.Region,
			Endpoint:        cfg.S3.Endpoint,
			AccessKeyID:     cfg.S3.AccessKeyID,
			SecretAccessKey: decryptedSecret,
			UseSSL:          cfg.S3.UseSSL,
			ForcePathStyle:  cfg.S3.ForcePathStyle,
			BaseURL:         cfg.S3.BaseURL,
			HTTPClient:      &http.Client{Timeout: 10 * time.Second},
		})
	default:
		return nil, fmt.Errorf("unsupported storage provider %q", cfg.ActiveProvider)
	}
}

// TestStorageConnection instantiates the specified (or currently configured) storage provider and verifies health.
func (s *Service) TestStorageConnection(ctx context.Context, override *StorageSettings) (*StorageTestResult, error) {
	var target StorageSettings
	var secret string

	stored, storedSecret, err := s.ResolveStorageConfig(ctx)
	if err != nil {
		return nil, err
	}
	if override == nil {
		target = stored
		secret = storedSecret
	} else {
		target = *override
		if err := target.NormalizeAndValidate(); err != nil {
			return nil, err
		}
		switch target.S3.SecretAccessKey {
		case SecretPreserveSentinel:
			secret = storedSecret
		case "":
			secret = ""
		default:
			secret = target.S3.SecretAccessKey
		}
		if (target.ActiveProvider == "s3" || target.ActiveProvider == "minio") && secret == "" {
			return nil, fmt.Errorf("s3 secret_access_key is required to test %s connection", target.ActiveProvider)
		}
	}

	provider, err := BuildStorageProvider(target, secret)
	if err != nil {
		return &StorageTestResult{
			OK:       false,
			Provider: target.ActiveProvider,
			Message:  err.Error(),
		}, nil
	}

	start := time.Now()
	var healthErr error
	if hc, ok := provider.(storage.HealthChecker); ok {
		healthErr = hc.CheckHealth(ctx)
	}
	latency := time.Since(start).Milliseconds()
	if healthErr != nil {
		return &StorageTestResult{
			OK:        false,
			Provider:  provider.Name(),
			LatencyMs: latency,
			Message:   healthErr.Error(),
		}, nil
	}
	return &StorageTestResult{
		OK:        true,
		Provider:  provider.Name(),
		LatencyMs: latency,
		Message:   fmt.Sprintf("Storage provider %q is healthy and reachable.", provider.Name()),
	}, nil
}

// GetSecuritySettings returns persisted admin panel security & lockout policy or defaults.
func (s *Service) GetSecuritySettings(ctx context.Context) (*SecuritySettings, error) {
	return loadTypedSetting(s, ctx, KeySecurity, &s.securityCache, DefaultSecuritySettings, func(v *SecuritySettings, ts time.Time) { v.UpdatedAt = ts })
}

// UpdateSecuritySettings validates, persists, and audits admin panel security policy.
func (s *Service) UpdateSecuritySettings(ctx context.Context, input SecuritySettings, meta AuditMeta) (*SecuritySettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	return saveTypedSetting(s, ctx, KeySecurity, input, func(v *SecuritySettings, ts time.Time) { v.UpdatedAt = ts }, &s.securityCache, meta, "settings:security", map[string]any{
		"session_timeout_minutes":  input.SessionTimeoutMinutes,
		"max_failed_logins":        input.MaxFailedLogins,
		"lockout_duration_minutes": input.LockoutDurationMinutes,
		"enforce_admin_mfa":        input.EnforceAdminMFA,
		"allowed_admin_ip_count":   len(input.AllowedAdminIPCIDRs),
	})
}

// GetPasswordPolicySettings returns persisted password complexity & expiry rules or defaults.
func (s *Service) GetPasswordPolicySettings(ctx context.Context) (*PasswordPolicySettings, error) {
	return loadTypedSetting(s, ctx, KeyPasswordPolicy, &s.passwordPolicyCache, DefaultPasswordPolicySettings, func(v *PasswordPolicySettings, ts time.Time) { v.UpdatedAt = ts })
}

// UpdatePasswordPolicySettings validates, persists, and audits password policy rules.
func (s *Service) UpdatePasswordPolicySettings(ctx context.Context, input PasswordPolicySettings, meta AuditMeta) (*PasswordPolicySettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	return saveTypedSetting(s, ctx, KeyPasswordPolicy, input, func(v *PasswordPolicySettings, ts time.Time) { v.UpdatedAt = ts }, &s.passwordPolicyCache, meta, "settings:password_policy", map[string]any{
		"min_length":        input.MinLength,
		"require_uppercase": input.RequireUppercase,
		"require_lowercase": input.RequireLowercase,
		"require_number":    input.RequireNumber,
		"require_special":   input.RequireSpecial,
		"max_age_days":      input.MaxAgeDays,
	})
}

// GetSSOSettings returns persisted enterprise Admin SSO policy or defaults.
func (s *Service) GetSSOSettings(ctx context.Context) (*SSOSettings, error) {
	return loadTypedSetting(s, ctx, KeySSO, &s.ssoCache, DefaultSSOSettings, func(v *SSOSettings, ts time.Time) { v.UpdatedAt = ts })
}

// UpdateSSOSettings validates, persists, and audits Admin SSO policy updates.
func (s *Service) UpdateSSOSettings(ctx context.Context, input SSOSettings, meta AuditMeta) (*SSOSettings, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	return saveTypedSetting(s, ctx, KeySSO, input, func(v *SSOSettings, ts time.Time) { v.UpdatedAt = ts }, &s.ssoCache, meta, "settings:sso", map[string]any{
		"enabled":                 input.Enabled,
		"auto_provision_admin":    input.AutoProvisionAdmin,
		"default_role_id":         input.DefaultRoleID,
		"allowed_domains_count":   len(input.AllowedDomains),
		"enforce_sso_for_domains": input.EnforceSSOForDomains,
	})
}

package settings

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	KeyGeneral        = "general"
	KeyMedia          = "media"
	KeyStorage        = "storage"
	KeyRestAPI        = "rest_api"
	KeyCORS           = "cors"
	KeyRateLimit      = "rate_limit"
	KeySecurity       = "security"
	KeyPasswordPolicy = "password_policy"
	KeyEmail          = "email"
	KeyAppAuth        = "auth_app"
	KeySSO            = "sso"
)

var (
	adminPathRegex = regexp.MustCompile(`^/[a-zA-Z0-9/_-]*$`)
	localeCodeRe   = regexp.MustCompile(`^[a-zA-Z0-9_-]{2,20}$`)
)

// GeneralSettings controls site identity, branding, and regional formatting defaults.
type GeneralSettings struct {
	SiteName        string    `json:"site_name"`
	SiteDescription string    `json:"site_description"`
	PublicURL       string    `json:"public_url"`
	AdminPath       string    `json:"admin_path"`
	DefaultTimezone string    `json:"default_timezone"`
	DefaultLocale   string    `json:"default_locale"`
	DateFormat      string    `json:"date_format"`
	TimeFormat      string    `json:"time_format"`
	LogoURL         string    `json:"logo_url"`
	LogoMediaID     string    `json:"logo_media_id"`
	FaviconURL      string    `json:"favicon_url"`
	FaviconMediaID  string    `json:"favicon_media_id"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DefaultGeneralSettings returns safe defaults derived from server config when available.
func DefaultGeneralSettings(publicURL string) GeneralSettings {
	return GeneralSettings{
		SiteName:        "Go CMS",
		SiteDescription: "Modern Extensible CMS Built in Go",
		PublicURL:       strings.TrimRight(publicURL, "/"),
		AdminPath:       "/admin",
		DefaultTimezone: "UTC",
		DefaultLocale:   "en",
		DateFormat:      "YYYY-MM-DD",
		TimeFormat:      "24h",
	}
}

// PublicGeneralSettings exposes safe public branding fields without admin paths or internal metadata.
type PublicGeneralSettings struct {
	SiteName        string `json:"site_name"`
	SiteDescription string `json:"site_description"`
	PublicURL       string `json:"public_url"`
	DefaultTimezone string `json:"default_timezone"`
	DefaultLocale   string `json:"default_locale"`
	DateFormat      string `json:"date_format"`
	TimeFormat      string `json:"time_format"`
	LogoURL         string `json:"logo_url"`
	FaviconURL      string `json:"favicon_url"`
}

// NormalizeAndValidate validates general settings fields and canonicalizes safe whitespace/URLs.
func (g *GeneralSettings) NormalizeAndValidate() error {
	g.SiteName = strings.TrimSpace(g.SiteName)
	g.SiteDescription = strings.TrimSpace(g.SiteDescription)
	g.PublicURL = strings.TrimRight(strings.TrimSpace(g.PublicURL), "/")
	g.AdminPath = strings.TrimSpace(g.AdminPath)
	g.DefaultTimezone = strings.TrimSpace(g.DefaultTimezone)
	g.DefaultLocale = strings.TrimSpace(g.DefaultLocale)
	g.DateFormat = strings.TrimSpace(g.DateFormat)
	g.TimeFormat = strings.TrimSpace(g.TimeFormat)
	g.LogoURL = strings.TrimSpace(g.LogoURL)
	g.LogoMediaID = strings.TrimSpace(g.LogoMediaID)
	g.FaviconURL = strings.TrimSpace(g.FaviconURL)
	g.FaviconMediaID = strings.TrimSpace(g.FaviconMediaID)

	if len(g.SiteName) < 1 || len(g.SiteName) > 120 {
		return fmt.Errorf("site_name must be between 1 and 120 characters")
	}
	if len(g.SiteDescription) > 500 {
		return fmt.Errorf("site_description cannot exceed 500 characters")
	}
	if g.PublicURL != "" {
		u, err := url.Parse(g.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return fmt.Errorf("public_url must be a valid http or https URL")
		}
	}
	if g.AdminPath == "" {
		g.AdminPath = "/admin"
	}
	if !adminPathRegex.MatchString(g.AdminPath) || strings.Contains(g.AdminPath, "//") {
		return fmt.Errorf("admin_path must start with / and contain only letters, digits, /, -, or _")
	}
	if g.DefaultTimezone == "" {
		g.DefaultTimezone = "UTC"
	}
	if _, err := time.LoadLocation(g.DefaultTimezone); err != nil {
		return fmt.Errorf("default_timezone %q is not a valid IANA timezone", g.DefaultTimezone)
	}
	if g.DefaultLocale == "" {
		g.DefaultLocale = "en"
	}
	if !localeCodeRe.MatchString(g.DefaultLocale) {
		return fmt.Errorf("default_locale must be 2 to 20 valid locale characters")
	}
	allowedDateFormats := []string{"YYYY-MM-DD", "MM/DD/YYYY", "DD/MM/YYYY", "MMM D, YYYY"}
	if g.DateFormat == "" {
		g.DateFormat = "YYYY-MM-DD"
	}
	if !slices.Contains(allowedDateFormats, g.DateFormat) {
		return fmt.Errorf("unsupported date_format %q", g.DateFormat)
	}
	allowedTimeFormats := []string{"12h", "24h"}
	if g.TimeFormat == "" {
		g.TimeFormat = "24h"
	}
	if !slices.Contains(allowedTimeFormats, g.TimeFormat) {
		return fmt.Errorf("time_format must be '12h' or '24h'")
	}
	if err := validateAssetURL("logo_url", g.LogoURL); err != nil {
		return err
	}
	if err := validateAssetURL("favicon_url", g.FaviconURL); err != nil {
		return err
	}
	return nil
}

func validateAssetURL(field, raw string) error {
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return fmt.Errorf("%s must be an application-relative path or valid http/https URL", field)
	}
	return nil
}

// UserPreferences stores individual operator UI preferences in cms_user_preferences.
type UserPreferences struct {
	UserID            string    `json:"user_id"`
	Theme             string    `json:"theme"`
	PreferredLanguage string    `json:"preferred_language"`
	Timezone          string    `json:"timezone"`
	DateFormat        string    `json:"date_format"`
	TimeFormat        string    `json:"time_format"`
	SidebarCollapsed  bool      `json:"sidebar_collapsed"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// DefaultUserPreferences returns baseline UI preferences for a user.
func DefaultUserPreferences(userID string) UserPreferences {
	return UserPreferences{
		UserID:            userID,
		Theme:             "system",
		PreferredLanguage: "en",
		Timezone:          "UTC",
		DateFormat:        "YYYY-MM-DD",
		TimeFormat:        "24h",
		SidebarCollapsed:  false,
	}
}

// NormalizeAndValidate validates user preference inputs.
func (p *UserPreferences) NormalizeAndValidate() error {
	p.Theme = strings.TrimSpace(strings.ToLower(p.Theme))
	p.PreferredLanguage = strings.TrimSpace(p.PreferredLanguage)
	p.Timezone = strings.TrimSpace(p.Timezone)
	p.DateFormat = strings.TrimSpace(p.DateFormat)
	p.TimeFormat = strings.TrimSpace(p.TimeFormat)

	if p.Theme == "" {
		p.Theme = "system"
	}
	if !slices.Contains([]string{"system", "light", "dark"}, p.Theme) {
		return fmt.Errorf("theme must be system, light, or dark")
	}
	if p.PreferredLanguage == "" {
		p.PreferredLanguage = "en"
	}
	if !localeCodeRe.MatchString(p.PreferredLanguage) {
		return fmt.Errorf("preferred_language must be 2 to 20 valid characters")
	}
	if p.Timezone == "" {
		p.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("timezone %q is not a valid IANA timezone", p.Timezone)
	}
	if p.DateFormat == "" {
		p.DateFormat = "YYYY-MM-DD"
	}
	if !slices.Contains([]string{"YYYY-MM-DD", "MM/DD/YYYY", "DD/MM/YYYY", "MMM D, YYYY"}, p.DateFormat) {
		return fmt.Errorf("unsupported date_format %q", p.DateFormat)
	}
	if p.TimeFormat == "" {
		p.TimeFormat = "24h"
	}
	if !slices.Contains([]string{"12h", "24h"}, p.TimeFormat) {
		return fmt.Errorf("time_format must be '12h' or '24h'")
	}
	return nil
}

// OverviewResponse summarizes Settings status without reading stale mock counts.
type OverviewResponse struct {
	SiteName         string `json:"site_name"`
	PublicURL        string `json:"public_url"`
	Environment      string `json:"environment"`
	CMSVersion       string `json:"cms_version"`
	DatabaseDriver   string `json:"database_driver"`
	DatabaseStatus   string `json:"database_status"`
	StorageProvider  string `json:"storage_provider"`
	CacheDriver      string `json:"cache_driver"`
	JobsEnabled      bool   `json:"jobs_enabled"`
	PluginsTotal     int    `json:"plugins_total"`
	PluginsEnabled   int    `json:"plugins_enabled"`
	ThemesCount      int    `json:"themes_count"`
	ActiveTheme      string `json:"active_theme"`
	DefaultLocale    string `json:"default_locale"`
	SignedURLsActive bool   `json:"signed_urls_active"`
}

// MigrationItem summarizes one migration status record for the Database settings view.
type MigrationItem struct {
	Version   string `json:"version"`
	Name      string `json:"name"`
	Applied   bool   `json:"applied"`
	AppliedAt string `json:"applied_at,omitempty"`
}

// SystemInfoResponse exposes sanitized runtime diagnostics and read-only deployment info.
type SystemInfoResponse struct {
	Application struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Environment string `json:"environment"`
		StartedAt   string `json:"started_at"`
		UptimeSec   int64  `json:"uptime_seconds"`
		GoVersion   string `json:"go_version"`
		OS          string `json:"os"`
		Arch        string `json:"arch"`
		Goroutines  int    `json:"goroutines"`
		Host        string `json:"host"`
		Port        int    `json:"port"`
	} `json:"application"`
	EnvironmentConfig struct {
		ReadOnly         bool     `json:"read_only"`
		LoggingLevel     string   `json:"logging_level"`
		LoggingFormat    string   `json:"logging_format"`
		CORSOriginsCount int      `json:"cors_origins_count"`
		TrustedProxies   []string `json:"trusted_proxies"`
		TokenExpiration  string   `json:"token_expiration"`
	} `json:"environment_config"`
	Database struct {
		Driver          string          `json:"driver"`
		Status          string          `json:"status"`
		Host            string          `json:"host"`
		Port            int             `json:"port"`
		Database        string          `json:"database"`
		SSLMode         string          `json:"ssl_mode"`
		MaxOpenConns    int             `json:"max_open_conns"`
		MaxIdleConns    int             `json:"max_idle_conns"`
		AppliedCount    int             `json:"applied_migrations"`
		PendingCount    int             `json:"pending_migrations"`
		Migrations      []MigrationItem `json:"migrations"`
		ConnectionNote  string          `json:"connection_note"`
		Error           string          `json:"error,omitempty"`
	} `json:"database"`
	Storage struct {
		Provider          string `json:"provider"`
		UploadDir         string `json:"upload_dir"`
		SignedURLsEnabled bool   `json:"signed_urls_enabled"`
		SignedURLAlg      string `json:"signed_url_algorithm"`
		SignedURLExpiry   string `json:"signed_url_expiration"`
	} `json:"storage"`
	Cache struct {
		Driver     string `json:"driver"`
		DefaultTTL string `json:"default_ttl"`
		Prefix     string `json:"prefix"`
		Status     string `json:"status"`
	} `json:"cache"`
	Jobs struct {
		Enabled      bool           `json:"enabled"`
		Concurrency  int            `json:"concurrency"`
		MaxRetries   int            `json:"max_retries"`
		PollInterval string         `json:"poll_interval"`
		QueueStats   map[string]int `json:"queue_stats"`
	} `json:"jobs"`
	Themes struct {
		ActiveTheme string   `json:"active_theme"`
		ThemesDir   string   `json:"themes_dir"`
		Cache       bool     `json:"cache"`
		Installed   []string `json:"installed"`
	} `json:"themes"`
	Plugins struct {
		PluginsDir string `json:"plugins_dir"`
		Total      int    `json:"total"`
		Enabled    int    `json:"enabled"`
	} `json:"plugins"`
}

// RestAPISettings configures headless REST API pagination, populate limits, and draft preview behavior.
type RestAPISettings struct {
	DefaultLimit       int       `json:"default_limit"`
	MaxLimit           int       `json:"max_limit"`
	WithCount          bool      `json:"with_count"`
	MaxPopulateDepth   int       `json:"max_populate_depth"`
	ExposeDraftPreview bool      `json:"expose_draft_preview"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// DefaultRestAPISettings returns safe defaults matching CMS REST behavior.
func DefaultRestAPISettings() RestAPISettings {
	return RestAPISettings{
		DefaultLimit:       25,
		MaxLimit:           100,
		WithCount:          true,
		MaxPopulateDepth:   5,
		ExposeDraftPreview: true,
	}
}

// NormalizeAndValidate validates REST API pagination and response guardrails.
func (r *RestAPISettings) NormalizeAndValidate() error {
	if r.DefaultLimit < 1 || r.DefaultLimit > 500 {
		return fmt.Errorf("default_limit must be between 1 and 500")
	}
	if r.MaxLimit < 1 || r.MaxLimit > 1000 {
		return fmt.Errorf("max_limit must be between 1 and 1000")
	}
	if r.DefaultLimit > r.MaxLimit {
		return fmt.Errorf("default_limit (%d) cannot exceed max_limit (%d)", r.DefaultLimit, r.MaxLimit)
	}
	if r.MaxPopulateDepth < 1 || r.MaxPopulateDepth > 8 {
		return fmt.Errorf("max_populate_depth must be between 1 and 8")
	}
	return nil
}

// CORSSettings configures HTTP CORS enforcement applied live on incoming API requests.
type CORSSettings struct {
	Enabled          bool      `json:"enabled"`
	AllowedOrigins   []string  `json:"allowed_origins"`
	AllowedMethods   []string  `json:"allowed_methods"`
	AllowedHeaders   []string  `json:"allowed_headers"`
	ExposedHeaders   []string  `json:"exposed_headers"`
	AllowCredentials bool      `json:"allow_credentials"`
	MaxAgeSeconds    int       `json:"max_age_seconds"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// DefaultCORSSettings builds baseline CORS settings from deployment config while forbidding wildcard+credentials.
func DefaultCORSSettings(cfgOrigins []string) CORSSettings {
	origins := make([]string, 0, len(cfgOrigins))
	for _, o := range cfgOrigins {
		o = strings.TrimSpace(o)
		if o != "" {
			origins = append(origins, o)
		}
	}
	if len(origins) == 0 {
		origins = []string{"*"}
	}
	allowCreds := !slices.Contains(origins, "*")
	return CORSSettings{
		Enabled:          true,
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Tenant-ID"},
		ExposedHeaders:   []string{"Link", "X-Total-Count"},
		AllowCredentials: allowCreds,
		MaxAgeSeconds:    300,
	}
}

// NormalizeAndValidate validates CORS origins, HTTP methods, and prohibits wildcard origin with credentials.
func (c *CORSSettings) NormalizeAndValidate() error {
	cleanList := func(items []string, upper bool) []string {
		out := make([]string, 0, len(items))
		seen := make(map[string]struct{}, len(items))
		for _, item := range items {
			v := strings.TrimSpace(item)
			if v == "" {
				continue
			}
			if upper {
				v = strings.ToUpper(v)
			}
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				out = append(out, v)
			}
		}
		return out
	}

	c.AllowedOrigins = cleanList(c.AllowedOrigins, false)
	for i, origin := range c.AllowedOrigins {
		if origin == "*" {
			continue
		}
		origin = strings.TrimRight(origin, "/")
		c.AllowedOrigins[i] = origin
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("invalid CORS origin %q: must be '*' or scheme://host[:port]", origin)
		}
	}

	if len(c.AllowedMethods) == 0 {
		c.AllowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	} else {
		c.AllowedMethods = cleanList(c.AllowedMethods, true)
	}
	allowedHTTPMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	for _, m := range c.AllowedMethods {
		if !slices.Contains(allowedHTTPMethods, m) {
			return fmt.Errorf("unsupported CORS HTTP method %q", m)
		}
	}

	c.AllowedHeaders = cleanList(c.AllowedHeaders, false)
	c.ExposedHeaders = cleanList(c.ExposedHeaders, false)

	if c.AllowCredentials && slices.Contains(c.AllowedOrigins, "*") {
		return fmt.Errorf("allow_credentials cannot be true when allowed_origins includes wildcard '*'")
	}
	if c.Enabled && len(c.AllowedOrigins) == 0 {
		return fmt.Errorf("at least one allowed_origin is required when CORS is enabled")
	}
	if c.MaxAgeSeconds < 0 || c.MaxAgeSeconds > 86400 {
		return fmt.Errorf("max_age_seconds must be between 0 and 86400")
	}
	return nil
}

// RateLimitSettings configures global IP and authentication endpoint token-bucket rate limits.
type RateLimitSettings struct {
	Enabled              bool      `json:"enabled"`
	GlobalRequestsPerSec float64   `json:"global_requests_per_sec"`
	GlobalBurst          int       `json:"global_burst"`
	AuthRequestsPerMin   int       `json:"auth_requests_per_min"`
	AuthBurst            int       `json:"auth_burst"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// DefaultRateLimitSettings returns baseline server protection thresholds.
func DefaultRateLimitSettings() RateLimitSettings {
	return RateLimitSettings{
		Enabled:              true,
		GlobalRequestsPerSec: 50,
		GlobalBurst:          100,
		AuthRequestsPerMin:   30,
		AuthBurst:            10,
	}
}

// NormalizeAndValidate ensures rate limit parameters remain positive and bounded.
func (rl *RateLimitSettings) NormalizeAndValidate() error {
	if rl.GlobalRequestsPerSec < 1 || rl.GlobalRequestsPerSec > 10000 {
		return fmt.Errorf("global_requests_per_sec must be between 1 and 10000")
	}
	if rl.GlobalBurst < 1 || rl.GlobalBurst > 50000 {
		return fmt.Errorf("global_burst must be between 1 and 50000")
	}
	if rl.AuthRequestsPerMin < 1 || rl.AuthRequestsPerMin > 1000 {
		return fmt.Errorf("auth_requests_per_min must be between 1 and 1000")
	}
	if rl.AuthBurst < 1 || rl.AuthBurst > 1000 {
		return fmt.Errorf("auth_burst must be between 1 and 1000")
	}
	return nil
}

// MediaSettings controls upload validation, thumbnail generation, and asset access defaults.
type MediaSettings struct {
	MaxFileSizeBytes       int64     `json:"max_file_size_bytes"`
	AllowedMIMETypes       []string  `json:"allowed_mime_types"`
	ResponsiveSizesEnabled bool      `json:"responsive_sizes_enabled"`
	GenerateSmall          bool      `json:"generate_small"`
	GenerateMedium         bool      `json:"generate_medium"`
	GenerateLarge          bool      `json:"generate_large"`
	RequireAltText         bool      `json:"require_alt_text"`
	DefaultFolderID        *string   `json:"default_folder_id,omitempty"`
	DefaultAccess          string    `json:"default_access"`
	SignedURLTTLSeconds    int       `json:"signed_url_ttl_seconds"`
	UpdatedAt              time.Time `json:"updated_at,omitempty"`
}

// DefaultMediaSettings builds baseline media library policy values.
func DefaultMediaSettings(defaultSigned bool, signedTTLSeconds int) MediaSettings {
	access := "public"
	if defaultSigned {
		access = "signed"
	}
	if signedTTLSeconds <= 0 {
		signedTTLSeconds = 3600
	}
	return MediaSettings{
		MaxFileSizeBytes:       10 << 20, // 10 MB
		AllowedMIMETypes:       []string{"image/*", "video/mp4", "application/pdf"},
		ResponsiveSizesEnabled: true,
		GenerateSmall:          true,
		GenerateMedium:         true,
		GenerateLarge:          true,
		RequireAltText:         false,
		DefaultAccess:          access,
		SignedURLTTLSeconds:    signedTTLSeconds,
	}
}

// NormalizeAndValidate validates media library limits, MIME patterns, and URL signing defaults.
func (m *MediaSettings) NormalizeAndValidate() error {
	if m.MaxFileSizeBytes < 1024 || m.MaxFileSizeBytes > (512<<20) {
		return fmt.Errorf("max_file_size_bytes must be between 1024 and %d", 512<<20)
	}
	if len(m.AllowedMIMETypes) == 0 {
		return fmt.Errorf("allowed_mime_types must contain at least one MIME type or wildcard pattern")
	}
	cleaned := make([]string, 0, len(m.AllowedMIMETypes))
	seen := make(map[string]struct{}, len(m.AllowedMIMETypes))
	for _, raw := range m.AllowedMIMETypes {
		mt := strings.ToLower(strings.TrimSpace(raw))
		if mt == "" {
			continue
		}
		parts := strings.Split(mt, "/")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("invalid MIME type pattern: %q", raw)
		}
		if _, exists := seen[mt]; !exists {
			seen[mt] = struct{}{}
			cleaned = append(cleaned, mt)
		}
	}
	if len(cleaned) == 0 {
		return fmt.Errorf("allowed_mime_types must contain at least one valid MIME type")
	}
	m.AllowedMIMETypes = cleaned

	m.DefaultAccess = strings.ToLower(strings.TrimSpace(m.DefaultAccess))
	if m.DefaultAccess == "" {
		m.DefaultAccess = "public"
	}
	if m.DefaultAccess != "public" && m.DefaultAccess != "signed" {
		return fmt.Errorf("default_access must be 'public' or 'signed'")
	}
	if m.SignedURLTTLSeconds < 60 || m.SignedURLTTLSeconds > 604800 {
		return fmt.Errorf("signed_url_ttl_seconds must be between 60 and 604800")
	}
	if m.DefaultFolderID != nil {
		trimmed := strings.TrimSpace(*m.DefaultFolderID)
		if trimmed == "" {
			m.DefaultFolderID = nil
		} else {
			m.DefaultFolderID = &trimmed
		}
	}
	return nil
}

// StorageProviderCapability communicates whether a storage provider driver is available in this build.
type StorageProviderCapability struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Implemented bool   `json:"implemented"`
	Reason      string `json:"reason,omitempty"`
}

// LocalStorageSettings configures local filesystem upload storage.
type LocalStorageSettings struct {
	UploadDir string `json:"upload_dir"`
	URLPrefix string `json:"url_prefix"`
}

// S3StorageSettings configures AWS S3 or MinIO-compatible object storage.
type S3StorageSettings struct {
	Bucket             string `json:"bucket"`
	Region             string `json:"region"`
	Endpoint           string `json:"endpoint"`
	AccessKeyID        string `json:"access_key_id"`
	SecretAccessKey    string `json:"secret_access_key,omitempty"`
	SecretAccessKeyEnc string `json:"secret_access_key_enc,omitempty"`
	SecretConfigured   bool   `json:"secret_configured"`
	UseSSL             bool   `json:"use_ssl"`
	ForcePathStyle     bool   `json:"force_path_style"`
	BaseURL            string `json:"base_url"`
}

// StorageTestResult captures connectivity test results for storage providers.
type StorageTestResult struct {
	OK        bool   `json:"ok"`
	Provider  string `json:"provider"`
	LatencyMs int64  `json:"latency_ms"`
	Message   string `json:"message"`
}

// StorageSettings configures the active storage provider and backend parameters.
type StorageSettings struct {
	ActiveProvider     string                      `json:"active_provider"`
	Local              LocalStorageSettings        `json:"local"`
	S3                 S3StorageSettings           `json:"s3"`
	SupportedProviders []StorageProviderCapability `json:"supported_providers,omitempty"`
	UpdatedAt          time.Time                   `json:"updated_at,omitempty"`
}

// SupportedStorageProviders returns the honest capability list for storage backends in this binary.
func SupportedStorageProviders() []StorageProviderCapability {
	return []StorageProviderCapability{
		{ID: "local", Label: "Local Filesystem", Implemented: true},
		{ID: "s3", Label: "Amazon S3", Implemented: true},
		{ID: "minio", Label: "MinIO / S3-Compatible", Implemented: true},
		{ID: "gcs", Label: "Google Cloud Storage", Implemented: false, Reason: "Google Cloud Storage driver is not included in this build."},
		{ID: "azure", Label: "Azure Blob Storage", Implemented: false, Reason: "Azure Blob Storage driver is not included in this build."},
	}
}

// DefaultStorageSettings builds baseline local/S3 storage provider configuration.
func DefaultStorageSettings(uploadDir string) StorageSettings {
	uploadDir = strings.TrimSpace(uploadDir)
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	return StorageSettings{
		ActiveProvider: "local",
		Local: LocalStorageSettings{
			UploadDir: uploadDir,
			URLPrefix: "/uploads",
		},
		S3: S3StorageSettings{
			Region: "us-east-1",
			UseSSL: true,
		},
		SupportedProviders: SupportedStorageProviders(),
	}
}

// Sanitized returns a copy safe for HTTP JSON serialization (no encrypted blob or plaintext secret).
func (s StorageSettings) Sanitized() StorageSettings {
	out := s
	configured := out.S3.SecretAccessKeyEnc != "" || out.S3.SecretConfigured
	out.S3.SecretConfigured = configured
	out.S3.SecretAccessKeyEnc = ""
	if configured {
		out.S3.SecretAccessKey = SecretPreserveSentinel
	} else {
		out.S3.SecretAccessKey = ""
	}
	out.SupportedProviders = SupportedStorageProviders()
	return out
}

// NormalizeAndValidate validates storage provider selection and connection fields.
func (s *StorageSettings) NormalizeAndValidate() error {
	s.ActiveProvider = strings.ToLower(strings.TrimSpace(s.ActiveProvider))
	if s.ActiveProvider == "" {
		s.ActiveProvider = "local"
	}
	switch s.ActiveProvider {
	case "local", "s3", "minio":
	case "gcs", "azure":
		return fmt.Errorf("storage provider %q is not implemented in this server build", s.ActiveProvider)
	default:
		return fmt.Errorf("unsupported storage provider %q", s.ActiveProvider)
	}

	s.Local.UploadDir = strings.TrimSpace(s.Local.UploadDir)
	if s.Local.UploadDir == "" {
		s.Local.UploadDir = "./uploads"
	}
	if strings.Contains(s.Local.UploadDir, "..") || strings.ContainsRune(s.Local.UploadDir, '\x00') {
		return fmt.Errorf("invalid local upload_dir")
	}
	s.Local.URLPrefix = strings.TrimSpace(s.Local.URLPrefix)
	if s.Local.URLPrefix == "" {
		s.Local.URLPrefix = "/uploads"
	}
	if !strings.HasPrefix(s.Local.URLPrefix, "/") {
		return fmt.Errorf("local url_prefix must start with '/'")
	}
	s.Local.URLPrefix = "/" + strings.Trim(s.Local.URLPrefix, "/")
	if s.Local.URLPrefix == "/" {
		s.Local.URLPrefix = "/uploads"
	}

	s.S3.Bucket = strings.TrimSpace(s.S3.Bucket)
	s.S3.Region = strings.TrimSpace(s.S3.Region)
	if s.S3.Region == "" {
		s.S3.Region = "us-east-1"
	}
	s.S3.Endpoint = strings.TrimSpace(s.S3.Endpoint)
	s.S3.AccessKeyID = strings.TrimSpace(s.S3.AccessKeyID)
	s.S3.BaseURL = strings.TrimRight(strings.TrimSpace(s.S3.BaseURL), "/")

	if s.S3.BaseURL != "" {
		u, err := url.Parse(s.S3.BaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("invalid s3 base_url")
		}
	}
	if s.S3.Endpoint != "" && (strings.HasPrefix(s.S3.Endpoint, "http://") || strings.HasPrefix(s.S3.Endpoint, "https://")) {
		u, err := url.Parse(s.S3.Endpoint)
		if err != nil || u.Host == "" {
			return fmt.Errorf("invalid s3 endpoint URL")
		}
	}

	if s.ActiveProvider == "s3" || s.ActiveProvider == "minio" {
		if s.S3.Bucket == "" {
			return fmt.Errorf("s3 bucket is required for provider %s", s.ActiveProvider)
		}
		if s.S3.AccessKeyID == "" {
			return fmt.Errorf("s3 access_key_id is required for provider %s", s.ActiveProvider)
		}
		if s.ActiveProvider == "minio" && s.S3.Endpoint == "" {
			return fmt.Errorf("s3 endpoint is required for MinIO provider")
		}
	}
	return nil
}

// SecuritySettings controls admin session lifetime, brute-force lockout, MFA enforcement, and IP allowlisting.
type SecuritySettings struct {
	SessionTimeoutMinutes  int       `json:"session_timeout_minutes"`
	MaxFailedLogins        int       `json:"max_failed_logins"`
	LockoutDurationMinutes int       `json:"lockout_duration_minutes"`
	EnforceAdminMFA        bool      `json:"enforce_admin_mfa"`
	AllowedAdminIPCIDRs    []string  `json:"allowed_admin_ip_cidrs"`
	UpdatedAt              time.Time `json:"updated_at,omitempty"`
}

// DefaultSecuritySettings returns safe baseline admin panel security defaults.
func DefaultSecuritySettings() SecuritySettings {
	return SecuritySettings{
		SessionTimeoutMinutes:  1440, // 24 hours
		MaxFailedLogins:        5,
		LockoutDurationMinutes: 15,
		EnforceAdminMFA:        false,
		AllowedAdminIPCIDRs:    []string{},
	}
}

// NormalizeAndValidate validates session timeout, lockout thresholds, and CIDR entries.
func (sec *SecuritySettings) NormalizeAndValidate() error {
	if sec.SessionTimeoutMinutes < 15 || sec.SessionTimeoutMinutes > 43200 {
		return fmt.Errorf("session_timeout_minutes must be between 15 and 43200")
	}
	if sec.MaxFailedLogins < 3 || sec.MaxFailedLogins > 50 {
		return fmt.Errorf("max_failed_logins must be between 3 and 50")
	}
	if sec.LockoutDurationMinutes < 1 || sec.LockoutDurationMinutes > 1440 {
		return fmt.Errorf("lockout_duration_minutes must be between 1 and 1440")
	}

	cleaned := make([]string, 0, len(sec.AllowedAdminIPCIDRs))
	seen := make(map[string]struct{}, len(sec.AllowedAdminIPCIDRs))
	for _, raw := range sec.AllowedAdminIPCIDRs {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(item); err != nil {
			ip := net.ParseIP(item)
			if ip == nil {
				return fmt.Errorf("invalid IP or CIDR block %q", raw)
			}
			if ip.To4() != nil {
				item = ip.String() + "/32"
			} else {
				item = ip.String() + "/128"
			}
		}
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			cleaned = append(cleaned, item)
		}
	}
	sec.AllowedAdminIPCIDRs = cleaned
	return nil
}

// PasswordPolicySettings defines complexity and rotation rules enforced for user passwords.
type PasswordPolicySettings struct {
	MinLength        int       `json:"min_length"`
	RequireUppercase bool      `json:"require_uppercase"`
	RequireLowercase bool      `json:"require_lowercase"`
	RequireNumber    bool      `json:"require_number"`
	RequireSpecial   bool      `json:"require_special"`
	MaxAgeDays       int       `json:"max_age_days"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
}

// DefaultPasswordPolicySettings returns default password complexity requirements.
func DefaultPasswordPolicySettings() PasswordPolicySettings {
	return PasswordPolicySettings{
		MinLength:        8,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireNumber:    true,
		RequireSpecial:   false,
		MaxAgeDays:       0,
	}
}

// NormalizeAndValidate ensures password policy bounds remain sane and secure.
func (p *PasswordPolicySettings) NormalizeAndValidate() error {
	if p.MinLength < 8 || p.MinLength > 128 {
		return fmt.Errorf("min_length must be between 8 and 128")
	}
	if p.MaxAgeDays < 0 || p.MaxAgeDays > 3650 {
		return fmt.Errorf("max_age_days must be between 0 and 3650")
	}
	return nil
}

// SSOSettings configures Enterprise Single Sign-On behavior for administrators.
type SSOSettings struct {
	Enabled              bool      `json:"enabled"`
	AutoProvisionAdmin   bool      `json:"auto_provision_admin"`
	DefaultRoleID        string    `json:"default_role_id"`
	AllowedDomains       []string  `json:"allowed_domains"`
	EnforceSSOForDomains bool      `json:"enforce_sso_for_domains"`
	UpdatedAt            time.Time `json:"updated_at,omitempty"`
}

// DefaultSSOSettings returns disabled-by-default SSO configuration.
func DefaultSSOSettings() SSOSettings {
	return SSOSettings{
		Enabled:        false,
		AllowedDomains: []string{},
	}
}

// NormalizeAndValidate normalizes email domain allowlists for Admin SSO.
func (s *SSOSettings) NormalizeAndValidate() error {
	s.DefaultRoleID = strings.TrimSpace(s.DefaultRoleID)
	cleaned := make([]string, 0, len(s.AllowedDomains))
	seen := make(map[string]struct{}, len(s.AllowedDomains))
	for _, raw := range s.AllowedDomains {
		d := strings.ToLower(strings.TrimSpace(raw))
		d = strings.TrimPrefix(d, "@")
		if d == "" {
			continue
		}
		if !strings.Contains(d, ".") || strings.ContainsAny(d, " \t\r\n/") {
			return fmt.Errorf("invalid SSO allowed domain %q", raw)
		}
		if _, ok := seen[d]; !ok {
			seen[d] = struct{}{}
			cleaned = append(cleaned, d)
		}
	}
	s.AllowedDomains = cleaned
	return nil
}

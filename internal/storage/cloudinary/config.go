package cloudinary

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultAPIBaseURL      = "https://api.cloudinary.com"
	DefaultDeliveryBaseURL = "https://res.cloudinary.com"
	DefaultUploadTimeout   = 60 * time.Second
	DefaultHTTPTimeout     = 15 * time.Second
)

var cloudNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
var presetPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)

// Config holds runtime settings for connecting Go CMS to a Cloudinary account.
type Config struct {
	CloudName           string
	APIKey              string
	APISecret           string
	RootFolder          string
	DeliveryType        string // upload, authenticated, private
	DefaultResourceType string // auto, image, video, raw
	UseSecureURL        bool
	UploadPreset        string
	AssetNamingStrategy string // cms_key, original_filename, random_id
	OptimizationEnabled bool
	DefaultFormat       string
	DefaultQuality      string
	MaxWidth            int
	UploadTimeout       time.Duration
	APIBaseURL          string
	DeliveryBaseURL     string
	HTTPClient          *http.Client
	AllowInsecureHTTP   bool // Only for local mock httptest servers in tests
}

// String redacts sensitive credentials so Config can never accidentally leak APISecret or APIKey in logs.
func (c Config) String() string {
	return fmt.Sprintf("cloudinary.Config{CloudName:%q, RootFolder:%q, DeliveryType:%q, DefaultResourceType:%q, UseSecureURL:%v}",
		c.CloudName, c.RootFolder, c.DeliveryType, c.DefaultResourceType, c.UseSecureURL)
}

// NormalizeAndValidate applies safe defaults and validates all configuration parameters.
func (c *Config) NormalizeAndValidate() error {
	c.CloudName = strings.TrimSpace(c.CloudName)
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.APISecret = strings.TrimSpace(c.APISecret)
	c.RootFolder = NormalizeFolderPath(c.RootFolder)
	c.DeliveryType = strings.ToLower(strings.TrimSpace(c.DeliveryType))
	c.DefaultResourceType = strings.ToLower(strings.TrimSpace(c.DefaultResourceType))
	c.UploadPreset = strings.TrimSpace(c.UploadPreset)
	c.AssetNamingStrategy = strings.ToLower(strings.TrimSpace(c.AssetNamingStrategy))
	c.DefaultFormat = strings.ToLower(strings.TrimSpace(c.DefaultFormat))
	c.DefaultQuality = strings.ToLower(strings.TrimSpace(c.DefaultQuality))
	c.APIBaseURL = strings.TrimRight(strings.TrimSpace(c.APIBaseURL), "/")
	c.DeliveryBaseURL = strings.TrimRight(strings.TrimSpace(c.DeliveryBaseURL), "/")

	if !cloudNamePattern.MatchString(c.CloudName) {
		return fmt.Errorf("invalid cloudinary cloud_name")
	}
	if c.APIKey == "" {
		return fmt.Errorf("cloudinary api_key is required")
	}
	if c.APISecret == "" {
		return fmt.Errorf("cloudinary api_secret is required")
	}

	if err := validateFolderSafe(c.RootFolder); err != nil {
		return err
	}

	if c.DeliveryType == "" {
		c.DeliveryType = "upload"
	}
	switch c.DeliveryType {
	case "upload", "authenticated", "private":
	default:
		return fmt.Errorf("invalid cloudinary delivery_type %q", c.DeliveryType)
	}

	if c.DefaultResourceType == "" {
		c.DefaultResourceType = "auto"
	}
	switch c.DefaultResourceType {
	case "auto", "image", "video", "raw":
	default:
		return fmt.Errorf("invalid cloudinary default_resource_type %q", c.DefaultResourceType)
	}

	if c.UploadPreset != "" && !presetPattern.MatchString(c.UploadPreset) {
		return fmt.Errorf("invalid cloudinary upload_preset")
	}

	if c.AssetNamingStrategy == "" {
		c.AssetNamingStrategy = "cms_key"
	}
	switch c.AssetNamingStrategy {
	case "cms_key", "original_filename", "random_id":
	default:
		return fmt.Errorf("invalid cloudinary asset_naming_strategy %q", c.AssetNamingStrategy)
	}

	if c.DefaultFormat != "" && !isValidFormat(c.DefaultFormat) {
		return fmt.Errorf("invalid cloudinary default_format %q", c.DefaultFormat)
	}
	if c.DefaultQuality != "" && !isValidQuality(c.DefaultQuality) {
		return fmt.Errorf("invalid cloudinary default_quality %q", c.DefaultQuality)
	}
	if c.MaxWidth < 0 || c.MaxWidth > 10000 {
		return fmt.Errorf("cloudinary max_width must be between 0 and 10000")
	}

	if c.APIBaseURL == "" {
		c.APIBaseURL = DefaultAPIBaseURL
	}
	if err := validateEndpointURL(c.APIBaseURL, c.AllowInsecureHTTP); err != nil {
		return fmt.Errorf("invalid api_base_url: %w", err)
	}

	if c.DeliveryBaseURL == "" {
		if c.UseSecureURL {
			c.DeliveryBaseURL = DefaultDeliveryBaseURL
		} else {
			c.DeliveryBaseURL = "http://res.cloudinary.com"
		}
	}
	if err := validateEndpointURL(c.DeliveryBaseURL, !c.UseSecureURL || c.AllowInsecureHTTP); err != nil {
		return fmt.Errorf("invalid delivery_base_url: %w", err)
	}

	if c.UploadTimeout <= 0 {
		c.UploadTimeout = DefaultUploadTimeout
	}

	return nil
}

func validateEndpointURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("malformed URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	if allowHTTP && u.Scheme == "http" {
		return nil
	}
	return fmt.Errorf("unsupported scheme %q (HTTPS required)", u.Scheme)
}

// NormalizeFolderPath cleans slashes without silently rewriting traversal tokens.
func NormalizeFolderPath(folder string) string {
	f := strings.TrimSpace(folder)
	f = strings.Trim(f, "/")
	for strings.Contains(f, "//") {
		f = strings.ReplaceAll(f, "//", "/")
	}
	return f
}

func validateFolderSafe(folder string) error {
	if folder == "" {
		return nil
	}
	if strings.ContainsRune(folder, '\x00') || strings.Contains(folder, "\\") {
		return fmt.Errorf("cloudinary root_folder contains invalid characters")
	}
	for _, seg := range strings.Split(folder, "/") {
		if seg == "." || seg == ".." || seg == "" {
			return fmt.Errorf("cloudinary root_folder contains invalid path segment %q", seg)
		}
	}
	return nil
}


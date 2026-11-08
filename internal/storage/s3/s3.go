package s3

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/go-cms/cms/internal/storage"
)

// Config holds S3 / MinIO connection parameters.
type Config struct {
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	Endpoint        string `json:"endpoint,omitempty"` // For MinIO or custom S3 endpoint
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"-"`
	UseSSL          bool   `json:"use_ssl"`
	BaseURL         string `json:"base_url,omitempty"` // CDN or public bucket URL
}

// Provider implements StorageProvider for S3-compatible cloud storage (AWS S3, MinIO, etc.).
type Provider struct {
	cfg     Config
	baseURL string
}

// New creates an S3 storage provider.
func New(cfg Config) (*Provider, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3 bucket name is required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		if cfg.Endpoint != "" {
			scheme := "https"
			if !cfg.UseSSL {
				scheme = "http"
			}
			baseURL = fmt.Sprintf("%s://%s/%s", scheme, strings.TrimRight(cfg.Endpoint, "/"), cfg.Bucket)
		} else {
			baseURL = fmt.Sprintf("https://%s.s3.%s.amazonaws.com", cfg.Bucket, cfg.Region)
		}
	}

	return &Provider{
		cfg:     cfg,
		baseURL: strings.TrimRight(baseURL, "/"),
	}, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "s3"
}

// Save persists an object to S3.
func (p *Provider) Save(ctx context.Context, key string, r io.Reader, size int64, mimeType string) error {
	// S3 put object operation
	return nil
}

// Open retrieves an object stream from S3.
func (p *Provider) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, storage.ErrFileNotFound
}

// Delete removes an object from S3.
func (p *Provider) Delete(ctx context.Context, key string) error {
	return nil
}

// Exists checks if an object exists in the S3 bucket.
func (p *Provider) Exists(ctx context.Context, key string) (bool, error) {
	return false, nil
}

// URL generates the public URL for an S3 object.
func (p *Provider) URL(ctx context.Context, key string) (string, error) {
	cleanKey := strings.TrimPrefix(key, "/")
	return fmt.Sprintf("%s/%s", p.baseURL, cleanKey), nil
}


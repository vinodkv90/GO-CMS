package s3

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/storage"
)

// Config holds S3 / MinIO connection parameters.
type Config struct {
	ProviderName    string       `json:"provider_name,omitempty"` // "s3" or "minio"
	Bucket          string       `json:"bucket"`
	Region          string       `json:"region"`
	Endpoint        string       `json:"endpoint,omitempty"` // For MinIO or custom S3-compatible endpoint
	AccessKeyID     string       `json:"access_key_id"`
	SecretAccessKey string       `json:"-"`
	UseSSL          bool         `json:"use_ssl"`
	ForcePathStyle  bool         `json:"force_path_style"`
	BaseURL         string       `json:"base_url,omitempty"` // CDN or public bucket URL
	HTTPClient      *http.Client `json:"-"`
}

// Provider implements storage.StorageProvider for AWS S3 and MinIO using AWS SigV4 HTTP requests.
type Provider struct {
	cfg            Config
	name           string
	endpointURL    *url.URL
	forcePathStyle bool
	baseURL        string
	client         *http.Client
}

// New creates an S3/MinIO storage provider and validates endpoint/credentials.
func New(cfg Config) (*Provider, error) {
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.Region = strings.TrimSpace(cfg.Region)
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")

	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3 bucket name is required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	name := strings.ToLower(strings.TrimSpace(cfg.ProviderName))
	if name == "" {
		if cfg.Endpoint != "" {
			name = "minio"
		} else {
			name = "s3"
		}
	}

	var parsedEndpoint *url.URL
	forcePathStyle := cfg.ForcePathStyle || cfg.Endpoint != ""
	if cfg.Endpoint != "" {
		rawEP := cfg.Endpoint
		if !strings.HasPrefix(rawEP, "http://") && !strings.HasPrefix(rawEP, "https://") {
			scheme := "https"
			if !cfg.UseSSL {
				scheme = "http"
			}
			rawEP = scheme + "://" + rawEP
		}
		u, err := url.Parse(rawEP)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("invalid s3 endpoint %q", cfg.Endpoint)
		}
		parsedEndpoint = u
	} else {
		rawEP := fmt.Sprintf("https://%s.s3.%s.amazonaws.com", cfg.Bucket, cfg.Region)
		u, err := url.Parse(rawEP)
		if err != nil {
			return nil, fmt.Errorf("invalid s3 URL: %w", err)
		}
		parsedEndpoint = u
		forcePathStyle = false
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		if forcePathStyle {
			baseURL = strings.TrimRight(parsedEndpoint.String(), "/") + "/" + url.PathEscape(cfg.Bucket)
		} else {
			baseURL = strings.TrimRight(parsedEndpoint.String(), "/")
		}
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &Provider{
		cfg:            cfg,
		name:           name,
		endpointURL:    parsedEndpoint,
		forcePathStyle: forcePathStyle,
		baseURL:        strings.TrimRight(baseURL, "/"),
		client:         client,
	}, nil
}

// Name returns the active provider identifier ("s3" or "minio").
func (p *Provider) Name() string {
	return p.name
}

func validateS3Key(key string) (string, error) {
	if strings.ContainsRune(key, 0) || strings.Contains(key, "\\") {
		return "", storage.ErrPathTraversal
	}
	trimmed := strings.TrimPrefix(strings.TrimSpace(key), "/")
	if trimmed == "" {
		return "", storage.ErrInvalidPath
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == ".." || seg == "." {
			return "", storage.ErrPathTraversal
		}
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+trimmed), "/")
	if cleaned == "" || cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return "", storage.ErrPathTraversal
	}
	return cleaned, nil
}

func escapeS3Path(rawPath string) string {
	parts := strings.Split(rawPath, "/")
	for i, part := range parts {
		parts[i] = awsURIEscape(part, false)
	}
	return strings.Join(parts, "/")
}

func awsURIEscape(s string, escapeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '~' || c == '.' ||
			(!escapeSlash && c == '/') {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func (p *Provider) objectURL(key string, query url.Values) (*url.URL, string) {
	u := *p.endpointURL
	basePath := strings.TrimRight(u.Path, "/")

	var objectPath string
	if p.forcePathStyle {
		if key != "" {
			objectPath = basePath + "/" + p.cfg.Bucket + "/" + key
		} else {
			objectPath = basePath + "/" + p.cfg.Bucket
		}
	} else {
		if key != "" {
			objectPath = basePath + "/" + key
		} else {
			objectPath = basePath + "/"
		}
	}
	canonicalURI := escapeS3Path(objectPath)
	if !strings.HasPrefix(canonicalURI, "/") {
		canonicalURI = "/" + canonicalURI
	}
	u.Path = objectPath
	u.RawPath = canonicalURI
	if len(query) > 0 {
		u.RawQuery = canonicalQueryString(query)
	} else {
		u.RawQuery = ""
	}
	return &u, canonicalURI
}

func canonicalQueryString(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var pairs []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		ek := awsURIEscape(k, true)
		for _, v := range vals {
			pairs = append(pairs, ek+"="+awsURIEscape(v, true))
		}
	}
	return strings.Join(pairs, "&")
}

func (p *Provider) newSignedRequest(ctx context.Context, method, key string, query url.Values, body []byte, extraHeaders map[string]string) (*http.Request, error) {
	u, canonicalURI := p.objectURL(key, query)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])

	req.Header.Set("Host", u.Host)
	req.Host = u.Host
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	for k, v := range extraHeaders {
		if k != "" && v != "" {
			req.Header.Set(k, v)
		}
	}

	if p.cfg.AccessKeyID == "" || p.cfg.SecretAccessKey == "" {
		return req, nil
	}

	// Build canonical headers (host + all x-amz-* headers sorted)
	headersToSign := map[string]string{
		"host":                 strings.TrimSpace(u.Host),
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
	}
	for k, v := range extraHeaders {
		lk := strings.ToLower(strings.TrimSpace(k))
		if strings.HasPrefix(lk, "x-amz-") {
			headersToSign[lk] = strings.TrimSpace(v)
		}
	}

	headerKeys := make([]string, 0, len(headersToSign))
	for k := range headersToSign {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)

	var canonHeaders strings.Builder
	for _, k := range headerKeys {
		canonHeaders.WriteString(k)
		canonHeaders.WriteByte(':')
		canonHeaders.WriteString(headersToSign[k])
		canonHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(headerKeys, ";")

	canonicalReq := strings.Join([]string{
		method,
		canonicalURI,
		u.RawQuery,
		canonHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	canonHash := sha256.Sum256([]byte(canonicalReq))
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, p.cfg.Region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(canonHash[:]),
	}, "\n")

	sigKey := deriveSigningKey(p.cfg.SecretAccessKey, dateStamp, p.cfg.Region, "s3")
	signature := hex.EncodeToString(hmacSHA256(sigKey, stringToSign))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		p.cfg.AccessKeyID, scope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
	return req, nil
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func deriveSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

// CheckHealth verifies bucket access and credentials with ListObjectsV2 (max-keys=1).
func (p *Provider) CheckHealth(ctx context.Context) error {
	q := url.Values{
		"list-type": []string{"2"},
		"max-keys":  []string{"1"},
	}
	req, err := p.newSignedRequest(ctx, http.MethodGet, "", q, nil, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("s3 connection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("s3 health check failed (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// Save persists an object to S3/MinIO via SigV4 PutObject.
func (p *Provider) Save(ctx context.Context, key string, r io.Reader, size int64, mimeType string) error {
	cleanKey, err := validateS3Key(key)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read payload for s3 upload: %w", err)
	}
	headers := map[string]string{}
	if mimeType != "" {
		headers["Content-Type"] = mimeType
	}
	req, err := p.newSignedRequest(ctx, http.MethodPut, cleanKey, nil, data, headers)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%w: put object status %d: %s", storage.ErrStorageOperation, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// Open retrieves an object stream from S3/MinIO via GetObject.
func (p *Provider) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	cleanKey, err := validateS3Key(key)
	if err != nil {
		return nil, err
	}
	req, err := p.newSignedRequest(ctx, http.MethodGet, cleanKey, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, storage.ErrFileNotFound
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return nil, fmt.Errorf("%w: get object status %d: %s", storage.ErrStorageOperation, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return resp.Body, nil
}

// Delete removes an object from S3/MinIO via DeleteObject.
func (p *Provider) Delete(ctx context.Context, key string) error {
	cleanKey, err := validateS3Key(key)
	if err != nil {
		return err
	}
	req, err := p.newSignedRequest(ctx, http.MethodDelete, cleanKey, nil, nil, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%w: delete object status %d: %s", storage.ErrStorageOperation, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// Exists checks if an object exists in the bucket via HeadObject.
func (p *Provider) Exists(ctx context.Context, key string) (bool, error) {
	cleanKey, err := validateS3Key(key)
	if err != nil {
		return false, err
	}
	req, err := p.newSignedRequest(ctx, http.MethodHead, cleanKey, nil, nil, nil)
	if err != nil {
		return false, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("%w: head object status %d", storage.ErrStorageOperation, resp.StatusCode)
	}
	return true, nil
}

// URL generates the public URL for an S3/MinIO object.
func (p *Provider) URL(_ context.Context, key string) (string, error) {
	cleanKey, err := validateS3Key(key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", p.baseURL, cleanKey), nil
}

// Move moves an object (CopyObject + DeleteObject) or prefix of objects cleanly.
func (p *Provider) Move(ctx context.Context, srcKey, dstKey string) error {
	cleanSrc, err := validateS3Key(srcKey)
	if err != nil {
		return err
	}
	cleanDst, err := validateS3Key(dstKey)
	if err != nil {
		return err
	}

	exists, err := p.Exists(ctx, cleanSrc)
	if err == nil && exists {
		return p.moveSingleObject(ctx, cleanSrc, cleanDst)
	}

	prefix := strings.TrimSuffix(cleanSrc, "/") + "/"
	keys, err := p.listKeys(ctx, prefix)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return storage.ErrFileNotFound
	}
	dstBase := strings.TrimSuffix(cleanDst, "/") + "/"
	for _, k := range keys {
		rel := strings.TrimPrefix(k, prefix)
		if err := p.moveSingleObject(ctx, k, dstBase+rel); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) moveSingleObject(ctx context.Context, cleanSrc, cleanDst string) error {
	copySource := "/" + url.PathEscape(p.cfg.Bucket) + "/" + escapeS3Path(cleanSrc)
	req, err := p.newSignedRequest(ctx, http.MethodPut, cleanDst, nil, nil, map[string]string{
		"X-Amz-Copy-Source": copySource,
	})
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: copy failed: %v", storage.ErrStorageOperation, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return storage.ErrFileNotFound
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%w: copy status %d: %s", storage.ErrStorageOperation, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return p.Delete(ctx, cleanSrc)
}

type listBucketResult struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	IsTruncated           bool     `xml:"IsTruncated"`
	NextContinuationToken string   `xml:"NextContinuationToken"`
	Contents              []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
}

func (p *Provider) listKeys(ctx context.Context, prefix string) ([]string, error) {
	var allKeys []string
	token := ""
	for {
		q := url.Values{
			"list-type": []string{"2"},
			"prefix":    []string{prefix},
		}
		if token != "" {
			q.Set("continuation-token", token)
		}
		req, err := p.newSignedRequest(ctx, http.MethodGet, "", q, nil, nil)
		if err != nil {
			return nil, err
		}
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%w: list objects failed: %v", storage.ErrStorageOperation, err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("%w: failed reading list response: %v", storage.ErrStorageOperation, readErr)
		}
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("%w: list objects status %d: %s", storage.ErrStorageOperation, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var parsed listBucketResult
		if err := xml.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("%w: invalid list XML response: %v", storage.ErrStorageOperation, err)
		}
		for _, item := range parsed.Contents {
			if item.Key != "" {
				allKeys = append(allKeys, item.Key)
			}
		}
		if !parsed.IsTruncated || parsed.NextContinuationToken == "" {
			break
		}
		token = parsed.NextContinuationToken
	}
	return allKeys, nil
}

// DeletePrefix deletes all objects matching prefix in S3/MinIO.
func (p *Provider) DeletePrefix(ctx context.Context, prefix string) error {
	cleanPrefix, err := validateS3Key(prefix)
	if err != nil {
		return err
	}
	keys, err := p.listKeys(ctx, cleanPrefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := p.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

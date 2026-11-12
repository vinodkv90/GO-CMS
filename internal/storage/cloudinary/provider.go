package cloudinary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-cms/cms/internal/storage"
)

const maxAPIResponseBytes = 2 << 20 // 2 MiB max metadata response

// Provider implements storage.StorageProvider and optional capability/media interfaces for Cloudinary.
type Provider struct {
	cfg        Config
	httpClient *http.Client

	mu    sync.RWMutex
	cache map[string]cachedAsset
}

type cachedAsset struct {
	PublicID     string
	ResourceType string
	DeliveryType string
	Version      int64
	Format       string
	SecureURL    string
}

var (
	_ storage.StorageProvider         = (*Provider)(nil)
	_ storage.HealthChecker           = (*Provider)(nil)
	_ storage.CapabilityReporter      = (*Provider)(nil)
	_ storage.RichUploader            = (*Provider)(nil)
	_ storage.AssetRefOperator        = (*Provider)(nil)
	_ storage.TransformURLGenerator   = (*Provider)(nil)
	_ storage.SignedAssetURLGenerator = (*Provider)(nil)
)

// New validates cfg and creates a ready Cloudinary storage provider.
func New(cfg Config) (*Provider, error) {
	if err := cfg.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.UploadTimeout
		if timeout <= 0 {
			timeout = DefaultUploadTimeout
		}
		client = &http.Client{
			Timeout: timeout,
		}
	}
	return &Provider{
		cfg:        cfg,
		httpClient: client,
		cache:      make(map[string]cachedAsset),
	}, nil
}

func (p *Provider) Name() string {
	return "cloudinary"
}

func (p *Provider) Capabilities() storage.ProviderCapabilities {
	return storage.ProviderCapabilities{
		Provider:              "cloudinary",
		SupportsHLS:           false,
		SupportsSignedURLs:    true,
		SupportsTransforms:    true,
		SupportsFolderMapping: true,
	}
}

// CheckHealth calls Cloudinary's non-destructive Admin ping endpoint using HTTP Basic Auth.
func (p *Provider) CheckHealth(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/v1_1/%s/ping", p.cfg.APIBaseURL, url.PathEscape(p.cfg.CloudName))
	reqCtx, cancel := withDefaultTimeout(ctx, DefaultHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}
	req.SetBasicAuth(p.cfg.APIKey, p.cfg.APISecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: cloudinary ping failed: %v", storage.ErrStorageOperation, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: cloudinary authentication failed (status %d)", storage.ErrStorageOperation, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: cloudinary ping returned status %d: %s", storage.ErrStorageOperation, resp.StatusCode, parseCloudinaryError(body))
	}
	return nil
}

func (p *Provider) Save(ctx context.Context, key string, r io.Reader, size int64, mimeType string) error {
	_, err := p.SaveWithOptions(ctx, key, r, size, mimeType, storage.UploadOptions{
		Overwrite: true,
	})
	return err
}

// SaveWithOptions uploads an asset to Cloudinary and returns complete normalized remote metadata.
func (p *Provider) SaveWithOptions(ctx context.Context, key string, r io.Reader, size int64, mimeType string, opts storage.UploadOptions) (*storage.UploadResult, error) {
	cleanKey, err := validateKey(key)
	if err != nil {
		return nil, err
	}

	resType := resolveUploadResourceType(cleanKey, mimeType, opts.ResourceType, p.cfg.DefaultResourceType)
	deliveryType := strings.ToLower(strings.TrimSpace(opts.DeliveryType))
	if deliveryType == "" {
		deliveryType = p.cfg.DeliveryType
	}
	switch deliveryType {
	case "upload", "authenticated", "private":
	default:
		return nil, fmt.Errorf("%w: invalid delivery type %q", storage.ErrInvalidPath, deliveryType)
	}

	publicID, formatHint := p.buildPublicID(cleanKey, resType, opts.Folder)
	assetFolder := path.Dir(publicID)
	if assetFolder == "." {
		assetFolder = ""
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	params := map[string]string{
		"timestamp": timestamp,
		"type":      deliveryType,
	}

	switch p.cfg.AssetNamingStrategy {
	case "random_id":
		if assetFolder != "" {
			params["folder"] = assetFolder
		}
		params["unique_filename"] = "true"
	case "original_filename":
		if assetFolder != "" {
			params["folder"] = assetFolder
		}
		params["use_filename"] = "true"
		params["unique_filename"] = "true"
	default: // cms_key
		params["public_id"] = publicID
		if assetFolder != "" {
			params["asset_folder"] = assetFolder
		}
		if opts.Overwrite {
			params["overwrite"] = "true"
		} else {
			params["overwrite"] = "false"
		}
	}

	uploadPreset := strings.TrimSpace(opts.UploadPreset)
	if uploadPreset == "" {
		uploadPreset = p.cfg.UploadPreset
	}
	if uploadPreset != "" {
		params["upload_preset"] = uploadPreset
	}
	if len(opts.Tags) > 0 {
		params["tags"] = strings.Join(opts.Tags, ",")
	}
	if len(opts.Context) > 0 {
		params["context"] = encodeContextMap(opts.Context)
	}

	signature := SignParams(params, p.cfg.APISecret, false)

	endpoint := fmt.Sprintf("%s/v1_1/%s/%s/upload",
		p.cfg.APIBaseURL,
		url.PathEscape(p.cfg.CloudName),
		url.PathEscape(resType),
	)

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	contentType := mw.FormDataContentType()

	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		writeFields := func() error {
			for k, v := range params {
				if v == "" {
					continue
				}
				if err := mw.WriteField(k, v); err != nil {
					return err
				}
			}
			if err := mw.WriteField("api_key", p.cfg.APIKey); err != nil {
				return err
			}
			if err := mw.WriteField("signature", signature); err != nil {
				return err
			}
			fileName := opts.Filename
			if fileName == "" {
				fileName = path.Base(cleanKey)
			}
			part, err := mw.CreateFormFile("file", fileName)
			if err != nil {
				return err
			}
			if _, err := io.Copy(part, r); err != nil {
				return err
			}
			return mw.Close()
		}
		if err := writeFields(); err != nil {
			_ = pw.CloseWithError(err)
			errCh <- err
			return
		}
		errCh <- nil
	}()

	reqCtx, cancel := withDefaultTimeout(ctx, p.cfg.UploadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, pr)
	if err != nil {
		_ = pr.Close()
		return nil, fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, doErr := p.httpClient.Do(req)
	pipeErr := <-errCh
	if doErr != nil {
		return nil, fmt.Errorf("%w: cloudinary upload failed: %v", storage.ErrStorageOperation, doErr)
	}
	defer resp.Body.Close()

	if pipeErr != nil {
		return nil, fmt.Errorf("%w: failed streaming upload payload: %v", storage.ErrStorageOperation, pipeErr)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: failed reading cloudinary response: %v", storage.ErrStorageOperation, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: cloudinary upload failed (status %d): %s", storage.ErrStorageOperation, resp.StatusCode, parseCloudinaryError(body))
	}

	var parsed struct {
		AssetID      string  `json:"asset_id"`
		PublicID     string  `json:"public_id"`
		Version      int64   `json:"version"`
		Format       string  `json:"format"`
		ResourceType string  `json:"resource_type"`
		Type         string  `json:"type"`
		Width        int     `json:"width"`
		Height       int     `json:"height"`
		Bytes        int64   `json:"bytes"`
		Duration     float64 `json:"duration"`
		URL          string  `json:"url"`
		SecureURL    string  `json:"secure_url"`
		Folder       string  `json:"folder"`
		AssetFolder  string  `json:"asset_folder"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: invalid cloudinary upload JSON response: %v", storage.ErrStorageOperation, err)
	}

	actualResType := parsed.ResourceType
	if actualResType == "" {
		actualResType = resType
		if actualResType == "auto" {
			actualResType = inferResourceType(cleanKey, mimeType)
		}
	}
	actualDelType := parsed.Type
	if actualDelType == "" {
		actualDelType = deliveryType
	}
	actualPublicID := parsed.PublicID
	if actualPublicID == "" {
		actualPublicID = publicID
	}
	actualFormat := parsed.Format
	if actualFormat == "" {
		actualFormat = formatHint
	}
	folderOut := parsed.AssetFolder
	if folderOut == "" {
		folderOut = parsed.Folder
	}
	if folderOut == "" {
		folderOut = assetFolder
	}

	secureURL := parsed.SecureURL
	if secureURL == "" {
		secureURL = p.buildDeliveryURL(storage.AssetRef{
			Key:          cleanKey,
			PublicID:     actualPublicID,
			ResourceType: actualResType,
			DeliveryType: actualDelType,
			Version:      parsed.Version,
			Format:       actualFormat,
		}, "")
	}

	p.cacheAsset(cleanKey, cachedAsset{
		PublicID:     actualPublicID,
		ResourceType: actualResType,
		DeliveryType: actualDelType,
		Version:      parsed.Version,
		Format:       actualFormat,
		SecureURL:    secureURL,
	})

	result := &storage.UploadResult{
		StorageKey:   cleanKey,
		PublicID:     actualPublicID,
		AssetID:      parsed.AssetID,
		ResourceType: actualResType,
		DeliveryType: actualDelType,
		Version:      parsed.Version,
		Format:       actualFormat,
		Width:        parsed.Width,
		Height:       parsed.Height,
		Bytes:        parsed.Bytes,
		Duration:     parsed.Duration,
		SecureURL:    secureURL,
		URL:          p.SelectCanonicalURL(secureURL, parsed.URL),
		Folder:       folderOut,
	}

	if actualResType == "image" && !strings.EqualFold(actualFormat, "svg") {
		ref := storage.AssetRef{
			Key:          cleanKey,
			PublicID:     actualPublicID,
			ResourceType: actualResType,
			DeliveryType: actualDelType,
			Version:      parsed.Version,
			Format:       actualFormat,
		}
		variants := make(map[string]string, 3)
		for name, dims := range map[string]int{"small": 245, "medium": 750, "large": 1000} {
			if u, err := p.TransformURL(ctx, ref, storage.ImageTransformOptions{
				Width:   dims,
				Height:  dims,
				Crop:    "limit",
				Format:  "auto",
				Quality: "auto",
			}); err == nil && u != "" {
				variants[name] = u
			}
		}
		if len(variants) > 0 {
			result.Variants = variants
		}
	}

	return result, nil
}

// SelectCanonicalURL returns secure_url when UseSecureURL is enabled, otherwise falls back cleanly.
func (p *Provider) SelectCanonicalURL(secureURL, plainURL string) string {
	if p.cfg.UseSecureURL {
		if secureURL != "" {
			return secureURL
		}
	}
	if plainURL != "" {
		return plainURL
	}
	return secureURL
}

func (p *Provider) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return p.OpenAsset(ctx, storage.AssetRef{Key: key})
}

func (p *Provider) OpenAsset(ctx context.Context, ref storage.AssetRef) (io.ReadCloser, error) {
	resolved, err := p.resolveRef(ref)
	if err != nil {
		return nil, err
	}

	targetURL := p.buildDeliveryURL(resolved, "")
	if resolved.SecureURL != "" && (p.cfg.DeliveryBaseURL == DefaultDeliveryBaseURL || p.cfg.DeliveryBaseURL == "http://res.cloudinary.com") {
		targetURL = resolved.SecureURL
	}
	if resolved.DeliveryType == "authenticated" || resolved.DeliveryType == "private" {
		signed, signErr := p.SignedURL(ctx, resolved, 300)
		if signErr == nil {
			targetURL = signed
		}
	}

	if err := p.validateDeliveryFetchURL(targetURL); err != nil {
		return nil, fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}

	client := *p.httpClient
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		return p.validateDeliveryFetchURL(req.URL.String())
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed downloading cloudinary asset: %v", storage.ErrStorageOperation, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, storage.ErrFileNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: cloudinary delivery returned status %d", storage.ErrStorageOperation, resp.StatusCode)
	}
	return resp.Body, nil
}

func (p *Provider) Delete(ctx context.Context, key string) error {
	return p.DeleteAsset(ctx, storage.AssetRef{Key: key})
}

func (p *Provider) DeleteAsset(ctx context.Context, ref storage.AssetRef) error {
	resolved, err := p.resolveRef(ref)
	if err != nil {
		return err
	}

	resTypes := candidateResourceTypes(resolved.ResourceType, ref.MIMEType, resolved.Key)
	for idx, resType := range resTypes {
		result, err := p.destroySingle(ctx, resolved.PublicID, resType, resolved.DeliveryType)
		if err != nil {
			return err
		}
		if result == "ok" {
			p.deleteCache(resolved.Key)
			return nil
		}
		if result == "not found" && idx < len(resTypes)-1 {
			continue
		}
		if result == "not found" {
			p.deleteCache(resolved.Key)
			return storage.ErrFileNotFound
		}
	}
	return storage.ErrFileNotFound
}

func (p *Provider) destroySingle(ctx context.Context, publicID, resType, deliveryType string) (string, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	params := map[string]string{
		"invalidate": "true",
		"public_id":  publicID,
		"timestamp":  timestamp,
		"type":       deliveryType,
	}
	sig := SignParams(params, p.cfg.APISecret, false)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	form.Set("api_key", p.cfg.APIKey)
	form.Set("signature", sig)

	endpoint := fmt.Sprintf("%s/v1_1/%s/%s/destroy",
		p.cfg.APIBaseURL,
		url.PathEscape(p.cfg.CloudName),
		url.PathEscape(resType),
	)

	reqCtx, cancel := withDefaultTimeout(ctx, DefaultHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: cloudinary destroy request failed: %v", storage.ErrStorageOperation, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes))
	if resp.StatusCode == http.StatusNotFound {
		return "not found", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: cloudinary destroy failed (status %d): %s", storage.ErrStorageOperation, resp.StatusCode, parseCloudinaryError(body))
	}

	var parsed struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("%w: invalid destroy response: %v", storage.ErrStorageOperation, err)
	}
	if parsed.Result == "" {
		parsed.Result = "ok"
	}
	return parsed.Result, nil
}

func (p *Provider) Exists(ctx context.Context, key string) (bool, error) {
	resolved, err := p.resolveRef(storage.AssetRef{Key: key})
	if err != nil {
		return false, err
	}
	resType := resolved.ResourceType
	if resType == "auto" || resType == "" {
		resType = inferResourceType(resolved.Key, "")
	}

	endpoint := fmt.Sprintf("%s/v1_1/%s/resources/%s/%s/%s",
		p.cfg.APIBaseURL,
		url.PathEscape(p.cfg.CloudName),
		url.PathEscape(resType),
		url.PathEscape(resolved.DeliveryType),
		escapePublicIDSegments(resolved.PublicID),
	)

	reqCtx, cancel := withDefaultTimeout(ctx, DefaultHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}
	req.SetBasicAuth(p.cfg.APIKey, p.cfg.APISecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: cloudinary resource check failed: %v", storage.ErrStorageOperation, err)
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxAPIResponseBytes))
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("%w: cloudinary resource check returned status %d", storage.ErrStorageOperation, resp.StatusCode)
	}
	return true, nil
}

func (p *Provider) URL(_ context.Context, key string) (string, error) {
	return p.AssetURL(context.Background(), storage.AssetRef{Key: key})
}

func (p *Provider) AssetURL(_ context.Context, ref storage.AssetRef) (string, error) {
	resolved, err := p.resolveRef(ref)
	if err != nil {
		return "", err
	}
	if resolved.DeliveryType == "authenticated" || resolved.DeliveryType == "private" {
		return p.SignedURL(context.Background(), resolved, 3600)
	}

	transformSeg := p.defaultOptimizationSegment(resolved)
	if resolved.SecureURL != "" && transformSeg == "" && p.cfg.UseSecureURL && p.cfg.DeliveryBaseURL == DefaultDeliveryBaseURL {
		return resolved.SecureURL, nil
	}
	return p.buildDeliveryURL(resolved, transformSeg), nil
}

func (p *Provider) TransformURL(_ context.Context, ref storage.AssetRef, opts storage.ImageTransformOptions) (string, error) {
	resolved, err := p.resolveRef(ref)
	if err != nil {
		return "", err
	}
	if resolved.ResourceType == "raw" {
		return "", fmt.Errorf("%w: image transformations are not supported for raw assets", storage.ErrInvalidPath)
	}
	seg, err := BuildTransformSegment(opts)
	if err != nil {
		return "", fmt.Errorf("%w: %v", storage.ErrInvalidPath, err)
	}
	if resolved.DeliveryType == "authenticated" || resolved.DeliveryType == "private" {
		return p.buildSignedDeliveryURL(resolved, seg), nil
	}
	return p.buildDeliveryURL(resolved, seg), nil
}

func (p *Provider) SignedURL(_ context.Context, ref storage.AssetRef, _ int64) (string, error) {
	resolved, err := p.resolveRef(ref)
	if err != nil {
		return "", err
	}
	seg := p.defaultOptimizationSegment(resolved)
	return p.buildSignedDeliveryURL(resolved, seg), nil
}

func (p *Provider) Move(ctx context.Context, srcKey, dstKey string) error {
	_, err := p.MoveAsset(ctx, storage.AssetRef{Key: srcKey}, dstKey)
	return err
}

func (p *Provider) MoveAsset(ctx context.Context, src storage.AssetRef, dstKey string) (*storage.UploadResult, error) {
	resolvedSrc, err := p.resolveRef(src)
	if err != nil {
		return nil, err
	}
	cleanDst, err := validateKey(dstKey)
	if err != nil {
		return nil, err
	}

	resType := resolvedSrc.ResourceType
	if resType == "" || resType == "auto" {
		resType = inferResourceType(resolvedSrc.Key, src.MIMEType)
	}

	dstPublicID, dstFormat := p.buildPublicID(cleanDst, resType, "")
	if dstFormat == "" {
		dstFormat = resolvedSrc.Format
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	params := map[string]string{
		"from_public_id": resolvedSrc.PublicID,
		"invalidate":     "true",
		"overwrite":      "true",
		"timestamp":      timestamp,
		"to_public_id":   dstPublicID,
		"type":           resolvedSrc.DeliveryType,
	}
	sig := SignParams(params, p.cfg.APISecret, false)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	form.Set("api_key", p.cfg.APIKey)
	form.Set("signature", sig)

	endpoint := fmt.Sprintf("%s/v1_1/%s/%s/rename",
		p.cfg.APIBaseURL,
		url.PathEscape(p.cfg.CloudName),
		url.PathEscape(resType),
	)

	reqCtx, cancel := withDefaultTimeout(ctx, DefaultHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: cloudinary rename request failed: %v", storage.ErrStorageOperation, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes))
	if resp.StatusCode == http.StatusNotFound {
		return nil, storage.ErrFileNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: cloudinary rename failed (status %d): %s", storage.ErrStorageOperation, resp.StatusCode, parseCloudinaryError(body))
	}

	var parsed struct {
		AssetID      string `json:"asset_id"`
		PublicID     string `json:"public_id"`
		Version      int64  `json:"version"`
		Format       string `json:"format"`
		ResourceType string `json:"resource_type"`
		Type         string `json:"type"`
		SecureURL    string `json:"secure_url"`
		URL          string `json:"url"`
		AssetFolder  string `json:"asset_folder"`
	}
	_ = json.Unmarshal(body, &parsed)

	newPublicID := parsed.PublicID
	if newPublicID == "" {
		newPublicID = dstPublicID
	}
	newVersion := parsed.Version
	if newVersion == 0 {
		newVersion = resolvedSrc.Version
	}
	newFormat := parsed.Format
	if newFormat == "" {
		newFormat = dstFormat
	}
	newSecureURL := parsed.SecureURL
	if newSecureURL == "" {
		newSecureURL = p.buildDeliveryURL(storage.AssetRef{
			Key:          cleanDst,
			PublicID:     newPublicID,
			ResourceType: resType,
			DeliveryType: resolvedSrc.DeliveryType,
			Version:      newVersion,
			Format:       newFormat,
		}, "")
	}

	p.deleteCache(resolvedSrc.Key)
	p.cacheAsset(cleanDst, cachedAsset{
		PublicID:     newPublicID,
		ResourceType: resType,
		DeliveryType: resolvedSrc.DeliveryType,
		Version:      newVersion,
		Format:       newFormat,
		SecureURL:    newSecureURL,
	})

	folderOut := parsed.AssetFolder
	if folderOut == "" {
		if dir := path.Dir(newPublicID); dir != "." {
			folderOut = dir
		}
	}

	return &storage.UploadResult{
		StorageKey:   cleanDst,
		PublicID:     newPublicID,
		AssetID:      parsed.AssetID,
		ResourceType: resType,
		DeliveryType: resolvedSrc.DeliveryType,
		Version:      newVersion,
		Format:       newFormat,
		SecureURL:    newSecureURL,
		URL:          p.SelectCanonicalURL(newSecureURL, parsed.URL),
		Folder:       folderOut,
	}, nil
}

func (p *Provider) DeletePrefix(ctx context.Context, prefix string) error {
	cleanPrefix, err := validateKey(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return err
	}
	fullPrefix := cleanPrefix
	if p.cfg.RootFolder != "" && !strings.HasPrefix(cleanPrefix, p.cfg.RootFolder+"/") && cleanPrefix != p.cfg.RootFolder {
		fullPrefix = p.cfg.RootFolder + "/" + cleanPrefix
	}
	if !strings.HasSuffix(fullPrefix, "/") {
		fullPrefix += "/"
	}

	for _, resType := range []string{"image", "video", "raw"} {
		endpoint := fmt.Sprintf("%s/v1_1/%s/resources/%s/%s?prefix=%s",
			p.cfg.APIBaseURL,
			url.PathEscape(p.cfg.CloudName),
			url.PathEscape(resType),
			url.PathEscape(p.cfg.DeliveryType),
			url.QueryEscape(fullPrefix),
		)

		reqCtx, cancel := withDefaultTimeout(ctx, DefaultHTTPTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodDelete, endpoint, nil)
		if err != nil {
			cancel()
			return fmt.Errorf("%w: %v", storage.ErrStorageOperation, err)
		}
		req.SetBasicAuth(p.cfg.APIKey, p.cfg.APISecret)

		resp, err := p.httpClient.Do(req)
		cancel()
		if err != nil {
			return fmt.Errorf("%w: cloudinary prefix delete failed: %v", storage.ErrStorageOperation, err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes))
		resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("%w: cloudinary prefix delete failed (status %d): %s", storage.ErrStorageOperation, resp.StatusCode, parseCloudinaryError(body))
		}
	}
	return nil
}

func (p *Provider) resolveRef(ref storage.AssetRef) (storage.AssetRef, error) {
	cleanKey, err := validateKey(ref.Key)
	if err != nil {
		return storage.AssetRef{}, err
	}

	cached, hasCached := p.getCached(cleanKey)
	resType := ref.ResourceType
	if resType == "" && hasCached {
		resType = cached.ResourceType
	}
	if resType == "" || resType == "auto" {
		resType = inferResourceType(cleanKey, ref.MIMEType)
	}

	delType := ref.DeliveryType
	if delType == "" && hasCached {
		delType = cached.DeliveryType
	}
	if delType == "" {
		delType = p.cfg.DeliveryType
	}

	publicID := ref.PublicID
	format := ref.Format
	if publicID == "" && hasCached {
		publicID = cached.PublicID
		if format == "" {
			format = cached.Format
		}
	}
	if publicID == "" {
		var inferredFmt string
		publicID, inferredFmt = p.buildPublicID(cleanKey, resType, "")
		if format == "" {
			format = inferredFmt
		}
	}

	version := ref.Version
	if version == 0 && hasCached {
		version = cached.Version
	}

	secureURL := ref.SecureURL
	if secureURL == "" && hasCached {
		secureURL = cached.SecureURL
	}

	return storage.AssetRef{
		Key:          cleanKey,
		MIMEType:     ref.MIMEType,
		PublicID:     publicID,
		ResourceType: resType,
		DeliveryType: delType,
		Version:      version,
		Format:       format,
		SecureURL:    secureURL,
	}, nil
}

func (p *Provider) buildPublicID(cleanKey, resType, extraFolder string) (publicID string, format string) {
	k := strings.TrimPrefix(cleanKey, "/")
	ext := path.Ext(k)
	if ext != "" {
		format = strings.TrimPrefix(strings.ToLower(ext), ".")
	}

	folderPrefix := p.cfg.RootFolder
	if extra := NormalizeFolderPath(extraFolder); extra != "" {
		if folderPrefix != "" && !strings.HasPrefix(extra, folderPrefix+"/") && extra != folderPrefix {
			folderPrefix = folderPrefix + "/" + extra
		} else {
			folderPrefix = extra
		}
	}

	idPath := k
	if resType != "raw" && ext != "" {
		idPath = strings.TrimSuffix(k, ext)
	}

	if folderPrefix != "" && !strings.HasPrefix(idPath, folderPrefix+"/") && idPath != folderPrefix {
		idPath = folderPrefix + "/" + idPath
	}
	return idPath, format
}

func (p *Provider) buildDeliveryURL(ref storage.AssetRef, transformSeg string) string {
	base := p.cfg.DeliveryBaseURL
	resType := ref.ResourceType
	if resType == "" || resType == "auto" {
		resType = "image"
	}
	delType := ref.DeliveryType
	if delType == "" {
		delType = p.cfg.DeliveryType
	}

	parts := []string{
		strings.TrimRight(base, "/"),
		url.PathEscape(p.cfg.CloudName),
		url.PathEscape(resType),
		url.PathEscape(delType),
	}
	if transformSeg != "" {
		parts = append(parts, transformSeg)
	}
	if ref.Version > 0 {
		parts = append(parts, fmt.Sprintf("v%d", ref.Version))
	}

	publicSeg := escapePublicIDSegments(ref.PublicID)
	if resType != "raw" && ref.Format != "" && !strings.HasSuffix(strings.ToLower(publicSeg), "."+strings.ToLower(ref.Format)) {
		publicSeg += "." + ref.Format
	}
	parts = append(parts, publicSeg)
	return strings.Join(parts, "/")
}

func (p *Provider) buildSignedDeliveryURL(ref storage.AssetRef, transformSeg string) string {
	resType := ref.ResourceType
	if resType == "" || resType == "auto" {
		resType = "image"
	}
	delType := ref.DeliveryType
	if delType == "" {
		delType = p.cfg.DeliveryType
	}

	publicWithExt := ref.PublicID
	if resType != "raw" && ref.Format != "" && !strings.HasSuffix(strings.ToLower(publicWithExt), "."+strings.ToLower(ref.Format)) {
		publicWithExt += "." + ref.Format
	}

	toSign := publicWithExt
	if transformSeg != "" {
		toSign = transformSeg + "/" + publicWithExt
	}
	sigSeg := SignDeliveryPath(toSign, p.cfg.APISecret)

	parts := []string{
		strings.TrimRight(p.cfg.DeliveryBaseURL, "/"),
		url.PathEscape(p.cfg.CloudName),
		url.PathEscape(resType),
		url.PathEscape(delType),
		sigSeg,
	}
	if transformSeg != "" {
		parts = append(parts, transformSeg)
	}
	if ref.Version > 0 {
		parts = append(parts, fmt.Sprintf("v%d", ref.Version))
	}

	publicEscaped := escapePublicIDSegments(ref.PublicID)
	if resType != "raw" && ref.Format != "" && !strings.HasSuffix(strings.ToLower(publicEscaped), "."+strings.ToLower(ref.Format)) {
		publicEscaped += "." + ref.Format
	}
	parts = append(parts, publicEscaped)
	return strings.Join(parts, "/")
}

func (p *Provider) defaultOptimizationSegment(ref storage.AssetRef) string {
	if !p.cfg.OptimizationEnabled || ref.ResourceType != "image" || strings.EqualFold(ref.Format, "svg") {
		return ""
	}
	opts := storage.ImageTransformOptions{
		Format:  p.cfg.DefaultFormat,
		Quality: p.cfg.DefaultQuality,
		Width:   p.cfg.MaxWidth,
	}
	if opts.Width > 0 {
		opts.Crop = "limit"
	}
	seg, err := BuildTransformSegment(opts)
	if err != nil {
		return ""
	}
	return seg
}

func (p *Provider) validateDeliveryFetchURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid delivery URL")
	}
	if !p.cfg.AllowInsecureHTTP && p.cfg.UseSecureURL && u.Scheme != "https" {
		return fmt.Errorf("delivery URL must use HTTPS")
	}
	expectedBase, _ := url.Parse(p.cfg.DeliveryBaseURL)
	host := strings.ToLower(u.Hostname())
	expectedHost := strings.ToLower(expectedBase.Hostname())
	if host != expectedHost && !strings.HasSuffix(host, ".cloudinary.com") {
		return fmt.Errorf("disallowed delivery host %q", host)
	}
	return nil
}

func (p *Provider) cacheAsset(key string, meta cachedAsset) {
	p.mu.Lock()
	p.cache[key] = meta
	p.mu.Unlock()
}

func (p *Provider) getCached(key string) (cachedAsset, bool) {
	p.mu.RLock()
	v, ok := p.cache[key]
	p.mu.RUnlock()
	return v, ok
}

func (p *Provider) deleteCache(key string) {
	p.mu.Lock()
	delete(p.cache, key)
	p.mu.Unlock()
}

func validateKey(key string) (string, error) {
	if key == "" {
		return "", storage.ErrInvalidPath
	}
	if strings.ContainsRune(key, '\x00') || strings.Contains(key, "\\") {
		return "", storage.ErrPathTraversal
	}
	trimmed := strings.TrimPrefix(key, "/")
	if trimmed == "" {
		return "", storage.ErrInvalidPath
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == ".." || seg == "." {
			return "", storage.ErrPathTraversal
		}
	}
	return trimmed, nil
}

func resolveUploadResourceType(key, mimeType, explicitHint, defaultResType string) string {
	switch strings.ToLower(strings.TrimSpace(explicitHint)) {
	case "image", "video", "raw":
		return strings.ToLower(strings.TrimSpace(explicitHint))
	}
	if defaultResType == "image" || defaultResType == "video" || defaultResType == "raw" {
		return defaultResType
	}
	if mimeType != "" || key != "" {
		if defaultResType == "auto" {
			return "auto"
		}
	}
	return inferResourceType(key, mimeType)
}

func inferResourceType(key, mimeType string) string {
	mt := strings.ToLower(strings.TrimSpace(mimeType))
	if mt != "" {
		if strings.HasPrefix(mt, "image/") {
			return "image"
		}
		if strings.HasPrefix(mt, "video/") || strings.HasPrefix(mt, "audio/") {
			return "video"
		}
		return "raw"
	}

	ext := strings.ToLower(path.Ext(key))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".svg", ".bmp", ".ico", ".tif", ".tiff":
		return "image"
	case ".mp4", ".mov", ".webm", ".mkv", ".avi", ".m4v", ".mp3", ".wav", ".ogg", ".m4a":
		return "video"
	case ".pdf", ".zip", ".tar", ".gz", ".json", ".csv", ".txt", ".md", ".xml", ".doc", ".docx", ".xls", ".xlsx":
		return "raw"
	default:
		return "image"
	}
}

func candidateResourceTypes(primary, mimeType, key string) []string {
	seen := make(map[string]struct{}, 3)
	out := make([]string, 0, 3)
	add := func(rt string) {
		if rt != "image" && rt != "video" && rt != "raw" {
			return
		}
		if _, ok := seen[rt]; !ok {
			seen[rt] = struct{}{}
			out = append(out, rt)
		}
	}
	add(primary)
	add(inferResourceType(key, mimeType))
	add("image")
	add("raw")
	add("video")
	return out
}

func escapePublicIDSegments(publicID string) string {
	parts := strings.Split(strings.TrimPrefix(publicID, "/"), "/")
	for i, seg := range parts {
		parts[i] = url.PathEscape(seg)
	}
	return strings.Join(parts, "/")
}

func encodeContextMap(ctxMap map[string]string) string {
	if len(ctxMap) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(ctxMap))
	for k, v := range ctxMap {
		cleanK := strings.ReplaceAll(strings.TrimSpace(k), "|", "_")
		cleanV := strings.ReplaceAll(strings.TrimSpace(v), "|", " ")
		cleanK = strings.ReplaceAll(cleanK, "=", "_")
		cleanV = strings.ReplaceAll(cleanV, "=", " ")
		if cleanK != "" && cleanV != "" {
			pairs = append(pairs, cleanK+"="+cleanV)
		}
	}
	return strings.Join(pairs, "|")
}

func parseCloudinaryError(body []byte) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		return parsed.Error.Message
	}
	msg := strings.TrimSpace(string(body))
	if len(msg) > 200 {
		msg = msg[:200]
	}
	if msg == "" {
		return "unknown error"
	}
	return msg
}

func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

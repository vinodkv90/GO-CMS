package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

const (
	maxResponseSnippetBytes = 2048
	deliveryTimeout         = 10 * time.Second
)

// ErrUnsafeWebhookURL indicates a webhook target failed SSRF safety checks.
var ErrUnsafeWebhookURL = errors.New("unsafe webhook target URL")

var reservedHeaders = map[string]bool{
	"host":                    true,
	"content-length":          true,
	"content-type":            true,
	"x-cms-webhook-event":     true,
	"x-cms-webhook-delivery":  true,
	"x-cms-webhook-signature": true,
}

func isRestrictedAddr(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	// Block CGNAT 100.64.0.0/10 and benchmark/metadata space
	cgnatPrefix := netip.MustParsePrefix("100.64.0.0/10")
	return cgnatPrefix.Contains(addr)
}

// ValidateWebhookURL verifies a webhook destination uses HTTP(S) and is not a private/loopback/internal endpoint.
func ValidateWebhookURL(rawURL string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: must be a valid absolute http(s) URL", ErrUnsafeWebhookURL)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return fmt.Errorf("%w: unsupported scheme %q", ErrUnsafeWebhookURL, parsed.Scheme)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("%w: internal host %q is not allowed", ErrUnsafeWebhookURL, host)
	}
	if ip := net.ParseIP(host); ip != nil && isRestrictedAddr(ip) {
		return fmt.Errorf("%w: private or loopback IP %q is not allowed", ErrUnsafeWebhookURL, host)
	}
	return nil
}

// ComputeHMACSignature returns the canonical X-CMS-Webhook-Signature header value ("sha256=<hex>").
func ComputeHMACSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newSSRFSafeHTTPClient(allowUnsafe bool) *http.Client {
	dialer := &net.Dialer{
		Timeout:   deliveryTimeout,
		KeepAlive: 30 * time.Second,
	}
	if !allowUnsafe {
		dialer.Control = func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			ip := net.ParseIP(host)
			if isRestrictedAddr(ip) {
				return fmt.Errorf("%w: refused connection to restricted IP %s", ErrUnsafeWebhookURL, host)
			}
			return nil
		}
	}
	return &http.Client{
		Timeout: deliveryTimeout,
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   deliveryTimeout,
			ResponseHeaderTimeout: deliveryTimeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if !allowUnsafe {
				return ValidateWebhookURL(req.URL.String())
			}
			return nil
		},
	}
}

func executeDeliveryAttempt(
	ctx context.Context,
	client *http.Client,
	wh *Webhook,
	deliveryID string,
	eventName string,
	payload []byte,
	signingSecret string,
	attempt int,
) (int, int64, bool, string, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(payload))
	if err != nil {
		return 0, 0, false, err.Error(), ""
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "Go-CMS-Webhook/1.0")
	req.Header.Set("X-CMS-Webhook-Event", eventName)
	req.Header.Set("X-CMS-Webhook-Delivery", deliveryID)
	if signingSecret != "" {
		req.Header.Set("X-CMS-Webhook-Signature", ComputeHMACSignature(signingSecret, payload))
	}
	for k, v := range wh.Headers {
		cleanKey := strings.TrimSpace(k)
		if cleanKey == "" || reservedHeaders[strings.ToLower(cleanKey)] {
			continue
		}
		req.Header.Set(cleanKey, v)
	}

	start := time.Now()
	resp, err := client.Do(req)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return 0, durationMs, false, err.Error(), ""
	}
	defer resp.Body.Close()

	snippetBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSnippetBytes))
	snippet := strings.TrimSpace(string(snippetBytes))
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	var errMsg string
	if !success {
		errMsg = fmt.Sprintf("endpoint returned HTTP %d (attempt %d)", resp.StatusCode, attempt)
	}
	return resp.StatusCode, durationMs, success, errMsg, snippet
}


package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrSignatureExpired = errors.New("signature expired")
	ErrMissingSignature = errors.New("missing signature parameters")
)

// URLSigner signs and verifies URLs using HMAC-SHA256.
type URLSigner struct {
	secretKey  []byte
	defaultTTL time.Duration
}

// NewURLSigner creates a URLSigner instance with the given secret key and default expiration duration.
func NewURLSigner(secretKey string, defaultTTL time.Duration) *URLSigner {
	if defaultTTL <= 0 {
		defaultTTL = 1 * time.Hour
	}
	return &URLSigner{
		secretKey:  []byte(secretKey),
		defaultTTL: defaultTTL,
	}
}

// computeMAC computes the hex-encoded HMAC-SHA256 over message.
func (s *URLSigner) computeMAC(message string) string {
	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// Sign returns a signed URL containing ?expires=...&signature=... query parameters.
func (s *URLSigner) Sign(rawURL string, ttl time.Duration) (string, error) {
	return s.SignForTenant(rawURL, ttl, "")
}

// SignForTenant signs a URL for one tenant. The tenant identifier is part of
// the MAC input so a valid URL cannot be replayed across tenant contexts.
func (s *URLSigner) SignForTenant(rawURL string, ttl time.Duration, tenantID string) (string, error) {
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	return s.SignWithTenantExpiration(rawURL, time.Now().Add(ttl), tenantID)
}

// SignWithExpiration creates a signed URL with an explicit expiration time.
func (s *URLSigner) SignWithExpiration(rawURL string, expiresAt time.Time) (string, error) {
	return s.SignWithTenantExpiration(rawURL, expiresAt, "")
}

// SignWithTenantExpiration creates a signed URL with an explicit tenant scope.
func (s *URLSigner) SignWithTenantExpiration(rawURL string, expiresAt time.Time, tenantID string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse url: %w", err)
	}

	expiresStr := strconv.FormatInt(expiresAt.Unix(), 10)

	// Clean path to ensure canonical message formatting
	path := u.Path
	if path == "" {
		path = "/"
	}

	message := fmt.Sprintf("%s:%s", path, expiresStr)
	if tenantID != "" {
		message = fmt.Sprintf("%s:%s:%s", path, expiresStr, tenantID)
	}
	signature := s.computeMAC(message)

	q := u.Query()
	q.Set("expires", expiresStr)
	q.Set("signature", signature)
	if tenantID != "" {
		q.Set("tenant", tenantID)
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// VerifyURL verifies that the provided URL contains a valid, non-expired HMAC-SHA256 signature.
func (s *URLSigner) VerifyURL(u *url.URL) error {
	return s.VerifyURLForTenant(u, "")
}

// VerifyURLForTenant verifies expiry, signature, and tenant scope.
func (s *URLSigner) VerifyURLForTenant(u *url.URL, tenantID string) error {
	q := u.Query()
	expiresStr := q.Get("expires")
	sigStr := q.Get("signature")

	if expiresStr == "" || sigStr == "" {
		return ErrMissingSignature
	}

	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return ErrInvalidSignature
	}

	if time.Now().Unix() > expires {
		return ErrSignatureExpired
	}

	if q.Get("tenant") != tenantID {
		return ErrInvalidSignature
	}

	path := u.Path
	if path == "" {
		path = "/"
	}

	message := fmt.Sprintf("%s:%s", path, expiresStr)
	if tenantID != "" {
		message = fmt.Sprintf("%s:%s:%s", path, expiresStr, tenantID)
	}
	expectedMAC := s.computeMAC(message)

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(sigStr)), []byte(strings.ToLower(expectedMAC))) != 1 {
		return ErrInvalidSignature
	}

	return nil
}

// VerifyPath verifies signature parameters against a specific path.
func (s *URLSigner) VerifyPath(path string, expires int64, signature string) error {
	if time.Now().Unix() > expires {
		return ErrSignatureExpired
	}

	message := fmt.Sprintf("%s:%d", path, expires)
	expectedMAC := s.computeMAC(message)

	if subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(strings.ToLower(expectedMAC))) != 1 {
		return ErrInvalidSignature
	}

	return nil
}

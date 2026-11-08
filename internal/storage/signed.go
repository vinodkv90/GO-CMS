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
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	return s.SignWithExpiration(rawURL, time.Now().Add(ttl))
}

// SignWithExpiration creates a signed URL with an explicit expiration time.
func (s *URLSigner) SignWithExpiration(rawURL string, expiresAt time.Time) (string, error) {
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
	signature := s.computeMAC(message)

	q := u.Query()
	q.Set("expires", expiresStr)
	q.Set("signature", signature)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// VerifyURL verifies that the provided URL contains a valid, non-expired HMAC-SHA256 signature.
func (s *URLSigner) VerifyURL(u *url.URL) error {
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

	path := u.Path
	if path == "" {
		path = "/"
	}

	message := fmt.Sprintf("%s:%s", path, expiresStr)
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

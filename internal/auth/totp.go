package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const (
	totpPeriodSeconds = 30
	totpDigits        = 6
	totpSecretBytes   = 20
	mfaEncPrefix      = "enc:v1:"
	recoveryAlphabet  = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

var b32NoPadding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret generates a random RFC 6238 base32-encoded TOTP secret.
func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, totpSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate TOTP entropy: %w", err)
	}
	return b32NoPadding.EncodeToString(buf), nil
}

// BuildOTPAuthURI builds a standard otpauth://totp/ URI compatible with authenticator apps.
func BuildOTPAuthURI(issuer, accountName, secret string) string {
	cleanIssuer := strings.TrimSpace(issuer)
	if cleanIssuer == "" {
		cleanIssuer = "Go CMS"
	}
	label := url.PathEscape(cleanIssuer) + ":" + url.PathEscape(strings.TrimSpace(accountName))
	q := url.Values{}
	q.Set("secret", strings.ToUpper(strings.TrimSpace(secret)))
	q.Set("issuer", cleanIssuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// ComputeTOTPCode calculates the 6-digit RFC 6238 HMAC-SHA1 TOTP code for a timestamp.
func ComputeTOTPCode(secret string, at time.Time) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(secret, " ", "")))
	if normalized == "" {
		return "", errors.New("empty TOTP secret")
	}
	// Accept both padded and unpadded base32 inputs cleanly
	normalized = strings.TrimRight(normalized, "=")
	key, err := b32NoPadding.DecodeString(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid base32 TOTP secret: %w", err)
	}

	counter := uint64(at.Unix() / totpPeriodSeconds)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)

	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	binCode := (uint32(sum[offset])&0x7f)<<24 |
		(uint32(sum[offset+1])&0xff)<<16 |
		(uint32(sum[offset+2])&0xff)<<8 |
		(uint32(sum[offset+3]) & 0xff)
	otp := binCode % 1000000
	return fmt.Sprintf("%06d", otp), nil
}

// VerifyTOTPCode validates a 6-digit TOTP code across a +-1 step window using constant-time comparison.
func VerifyTOTPCode(secret, code string, now time.Time) bool {
	trimmed := strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if len(trimmed) != totpDigits {
		return false
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return false
		}
	}
	for _, stepOffset := range []int{-1, 0, 1} {
		candidate, err := ComputeTOTPCode(secret, now.Add(time.Duration(stepOffset*totpPeriodSeconds)*time.Second))
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(trimmed)) == 1 {
			return true
		}
	}
	return false
}

func normalizeRecoveryCode(code string) string {
	cleaned := strings.ToUpper(strings.TrimSpace(code))
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	return cleaned
}

func hashRecoveryCode(normalized string) string {
	sum := sha256.Sum256([]byte("recovery:" + normalized))
	return hex.EncodeToString(sum[:])
}

// GenerateRecoveryCodes generates single-use backup recovery codes and their hashed JSON representation.
func GenerateRecoveryCodes(count int) ([]string, string, error) {
	if count <= 0 {
		count = 8
	}
	plain := make([]string, 0, count)
	hashes := make([]string, 0, count)
	buf := make([]byte, 8)
	for i := 0; i < count; i++ {
		if _, err := rand.Read(buf); err != nil {
			return nil, "", fmt.Errorf("failed to generate recovery code: %w", err)
		}
		raw := make([]byte, 8)
		for j := 0; j < 8; j++ {
			raw[j] = recoveryAlphabet[int(buf[j])%len(recoveryAlphabet)]
		}
		formatted := string(raw[:4]) + "-" + string(raw[4:])
		plain = append(plain, formatted)
		hashes = append(hashes, hashRecoveryCode(normalizeRecoveryCode(formatted)))
	}
	encoded, err := json.Marshal(hashes)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal recovery code hashes: %w", err)
	}
	return plain, string(encoded), nil
}

// VerifyAndConsumeRecoveryCode validates a single-use recovery code and returns remaining hashes.
func VerifyAndConsumeRecoveryCode(hashedJSON, code string) (bool, string, error) {
	if strings.TrimSpace(hashedJSON) == "" {
		return false, hashedJSON, nil
	}
	var hashes []string
	if err := json.Unmarshal([]byte(hashedJSON), &hashes); err != nil {
		return false, hashedJSON, fmt.Errorf("invalid stored recovery hashes: %w", err)
	}
	normalized := normalizeRecoveryCode(code)
	if len(normalized) < 6 {
		return false, hashedJSON, nil
	}
	targetHash := hashRecoveryCode(normalized)

	matchIdx := -1
	for i, h := range hashes {
		if len(h) == len(targetHash) && subtle.ConstantTimeCompare([]byte(h), []byte(targetHash)) == 1 && matchIdx == -1 {
			matchIdx = i
		}
	}
	if matchIdx == -1 {
		return false, hashedJSON, nil
	}
	remaining := append([]string{}, hashes[:matchIdx]...)
	remaining = append(remaining, hashes[matchIdx+1:]...)
	updated, err := json.Marshal(remaining)
	if err != nil {
		return false, hashedJSON, fmt.Errorf("failed to update recovery hashes: %w", err)
	}
	return true, string(updated), nil
}

func deriveMFAAESKey(keyMaterial string) []byte {
	sum := sha256.Sum256([]byte("mfa-totp:" + keyMaterial))
	return sum[:]
}

// EncryptMFASecret encrypts a TOTP secret using AES-256-GCM.
func EncryptMFASecret(keyMaterial, plaintext string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", nil
	}
	block, err := aes.NewCipher(deriveMFAAESKey(keyMaterial))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return mfaEncPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptMFASecret decrypts an AES-256-GCM encrypted TOTP secret.
func DecryptMFASecret(keyMaterial, encoded string) (string, error) {
	if strings.TrimSpace(encoded) == "" {
		return "", nil
	}
	if !strings.HasPrefix(encoded, mfaEncPrefix) {
		return "", errors.New("unsupported MFA secret envelope")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, mfaEncPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(deriveMFAAESKey(keyMaterial))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("corrupted MFA secret ciphertext")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}


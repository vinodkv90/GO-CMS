package settings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// SecretPreserveSentinel is returned by read endpoints and accepted on updates
	// to keep a stored encrypted secret unchanged without ever exposing plaintext.
	SecretPreserveSentinel = "__CMS_SECRET_PRESERVED__"

	encryptedPrefix = "enc:v1:"
)

func deriveKey(secretKey string) []byte {
	sum := sha256.Sum256([]byte(secretKey))
	return sum[:]
}

// EncryptSecret encrypts a non-empty plaintext string with AES-256-GCM.
func EncryptSecret(secretKey, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if secretKey == "" {
		return "", errors.New("encryption secret key is empty")
	}
	block, err := aes.NewCipher(deriveKey(secretKey))
	if err != nil {
		return "", fmt.Errorf("failed to initialize cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to initialize GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encryptedPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptSecret decrypts an AES-256-GCM string created by EncryptSecret.
func DecryptSecret(secretKey, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, encryptedPrefix) {
		return "", errors.New("unsupported or unencrypted secret format")
	}
	if secretKey == "" {
		return "", errors.New("encryption secret key is empty")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, encryptedPrefix))
	if err != nil {
		return "", fmt.Errorf("invalid base64 secret payload: %w", err)
	}
	block, err := aes.NewCipher(deriveKey(secretKey))
	if err != nil {
		return "", fmt.Errorf("failed to initialize cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to initialize GCM: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("malformed encrypted secret")
	}
	nonce, payload := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt secret: %w", err)
	}
	return string(plaintext), nil
}

// ResolveUpdatedSecret computes the next plaintext secret when processing an update payload.
// Passing SecretPreserveSentinel preserves existingPlaintext without modifying it.
func ResolveUpdatedSecret(incoming, existingPlaintext string) string {
	if incoming == SecretPreserveSentinel {
		return existingPlaintext
	}
	return strings.TrimSpace(incoming)
}


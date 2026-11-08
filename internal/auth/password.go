package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidHash         = errors.New("the encoded password hash is not in the correct format")
	ErrIncompatibleVersion = errors.New("incompatible version of argon2")
	ErrWeakPassword        = errors.New("password does not meet security requirements")
)

// Argon2Params holds the configuration parameters for Argon2id hashing.
type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultArgon2Params returns OWASP-recommended Argon2id parameters.
func DefaultArgon2Params() *Argon2Params {
	return &Argon2Params{
		Memory:      64 * 1024, // 64 MB
		Iterations:  3,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// HashPassword hashes a plaintext password using Argon2id.
func HashPassword(password string, params *Argon2Params) (string, error) {
	if params == nil {
		params = DefaultArgon2Params()
	}

	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate random salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encodedHash := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.Memory,
		params.Iterations,
		params.Parallelism,
		b64Salt,
		b64Hash,
	)

	return encodedHash, nil
}

// ComparePassword compares a plaintext password with an encoded Argon2id hash.
func ComparePassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, ErrInvalidHash
	}

	if parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrInvalidHash
	}
	if version != argon2.Version {
		return false, ErrIncompatibleVersion
	}

	params := &Argon2Params{}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.Memory, &params.Iterations, &params.Parallelism); err != nil {
		return false, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidHash
	}
	params.SaltLength = uint32(len(salt))

	expectedHash, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidHash
	}
	params.KeyLength = uint32(len(expectedHash))

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	if subtle.ConstantTimeCompare(expectedHash, computedHash) == 1 {
		return true, nil
	}

	return false, nil
}

// ValidatePasswordStrength enforces minimum password complexity rules.
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("%w: must be at least 8 characters long", ErrWeakPassword)
	}

	var hasUpper, hasLower, hasDigit bool
	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit {
		return fmt.Errorf("%w: must include uppercase, lowercase, and digit characters", ErrWeakPassword)
	}

	return nil
}


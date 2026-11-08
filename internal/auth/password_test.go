package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndComparePassword(t *testing.T) {
	// Fast test parameters for unit tests
	testParams := &Argon2Params{
		Memory:      1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}

	password := "SecretP@ssw0rd!"

	encodedHash, err := HashPassword(password, testParams)
	require.NoError(t, err)
	assert.Contains(t, encodedHash, "$argon2id$")

	// Correct password
	match, err := ComparePassword(password, encodedHash)
	require.NoError(t, err)
	assert.True(t, match)

	// Incorrect password
	match, err = ComparePassword("WrongPassword123", encodedHash)
	require.NoError(t, err)
	assert.False(t, match)

	// Corrupted hash
	_, err = ComparePassword(password, "invalid-hash-string")
	assert.ErrorIs(t, err, ErrInvalidHash)
}

func TestValidatePasswordStrength(t *testing.T) {
	assert.NoError(t, ValidatePasswordStrength("SecureP@ss1"))

	// Too short
	assert.Error(t, ValidatePasswordStrength("P@1a"))

	// Missing uppercase
	assert.Error(t, ValidatePasswordStrength("password123!"))

	// Missing lowercase
	assert.Error(t, ValidatePasswordStrength("PASSWORD123!"))

	// Missing digit
	assert.Error(t, ValidatePasswordStrength("PasswordOnly!"))
}


package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTOTP_RFC6238KnownVectorAndVerification(t *testing.T) {
	// RFC 6238 SHA-1 test seed "12345678901234567890" base32 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	rfcSecret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	// Timestamp 59s -> RFC 6238 8-digit 94287082, 6-digit = 287082
	ts59 := time.Unix(59, 0).UTC()
	code, err := ComputeTOTPCode(rfcSecret, ts59)
	require.NoError(t, err)
	assert.Equal(t, "287082", code)

	// Window verification (-1, 0, +1 step)
	assert.True(t, VerifyTOTPCode(rfcSecret, code, ts59))
	assert.True(t, VerifyTOTPCode(rfcSecret, code, ts59.Add(30*time.Second)))
	assert.False(t, VerifyTOTPCode(rfcSecret, code, ts59.Add(90*time.Second)))
	assert.False(t, VerifyTOTPCode(rfcSecret, "000000", ts59.Add(120*time.Second)))
	assert.False(t, VerifyTOTPCode(rfcSecret, "12345", ts59))
	assert.False(t, VerifyTOTPCode(rfcSecret, "abcdef", ts59))
}

func TestTOTP_SecretGenerationURIEncryptionAndRecoveryCodes(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(secret), 32)

	uri := BuildOTPAuthURI("Go CMS", "admin@example.com", secret)
	assert.Contains(t, uri, "otpauth://totp/")
	assert.Contains(t, uri, "secret="+secret)
	assert.Contains(t, uri, "issuer=Go+CMS")

	enc, err := EncryptMFASecret("session-secret-key", secret)
	require.NoError(t, err)
	assert.NotContains(t, enc, secret)
	assert.Contains(t, enc, mfaEncPrefix)

	dec, err := DecryptMFASecret("session-secret-key", enc)
	require.NoError(t, err)
	assert.Equal(t, secret, dec)

	_, err = DecryptMFASecret("wrong-secret-key", enc)
	assert.Error(t, err)

	// Single-use backup recovery codes
	plainCodes, hashesJSON, err := GenerateRecoveryCodes(8)
	require.NoError(t, err)
	require.Len(t, plainCodes, 8)
	assert.NotContains(t, hashesJSON, plainCodes[0])

	ok, nextHashes, err := VerifyAndConsumeRecoveryCode(hashesJSON, plainCodes[0])
	require.NoError(t, err)
	assert.True(t, ok)

	// Re-using the same code must fail!
	okAgain, _, err := VerifyAndConsumeRecoveryCode(nextHashes, plainCodes[0])
	require.NoError(t, err)
	assert.False(t, okAgain)

	// Unused code without dash & lowercase still validates
	okOther, _, err := VerifyAndConsumeRecoveryCode(nextHashes, " "+plainCodes[1]+" ")
	require.NoError(t, err)
	assert.True(t, okOther)
}

func TestValidatePasswordWithPolicy(t *testing.T) {
	policy := PasswordPolicy{
		MinLength:        12,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireNumber:    true,
		RequireSpecial:   true,
	}

	assert.ErrorIs(t, ValidatePasswordWithPolicy("Short@1", policy), ErrWeakPassword)
	assert.ErrorIs(t, ValidatePasswordWithPolicy("lowercaseonly1!", policy), ErrWeakPassword)
	assert.ErrorIs(t, ValidatePasswordWithPolicy("UPPERCASEONLY1!", policy), ErrWeakPassword)
	assert.ErrorIs(t, ValidatePasswordWithPolicy("NoDigitsHere!!", policy), ErrWeakPassword)
	assert.ErrorIs(t, ValidatePasswordWithPolicy("NoSpecialChars12", policy), ErrWeakPassword)
	assert.NoError(t, ValidatePasswordWithPolicy("StrongPass!2026", policy))
}


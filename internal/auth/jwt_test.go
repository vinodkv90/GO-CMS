package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidateAccessToken(t *testing.T) {
	secret := "test-super-secret-jwt-signing-key-123456"
	user := &User{
		ID:       "usr_12345",
		Username: "admin",
		Email:    "admin@example.com",
		Roles: []Role{
			{Slug: "super_admin"},
		},
	}

	// 1. Valid Token
	tokenStr, err := GenerateAccessToken(user, secret, 1*time.Hour)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	claims, err := ValidateAccessToken(tokenStr, secret)
	require.NoError(t, err)
	assert.Equal(t, "usr_12345", claims.Subject)
	assert.Equal(t, "admin", claims.Username)
	assert.Equal(t, "admin@example.com", claims.Email)
	assert.Equal(t, []string{"super_admin"}, claims.Roles)

	// 2. Invalid Secret Validation
	_, err = ValidateAccessToken(tokenStr, "wrong-secret-key")
	assert.ErrorIs(t, err, ErrInvalidToken)

	// 3. Expired Token
	expiredTokenStr, err := GenerateAccessToken(user, secret, -1*time.Second)
	require.NoError(t, err)

	_, err = ValidateAccessToken(expiredTokenStr, secret)
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestGenerateAPIToken(t *testing.T) {
	rawToken, hash, err := GenerateAPIToken()
	require.NoError(t, err)

	assert.True(t, IsAPIToken(rawToken))
	assert.NotEmpty(t, hash)
	assert.Equal(t, 64, len(hash)) // SHA-256 hex string

	// Verify HashAPIToken matches
	recomputedHash := HashAPIToken(rawToken)
	assert.Equal(t, hash, recomputedHash)
}

func TestValidateAccessTokenIssuerAndAudience(t *testing.T) {
	secret := "test-super-secret-jwt-signing-key-123456"
	now := time.Now().UTC()

	makeToken := func(issuer string, audience jwt.ClaimStrings) string {
		claims := TokenClaims{
			Username: "admin",
			Email:    "admin@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "usr_1",
				Issuer:    issuer,
				Audience:  audience,
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			},
		}
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		require.NoError(t, err)
		return tok
	}

	_, err := ValidateAccessToken(makeToken("other-issuer", jwt.ClaimStrings{"go-cms-api"}), secret)
	assert.ErrorIs(t, err, ErrInvalidToken)

	_, err = ValidateAccessToken(makeToken("go-cms", jwt.ClaimStrings{"other-audience"}), secret)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestGenerateInviteToken(t *testing.T) {
	rawToken, hash, err := GenerateInviteToken()
	require.NoError(t, err)
	assert.Contains(t, rawToken, InviteTokenPrefix)
	assert.Len(t, hash, 64)
	assert.Equal(t, hash, HashOpaqueToken(rawToken))
}

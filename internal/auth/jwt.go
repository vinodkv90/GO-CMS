package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid or expired authentication token")
	ErrTokenExpired = errors.New("token has expired")
)

// TokenClaims represents the JWT payload claims for an authenticated user.
type TokenClaims struct {
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

// GenerateAccessToken generates a signed HMAC-SHA256 JWT access token.
func GenerateAccessToken(user *User, secret string, duration time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("cannot generate token: empty JWT secret")
	}

	roleSlugs := make([]string, 0, len(user.Roles))
	for _, r := range user.Roles {
		roleSlugs = append(roleSlugs, r.Slug)
	}

	now := time.Now().UTC()
	claims := TokenClaims{
		Username: user.Username,
		Email:    user.Email,
		Roles:    roleSlugs,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			Issuer:    "go-cms",
			Audience:  jwt.ClaimStrings{"go-cms-api"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
			ID:        uuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateAccessToken parses and validates a signed JWT token.
func ValidateAccessToken(tokenStr string, secret string) (*TokenClaims, error) {
	if tokenStr == "" || secret == "" {
		return nil, ErrInvalidToken
	}

	token, err := jwt.ParseWithClaims(tokenStr, &TokenClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

const APITokenPrefix = "cms_pat_"

// GenerateAPIToken creates a secure Personal Access Token and its SHA-256 database hash.
func GenerateAPIToken() (rawToken string, hash string, err error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random token bytes: %w", err)
	}

	rawToken = APITokenPrefix + hex.EncodeToString(randomBytes)
	hash = HashAPIToken(rawToken)
	return rawToken, hash, nil
}

// HashAPIToken computes the SHA-256 hash of a Personal Access Token.
func HashAPIToken(rawToken string) string {
	h := sha256.New()
	h.Write([]byte(strings.TrimSpace(rawToken)))
	return hex.EncodeToString(h.Sum(nil))
}

// IsAPIToken checks if a token string matches the Personal Access Token format.
func IsAPIToken(token string) bool {
	return strings.HasPrefix(strings.TrimSpace(token), APITokenPrefix)
}


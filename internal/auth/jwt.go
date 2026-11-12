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
	}, jwt.WithIssuer("go-cms"), jwt.WithAudience("go-cms-api"))

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

const (
	APITokenPrefix      = "cms_pat_"
	AdminTokenPrefix    = "cms_adm_"
	TransferTokenPrefix = "cms_trn_"
	InviteTokenPrefix   = "cms_inv_"
)

// GenerateAPIToken creates a secure Personal Access Token and its SHA-256 database hash.
func GenerateAPIToken() (rawToken string, hash string, err error) {
	return generateOpaqueToken(APITokenPrefix)
}

// GenerateTokenForKind creates an opaque token for the requested kind, returning raw token, display prefix, and SHA-256 hash.
func GenerateTokenForKind(kind TokenKind) (rawToken string, prefix string, hash string, err error) {
	p := APITokenPrefix
	switch kind {
	case TokenKindAdmin:
		p = AdminTokenPrefix
	case TokenKindTransfer:
		p = TransferTokenPrefix
	}
	rawToken, hash, err = generateOpaqueToken(p)
	if err != nil {
		return "", "", "", err
	}
	if len(rawToken) >= 16 {
		prefix = rawToken[:16]
	} else {
		prefix = rawToken
	}
	return rawToken, prefix, hash, nil
}

// GenerateInviteToken creates an opaque admin invitation token and its SHA-256 database hash.
func GenerateInviteToken() (rawToken string, hash string, err error) {
	return generateOpaqueToken(InviteTokenPrefix)
}

func generateOpaqueToken(prefix string) (rawToken string, hash string, err error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random token bytes: %w", err)
	}

	rawToken = prefix + hex.EncodeToString(randomBytes)
	hash = HashOpaqueToken(rawToken)
	return rawToken, hash, nil
}

// HashAPIToken computes the SHA-256 hash of a Personal Access Token.
func HashAPIToken(rawToken string) string {
	return HashOpaqueToken(rawToken)
}

// HashOpaqueToken computes the lowercase hex SHA-256 hash of any opaque token.
func HashOpaqueToken(rawToken string) string {
	h := sha256.New()
	h.Write([]byte(strings.TrimSpace(rawToken)))
	return hex.EncodeToString(h.Sum(nil))
}

// IsAPIToken checks if a token string matches any programmatic token prefix (API, Admin PAT, or Transfer).
func IsAPIToken(token string) bool {
	trimmed := strings.TrimSpace(token)
	return strings.HasPrefix(trimmed, APITokenPrefix) ||
		strings.HasPrefix(trimmed, AdminTokenPrefix) ||
		strings.HasPrefix(trimmed, TransferTokenPrefix)
}


const (
	// AudienceMFAChallenge isolates short-lived MFA verification/setup JWTs from normal API access tokens.
	AudienceMFAChallenge = "go-cms-mfa-challenge"
)

// MFAChallengeClaims represents claims carried by a short-lived MFA verification or forced-enrollment token.
type MFAChallengeClaims struct {
	UserID  string `json:"uid"`
	Email   string `json:"email"`
	Purpose string `json:"purpose"` // "mfa_challenge" or "mfa_setup_challenge"
	jwt.RegisteredClaims
}

// GenerateMFAChallengeToken issues a short-lived JWT strictly scoped to completing MFA challenge or forced enrollment.
func GenerateMFAChallengeToken(userID, email, purpose, secret string, duration time.Duration) (string, error) {
	if duration <= 0 {
		duration = 5 * time.Minute
	}
	if purpose != "mfa_challenge" && purpose != "mfa_setup_challenge" {
		return "", fmt.Errorf("invalid MFA challenge purpose %q", purpose)
	}
	now := time.Now().UTC()
	claims := MFAChallengeClaims{
		UserID:  userID,
		Email:   email,
		Purpose: purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    "go-cms",
			Audience:  jwt.ClaimStrings{AudienceMFAChallenge},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateMFAChallengeToken verifies an MFA challenge/setup token and ensures its audience is NOT go-cms-api.
func ValidateMFAChallengeToken(tokenStr, secret string) (*MFAChallengeClaims, error) {
	token, err := jwt.ParseWithClaims(
		strings.TrimSpace(tokenStr),
		&MFAChallengeClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("%w: unexpected signing method", ErrInvalidToken)
			}
			return []byte(secret), nil
		},
		jwt.WithIssuer("go-cms"),
		jwt.WithAudience(AudienceMFAChallenge),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	claims, ok := token.Claims.(*MFAChallengeClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.Purpose != "mfa_challenge" && claims.Purpose != "mfa_setup_challenge" {
		return nil, fmt.Errorf("%w: invalid challenge purpose", ErrInvalidToken)
	}
	return claims, nil
}

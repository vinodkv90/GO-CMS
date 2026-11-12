package usersperm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	appJWTIssuer         = "go-cms-app"
	appJWTAudience       = "go-cms-app-users"
	oauthStateIssuer     = "go-cms-oauth-state"
	oauthStateAudience   = "go-cms-oauth-callback"
	maxOAuthResponseSize = 2 << 20 // 2 MB
)

// AppTokenClaims holds JWT claims strictly isolated to application end-users.
type AppTokenClaims struct {
	UserID     string `json:"uid"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	RoleID     string `json:"role_id"`
	RoleSlug   string `json:"role_slug"`
	TokenScope string `json:"token_scope"`
	jwt.RegisteredClaims
}

// oauthStateClaims binds OAuth2 PKCE verifier and redirect target inside a signed short-lived state token.
type oauthStateClaims struct {
	Provider      string `json:"provider"`
	Scope         string `json:"scope"`
	CodeVerifier  string `json:"cv"`
	RedirectAfter string `json:"ra,omitempty"`
	jwt.RegisteredClaims
}

func deriveAppJWTSecret(rawSecret string) []byte {
	sum := sha256.Sum256([]byte("go-cms-app-user-jwt:" + rawSecret))
	return sum[:]
}

func deriveOAuthStateSecret(rawSecret string) []byte {
	sum := sha256.Sum256([]byte("go-cms-oauth-state-jwt:" + rawSecret))
	return sum[:]
}

// GenerateAppAccessToken mints an HMAC JWT strictly scoped to end-user authentication.
func GenerateAppAccessToken(user *AppUser, secret string, duration time.Duration) (string, error) {
	if duration <= 0 {
		duration = 24 * time.Hour
	}
	roleSlug := ""
	if user.Role != nil {
		roleSlug = user.Role.Slug
	}
	now := time.Now().UTC()
	claims := AppTokenClaims{
		UserID:     user.ID,
		Email:      user.Email,
		Username:   user.Username,
		RoleID:     user.RoleID,
		RoleSlug:   roleSlug,
		TokenScope: "app_user",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			Issuer:    appJWTIssuer,
			Audience:  jwt.ClaimStrings{appJWTAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(deriveAppJWTSecret(secret))
}

// ValidateAppAccessToken parses and verifies an end-user JWT, rejecting admin or MFA tokens.
func ValidateAppAccessToken(tokenStr, secret string) (*AppTokenClaims, error) {
	parsed, err := jwt.ParseWithClaims(
		strings.TrimSpace(tokenStr),
		&AppTokenClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("%w: unexpected signing method", ErrAppUnauthorized)
			}
			return deriveAppJWTSecret(secret), nil
		},
		jwt.WithIssuer(appJWTIssuer),
		jwt.WithAudience(appJWTAudience),
	)
	if err != nil {
		return nil, ErrAppUnauthorized
	}
	claims, ok := parsed.Claims.(*AppTokenClaims)
	if !ok || !parsed.Valid || claims.TokenScope != "app_user" {
		return nil, ErrAppUnauthorized
	}
	return claims, nil
}

func generatePKCE() (verifier string, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func signOAuthState(provider, scope, codeVerifier, redirectAfter, secret string) (string, error) {
	now := time.Now().UTC()
	claims := oauthStateClaims{
		Provider:      provider,
		Scope:         scope,
		CodeVerifier:  codeVerifier,
		RedirectAfter: redirectAfter,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    oauthStateIssuer,
			Audience:  jwt.ClaimStrings{oauthStateAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(deriveOAuthStateSecret(secret))
}

func verifyOAuthState(stateToken, expectedProvider, expectedScope, secret string) (*oauthStateClaims, error) {
	parsed, err := jwt.ParseWithClaims(
		strings.TrimSpace(stateToken),
		&oauthStateClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidOAuthState
			}
			return deriveOAuthStateSecret(secret), nil
		},
		jwt.WithIssuer(oauthStateIssuer),
		jwt.WithAudience(oauthStateAudience),
	)
	if err != nil {
		return nil, ErrInvalidOAuthState
	}
	claims, ok := parsed.Claims.(*oauthStateClaims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidOAuthState
	}
	if claims.Provider != expectedProvider || claims.Scope != expectedScope {
		return nil, ErrInvalidOAuthState
	}
	return claims, nil
}

var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// isRestrictedAddr returns true if an IP is loopback, private, link-local, multicast, unspecified, or CGNAT.
func isRestrictedAddr(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if addr, ok := netip.AddrFromSlice(ip); ok {
		addr = addr.Unmap()
		if cgnatPrefix.Contains(addr) {
			return true
		}
	}
	return false
}

// ValidateExternalURL checks scheme and prevents direct SSRF targets against literal private/loopback/metadata IPs.
func ValidateExternalURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: invalid URL", ErrUnsafeURL)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return fmt.Errorf("%w: unsupported URL scheme %q", ErrUnsafeURL, parsed.Scheme)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("%w: local/internal host %q is not allowed", ErrUnsafeURL, host)
	}
	if ip := net.ParseIP(host); ip != nil && isRestrictedAddr(ip) {
		return fmt.Errorf("%w: private or loopback IP %q is not allowed", ErrUnsafeURL, host)
	}
	return nil
}

// newSSRFSafeHTTPClient creates an HTTP client that blocks connections to private/loopback/metadata IP addresses post-DNS-resolution.
func newSSRFSafeHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			ip := net.ParseIP(host)
			if isRestrictedAddr(ip) {
				return fmt.Errorf("%w: refused connection to restricted IP %s", ErrUnsafeURL, host)
			}
			return nil
		},
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return ValidateExternalURL(req.URL.String())
		},
	}
}

type externalUserProfile struct {
	Subject   string
	Email     string
	Username  string
	FirstName string
	LastName  string
}

func (s *Service) discoverOIDCWithClient(ctx context.Context, issuerURL string) (*OIDCDiscoveryResult, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(issuerURL), "/")
	if !s.allowUnsafeTestURLs {
		if err := ValidateExternalURL(trimmed); err != nil {
			return nil, err
		}
	}
	wellKnown := trimmed + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OIDC discovery document: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOAuthResponseSize))
	if err != nil {
		return nil, err
	}
	var res OIDCDiscoveryResult
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("invalid OIDC discovery JSON: %w", err)
	}
	if res.AuthorizationEndpoint == "" || res.TokenEndpoint == "" {
		return nil, errors.New("OIDC discovery response missing authorization_endpoint or token_endpoint")
	}
	return &res, nil
}

func (s *Service) exchangeCodeAndFetchProfile(ctx context.Context, provider *OAuthProvider, clientSecret, code, redirectURI, codeVerifier string) (*externalUserProfile, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", provider.ClientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}

	if !s.allowUnsafeTestURLs {
		if err := ValidateExternalURL(provider.TokenURL); err != nil {
			return nil, err
		}
	}
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := s.httpClient.Do(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("failed to contact OAuth token endpoint: %w", err)
	}
	defer tokenResp.Body.Close()
	tokenBody, err := io.ReadAll(io.LimitReader(tokenResp.Body, maxOAuthResponseSize))
	if err != nil {
		return nil, err
	}
	if tokenResp.StatusCode < 200 || tokenResp.StatusCode >= 300 {
		return nil, fmt.Errorf("OAuth token endpoint returned HTTP %d", tokenResp.StatusCode)
	}

	var tokenPayload struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	_ = json.Unmarshal(tokenBody, &tokenPayload)
	if tokenPayload.AccessToken == "" {
		// Support GitHub form-encoded fallback when Accept header ignored
		if q, qErr := url.ParseQuery(string(tokenBody)); qErr == nil {
			tokenPayload.AccessToken = q.Get("access_token")
		}
	}
	if tokenPayload.AccessToken == "" && tokenPayload.IDToken == "" {
		return nil, errors.New("OAuth token response missing access_token")
	}

	profile := &externalUserProfile{}
	if strings.TrimSpace(provider.UserinfoURL) != "" && tokenPayload.AccessToken != "" {
		if !s.allowUnsafeTestURLs {
			if err := ValidateExternalURL(provider.UserinfoURL); err != nil {
				return nil, err
			}
		}
		uReq, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.UserinfoURL, nil)
		if err != nil {
			return nil, err
		}
		uReq.Header.Set("Authorization", "Bearer "+tokenPayload.AccessToken)
		uReq.Header.Set("Accept", "application/json")
		uResp, err := s.httpClient.Do(uReq)
		if err != nil {
			return nil, fmt.Errorf("failed to contact OAuth userinfo endpoint: %w", err)
		}
		defer uResp.Body.Close()
		if uResp.StatusCode < 200 || uResp.StatusCode >= 300 {
			return nil, fmt.Errorf("OAuth userinfo endpoint returned HTTP %d", uResp.StatusCode)
		}
		uBody, err := io.ReadAll(io.LimitReader(uResp.Body, maxOAuthResponseSize))
		if err != nil {
			return nil, err
		}
		var raw map[string]any
		if err := json.Unmarshal(uBody, &raw); err != nil {
			return nil, fmt.Errorf("invalid OAuth userinfo JSON: %w", err)
		}
		profile.Subject = firstStringClaim(raw, "sub", "id")
		profile.Email = strings.ToLower(firstStringClaim(raw, "email"))
		profile.Username = firstStringClaim(raw, "preferred_username", "login", "username", "name")
		profile.FirstName = firstStringClaim(raw, "given_name")
		profile.LastName = firstStringClaim(raw, "family_name")

		// GitHub private email fallback when primary /user response omits email
		if profile.Email == "" && provider.ProviderType == "github" && !s.allowUnsafeTestURLs {
			if ghEmail := s.fetchGitHubPrimaryEmail(ctx, tokenPayload.AccessToken); ghEmail != "" {
				profile.Email = ghEmail
			}
		}
	}

	// Fallback / supplement from unverified JWT payload claims ONLY when returned directly over TLS token endpoint exchange
	if (profile.Email == "" || profile.Subject == "") && tokenPayload.IDToken != "" {
		parts := strings.Split(tokenPayload.IDToken, ".")
		if len(parts) >= 2 {
			if rawClaims, decErr := base64.RawURLEncoding.DecodeString(parts[1]); decErr == nil {
				var c map[string]any
				if json.Unmarshal(rawClaims, &c) == nil {
					if profile.Subject == "" {
						profile.Subject = firstStringClaim(c, "sub")
					}
					if profile.Email == "" {
						profile.Email = strings.ToLower(firstStringClaim(c, "email"))
					}
					if profile.Username == "" {
						profile.Username = firstStringClaim(c, "preferred_username", "name")
					}
					if profile.FirstName == "" {
						profile.FirstName = firstStringClaim(c, "given_name")
					}
					if profile.LastName == "" {
						profile.LastName = firstStringClaim(c, "family_name")
					}
				}
			}
		}
	}

	if profile.Email == "" {
		return nil, errors.New("OAuth provider did not return an email address")
	}
	if profile.Subject == "" {
		profile.Subject = profile.Email
	}
	if profile.Username == "" {
		profile.Username = strings.SplitN(profile.Email, "@", 2)[0]
	}
	return profile, nil
}

func (s *Service) fetchGitHubPrimaryEmail(ctx context.Context, accessToken string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOAuthResponseSize))
	if err != nil {
		return ""
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if json.Unmarshal(body, &emails) != nil {
		return ""
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return strings.ToLower(strings.TrimSpace(e.Email))
		}
	}
	for _, e := range emails {
		if e.Verified {
			return strings.ToLower(strings.TrimSpace(e.Email))
		}
	}
	return ""
}

func firstStringClaim(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch typed := v.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return strings.TrimSpace(typed)
				}
			case float64:
				return fmt.Sprintf("%.0f", typed)
			}
		}
	}
	return ""
}


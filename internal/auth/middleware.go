package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const (
	UserContextKey     contextKey = "cms_auth_user"
	ClaimsContextKey   contextKey = "cms_auth_claims"
	APITokenContextKey contextKey = "cms_auth_api_token"
	SessionCookieName             = "cms_token"
	CSRFCookieName                = "cms_csrf"
)

// GetUserFromContext retrieves the authenticated User entity from context.
func GetUserFromContext(ctx context.Context) *User {
	if ctx == nil {
		return nil
	}
	if u, ok := ctx.Value(UserContextKey).(*User); ok {
		return u
	}
	return nil
}

// GetTokenClaimsFromContext retrieves the JWT claims from context.
func GetTokenClaimsFromContext(ctx context.Context) *TokenClaims {
	if ctx == nil {
		return nil
	}
	if c, ok := ctx.Value(ClaimsContextKey).(*TokenClaims); ok {
		return c
	}
	return nil
}

// GetAPITokenFromContext retrieves the Personal Access Token from context.
func GetAPITokenFromContext(ctx context.Context) *APIToken {
	if ctx == nil {
		return nil
	}
	if t, ok := ctx.Value(APITokenContextKey).(*APIToken); ok {
		return t
	}
	return nil
}

// AuthMiddleware inspects incoming requests for Bearer JWT or Personal Access Tokens or Session Cookies.
func AuthMiddleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Case 1: Personal Access Token (PAT)
			if IsAPIToken(tokenStr) {
				apiToken, user, err := svc.ValidateAPIToken(r.Context(), tokenStr)
				if err == nil && user != nil {
					ctx := context.WithValue(r.Context(), UserContextKey, user)
					ctx = context.WithValue(ctx, APITokenContextKey, apiToken)
					r = r.WithContext(ctx)
				}
				next.ServeHTTP(w, r)
				return
			}

			// Case 2: JWT Access Token
			claims, err := ValidateAccessToken(tokenStr, svc.cfg.Auth.JWTSecret)
			if err == nil && claims != nil {
				user, err := svc.GetUserProfile(r.Context(), claims.Subject)
				if err == nil && user != nil && user.Status == StatusActive {
					ctx := context.WithValue(r.Context(), UserContextKey, user)
					ctx = context.WithValue(ctx, ClaimsContextKey, claims)
					r = r.WithContext(ctx)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth enforces that the request has an authenticated user.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())
		if user == nil {
			respondAuthError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission enforces that the authenticated user possesses the required RBAC permission.
func RequirePermission(action, resource string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				respondAuthError(w, http.StatusUnauthorized, "Authentication required")
				return
			}

			// If authenticated via API Token, check token permissions
			if apiToken := GetAPITokenFromContext(r.Context()); apiToken != nil {
				if !MatchTokenPermission(apiToken.Permissions, action, resource) {
					respondAuthError(w, http.StatusForbidden, "Forbidden: token has insufficient permissions")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Check user RBAC permissions
			if !HasPermission(user, action, resource) {
				respondAuthError(w, http.StatusForbidden, "Forbidden: insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole enforces that the user possesses at least one of the specified roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				respondAuthError(w, http.StatusUnauthorized, "Authentication required")
				return
			}

			if !HasAnyRole(user, roles...) {
				respondAuthError(w, http.StatusForbidden, "Forbidden: role required")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractToken retrieves the token string from Authorization header or cookie.
func extractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		return strings.TrimSpace(cookie.Value)
	}

	return ""
}

// SetAuthCookie writes an HTTP-only secure cookie containing the authentication token.
func SetAuthCookie(w http.ResponseWriter, token string, maxAge time.Duration, isProduction bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   isProduction,
		SameSite: http.SameSiteLaxMode,
	})

	csrfBytes := make([]byte, 32)
	if _, err := rand.Read(csrfBytes); err == nil {
		http.SetCookie(w, &http.Cookie{
			Name:     CSRFCookieName,
			Value:    hex.EncodeToString(csrfBytes),
			Path:     "/",
			MaxAge:   int(maxAge.Seconds()),
			HttpOnly: false,
			Secure:   isProduction,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// ClearAuthCookie clears the session cookie.
func ClearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{Name: CSRFCookieName, Value: "", Path: "/", MaxAge: -1, SameSite: http.SameSiteLaxMode})
}

// CSRFMiddleware protects cookie-authenticated state-changing requests with a
// double-submit token. Bearer-token requests are not vulnerable to browser
// cookie CSRF and are left unchanged.
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions &&
			r.Header.Get("Authorization") == "" {
			if _, err := r.Cookie(SessionCookieName); err == nil {
				cookie, cookieErr := r.Cookie(CSRFCookieName)
				header := r.Header.Get("X-CSRF-Token")
				if cookieErr != nil || header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
					respondAuthError(w, http.StatusForbidden, "CSRF validation failed")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func respondAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

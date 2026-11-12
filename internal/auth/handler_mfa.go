package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body LoginRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	identifier := strings.TrimSpace(body.Identifier)
	if identifier == "" {
		identifier = strings.TrimSpace(body.Username)
	}

	res, err := h.svc.AuthenticateWithOptions(r.Context(), identifier, body.Password, r.RemoteAddr)
	if err != nil {
		switch {
		case errors.Is(err, ErrPasswordExpired):
			respondJSON(w, http.StatusForbidden, map[string]any{
				"error":            err.Error(),
				"password_expired": true,
				"code":             "PASSWORD_EXPIRED",
			})
		case errors.Is(err, ErrAccountLocked):
			respondJSON(w, http.StatusTooManyRequests, map[string]any{
				"error": err.Error(),
				"code":  "ACCOUNT_LOCKED",
			})
		case errors.Is(err, ErrIPNotAllowed):
			respondJSON(w, http.StatusForbidden, map[string]any{
				"error": err.Error(),
				"code":  "IP_NOT_ALLOWED",
			})
		case errors.Is(err, ErrSSOOnlyAccount):
			respondJSON(w, http.StatusForbidden, map[string]any{
				"error": err.Error(),
				"code":  "SSO_REQUIRED",
			})
		case errors.Is(err, ErrUserSuspended):
			respondJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
		default:
			respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
		}
		return
	}

	if res.MFARequired {
		respondJSON(w, http.StatusOK, map[string]any{
			"mfa_required": true,
			"mfa_token":    res.MFAToken,
			"user":         res.User,
		})
		return
	}
	if res.MFASetupRequired {
		respondJSON(w, http.StatusOK, map[string]any{
			"mfa_setup_required": true,
			"mfa_token":          res.MFAToken,
			"user":               res.User,
		})
		return
	}

	h.issueLoginResponse(w, r, res)
}

func (h *Handler) issueLoginResponse(w http.ResponseWriter, r *http.Request, res *LoginResult) {
	isProd := h.svc.cfg != nil && h.svc.cfg.Server.Env == "production"
	ttl := h.svc.sessionTTL(h.svc.securityPolicy(r.Context()))
	SetAuthCookie(w, res.Token, ttl, isProd)
	respondJSON(w, http.StatusOK, map[string]any{
		"token": res.Token,
		"user":  res.User,
	})
}

// resolveMFAUserID extracts the user ID from either an active authenticated user context
// or a scoped mfa_setup_challenge token (passed via body or Authorization header).
func (h *Handler) resolveMFAUserID(r *http.Request, bodyToken string) (string, error) {
	if user := GetUserFromContext(r.Context()); user != nil {
		if GetAPITokenFromContext(r.Context()) != nil {
			return "", ErrForbidden
		}
		return user.ID, nil
	}

	tok := strings.TrimSpace(bodyToken)
	if tok == "" {
		if b, ok := bearerToken(r); ok {
			tok = b
		}
	}
	if tok == "" {
		return "", ErrUnauthorized
	}
	claims, err := ValidateMFAChallengeToken(tok, h.svc.cfg.Auth.JWTSecret)
	if err != nil || claims.Purpose != "mfa_setup_challenge" {
		return "", ErrUnauthorized
	}
	return claims.UserID, nil
}

// EnrollMFA starts TOTP enrollment and returns the secret + otpauth URI.
func (h *Handler) EnrollMFA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MFAToken string `json:"mfa_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	userID, err := h.resolveMFAUserID(r, body.MFAToken)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	res, err := h.svc.EnrollMFA(r.Context(), userID)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"data": res})
}

// ConfirmMFA verifies the initial TOTP code, enables MFA, returns backup recovery codes, and sets a session cookie/JWT.
func (h *Handler) ConfirmMFA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code     string `json:"code"`
		MFAToken string `json:"mfa_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	userID, err := h.resolveMFAUserID(r, body.MFAToken)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	res, err := h.svc.ConfirmMFA(r.Context(), userID, body.Code)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}

	isProd := h.svc.cfg != nil && h.svc.cfg.Server.Env == "production"
	SetAuthCookie(w, res.Token, h.svc.sessionTTL(h.svc.securityPolicy(r.Context())), isProd)
	respondJSON(w, http.StatusOK, map[string]any{"data": res})
}

// VerifyMFALogin completes step 2 of MFA login using a TOTP code or single-use backup recovery code.
func (h *Handler) VerifyMFALogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MFAToken string `json:"mfa_token"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	res, err := h.svc.VerifyMFALogin(r.Context(), body.MFAToken, body.Code, r.RemoteAddr)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	h.issueLoginResponse(w, r, res)
}

// DisableMFA disables TOTP multi-factor authentication on the current account.
func (h *Handler) DisableMFA(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if GetAPITokenFromContext(r.Context()) != nil {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "api tokens cannot disable MFA"})
		return
	}
	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updated, err := h.svc.DisableMFA(r.Context(), user.ID, body.Password, body.Code)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// RegenerateRecoveryCodes replaces single-use recovery codes after validating an active TOTP code.
func (h *Handler) RegenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if GetAPITokenFromContext(r.Context()) != nil {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "api tokens cannot regenerate recovery codes"})
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	codes, err := h.svc.RegenerateRecoveryCodes(r.Context(), user.ID, body.Code)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"recovery_codes": codes}})
}

// ResetExpiredPassword rotates an expired password and completes authentication.
func (h *Handler) ResetExpiredPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Identifier      string `json:"identifier"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	res, err := h.svc.ResetExpiredPassword(r.Context(), body.Identifier, body.CurrentPassword, body.NewPassword, r.RemoteAddr)
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	if res.MFARequired {
		respondJSON(w, http.StatusOK, map[string]any{
			"mfa_required": true,
			"mfa_token":    res.MFAToken,
			"user":         res.User,
		})
		return
	}
	if res.MFASetupRequired {
		respondJSON(w, http.StatusOK, map[string]any{
			"mfa_setup_required": true,
			"mfa_token":          res.MFAToken,
			"user":               res.User,
		})
		return
	}
	h.issueLoginResponse(w, r, res)
}

// UnlockUser clears brute-force lockout counters for an administrator account.
func (h *Handler) UnlockUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	updated, err := h.svc.UnlockUser(r.Context(), targetID, auditMetaFromRequest(r))
	if err != nil {
		respondJSON(w, mapAuthServiceErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, updated)
}

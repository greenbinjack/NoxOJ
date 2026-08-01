package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rs/zerolog"

	"noxoj/internal/auth"
	"noxoj/internal/config"
	"noxoj/internal/middleware"
	"noxoj/internal/ratelimit"
	"noxoj/internal/repository"
	"noxoj/internal/tokenstore"
)

// AuthHandler owns session/token concerns — login, refresh, logout,
// password reset — as opposed to UserHandler's resource concern
// (creating an account).
type AuthHandler struct {
	logger        zerolog.Logger
	users         *repository.UserRepository
	roles         *repository.RoleRepository
	jwtSecret     []byte
	loginLimiter  *ratelimit.LoginLimiter
	refreshTokens *tokenstore.RefreshTokenStore
	resetTokens   *tokenstore.PasswordResetTokenStore
	environment   config.Environment
}

func NewAuthHandler(
	logger zerolog.Logger,
	users *repository.UserRepository,
	roles *repository.RoleRepository,
	jwtSecret []byte,
	loginLimiter *ratelimit.LoginLimiter,
	refreshTokens *tokenstore.RefreshTokenStore,
	resetTokens *tokenstore.PasswordResetTokenStore,
	environment config.Environment,
) *AuthHandler {
	return &AuthHandler{
		logger:        logger,
		users:         users,
		roles:         roles,
		jwtSecret:     jwtSecret,
		loginLimiter:  loginLimiter,
		refreshTokens: refreshTokens,
		resetTokens:   resetTokens,
		environment:   environment,
	}
}

func (h *AuthHandler) setAccessTokenCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessTokenCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		// A config VALUE differing by environment, not a code-path
		// difference — see Sprint 9's note on this same pattern.
		Secure:   h.environment == config.Production,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(auth.AccessTokenTTL.Seconds()),
	})
}

func (h *AuthHandler) setRefreshTokenCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.RefreshTokenCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.environment == config.Production,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(tokenstore.TTL.Seconds()),
	})
}

func (h *AuthHandler) clearAuthCookies(w http.ResponseWriter) {
	for _, name := range [2]string{middleware.AccessTokenCookieName, middleware.RefreshTokenCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   h.environment == config.Production,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
		})
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if !h.loginLimiter.Allowed(req.Username) {
		writeError(w, http.StatusTooManyRequests, "too many failed attempts — try again later")
		return
	}

	user, err := h.users.GetByUsername(r.Context(), req.Username)
	if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
		h.logger.Error().Err(err).Msg("failed to look up user for login")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Always run a real bcrypt comparison, even when the user doesn't
	// exist — see internal/auth.DummyHash for why (timing-safe against
	// username enumeration).
	hashToCheck := auth.DummyHash()
	if user != nil && user.PasswordHash != nil {
		hashToCheck = *user.PasswordHash
	}
	passwordErr := auth.CheckPassword(hashToCheck, req.Password)

	if user == nil || passwordErr != nil {
		h.loginLimiter.RecordFailure(req.Username)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	h.loginLimiter.RecordSuccess(req.Username)

	roleNames, err := h.roles.GetRoleNames(r.Context(), user.ID)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to load roles for login")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	accessToken, err := auth.GenerateAccessToken(user.ID, roleNames, h.jwtSecret)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to generate access token")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	refreshToken, err := h.refreshTokens.Issue(r.Context(), user.ID)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to issue refresh token")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.setAccessTokenCookie(w, accessToken)
	h.setRefreshTokenCookie(w, refreshToken)

	writeJSON(w, http.StatusOK, loginResponse{
		ID:       user.ID.String(),
		Username: user.Username,
	})
}

// Refresh trades a valid, unused refresh token for a brand new pair
// of tokens — both rotate together. The old refresh token is consumed
// (single-use, see tokenstore.Consume) whether or not anything after
// that point succeeds, so a partially-failed refresh can't leave a
// token usable twice.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(middleware.RefreshTokenCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no refresh token provided")
		return
	}

	userID, err := h.refreshTokens.Consume(r.Context(), cookie.Value)
	if errors.Is(err, tokenstore.ErrTokenNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to consume refresh token")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Roles are re-fetched here, not carried over from the old token —
	// this is the moment a role change (promotion/demotion) actually
	// takes effect, since the access token itself can't be updated
	// mid-flight.
	roleNames, err := h.roles.GetRoleNames(r.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to load roles for refresh")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	newAccessToken, err := auth.GenerateAccessToken(userID, roleNames, h.jwtSecret)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to generate access token")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	newRefreshToken, err := h.refreshTokens.Issue(r.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to issue refresh token")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.setAccessTokenCookie(w, newAccessToken)
	h.setRefreshTokenCookie(w, newRefreshToken)

	writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed"})
}

// Logout revokes the refresh token so it can never be used again
// (unlike the access token, which — being a stateless JWT — simply
// expires on its own within 15 minutes) and clears both cookies.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.RefreshTokenCookieName); err == nil {
		if err := h.refreshTokens.Revoke(r.Context(), cookie.Value); err != nil {
			h.logger.Error().Err(err).Msg("failed to revoke refresh token on logout")
			// Not fatal to the request — the client's cookies still get
			// cleared below, so it's logged out locally either way.
		}
	}

	h.clearAuthCookies(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

type passwordResetRequestRequest struct {
	Username string `json:"username"`
}

// passwordResetRequestResponse is the same fixed shape whether or not
// the account exists — see RequestPasswordReset.
type passwordResetRequestResponse struct {
	Status string `json:"status"`
}

// RequestPasswordReset issues a one-time reset token for the account
// with the given username. Always responds 200 with the same generic
// message regardless of whether that username actually exists —
// mirroring Login's enumeration resistance (Sprint 8), just via
// identical *responses* rather than identical *timing*: unlike login,
// there's no expensive operation (bcrypt) on the "user exists" path
// worth faking here, so there's no comparable timing signal to close.
//
// No email delivery exists yet (that's a later sprint) — the token is
// logged server-side instead, standing in for "the email that would
// be sent." That's a deliberate, temporary substitution, not a
// shortcut meant to reach production this way.
func (h *AuthHandler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	const genericResponse = "if that account exists, a password reset link has been issued"

	var req passwordResetRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	user, err := h.users.GetByUsername(r.Context(), req.Username)
	if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
		h.logger.Error().Err(err).Msg("failed to look up user for password reset")
		writeJSON(w, http.StatusOK, passwordResetRequestResponse{Status: genericResponse})
		return
	}

	if user != nil {
		token, err := h.resetTokens.Issue(r.Context(), user.ID)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to issue password reset token")
		} else {
			h.logger.Info().
				Str("username", user.Username).
				Str("reset_token", token).
				Msg("password reset requested — a real deployment would email this as a link, not log it")
		}
	}

	writeJSON(w, http.StatusOK, passwordResetRequestResponse{Status: genericResponse})
}

type passwordResetConfirmRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// ConfirmPasswordReset spends a reset token to set a new password.
// The token is consumed (single-use, see tokenstore.Consume) before
// anything else happens — a partially-failed confirm can't leave the
// token usable twice, same reasoning as Refresh's token rotation.
//
// On success, every existing refresh token for this user is revoked
// too (RevokeAllForUser) — a password reset is often a response to a
// compromised account, so it should also end any session an attacker
// may already hold, not just block their ability to log in again with
// the old password.
func (h *AuthHandler) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req passwordResetConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := validatePassword(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	userID, err := h.resetTokens.Consume(r.Context(), req.Token)
	if errors.Is(err, tokenstore.ErrPasswordResetTokenNotFound) {
		writeError(w, http.StatusBadRequest, "invalid or expired reset token")
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to consume password reset token")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to hash new password")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.users.UpdatePassword(r.Context(), userID, newHash); err != nil {
		h.logger.Error().Err(err).Msg("failed to update password after reset")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.refreshTokens.RevokeAllForUser(r.Context(), userID); err != nil {
		h.logger.Error().Err(err).Msg("failed to revoke sessions after password reset")
		// Not fatal to the request — the password itself is already
		// changed, which is the security-critical part; a stray
		// still-valid old session is a smaller residual problem than
		// failing the whole reset over a Redis hiccup.
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "password updated"})
}

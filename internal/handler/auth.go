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

// AuthHandler owns session/token concerns — login, refresh, logout —
// as opposed to UserHandler's resource concern (creating an account).
type AuthHandler struct {
	logger        zerolog.Logger
	users         *repository.UserRepository
	roles         *repository.RoleRepository
	jwtSecret     []byte
	loginLimiter  *ratelimit.LoginLimiter
	refreshTokens *tokenstore.RefreshTokenStore
	environment   config.Environment
}

func NewAuthHandler(
	logger zerolog.Logger,
	users *repository.UserRepository,
	roles *repository.RoleRepository,
	jwtSecret []byte,
	loginLimiter *ratelimit.LoginLimiter,
	refreshTokens *tokenstore.RefreshTokenStore,
	environment config.Environment,
) *AuthHandler {
	return &AuthHandler{
		logger:        logger,
		users:         users,
		roles:         roles,
		jwtSecret:     jwtSecret,
		loginLimiter:  loginLimiter,
		refreshTokens: refreshTokens,
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

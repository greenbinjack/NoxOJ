package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"noxoj/internal/auth"
	"noxoj/internal/domain"
	"noxoj/internal/middleware"
	"noxoj/internal/repository"
	"noxoj/internal/tokenstore"
)

// UserHandler owns user-resource concerns — creating an account,
// and (Sprint 14) an admin deactivating one. Session/token concerns
// (login, refresh, logout, password reset) live in AuthHandler — a
// different set of responsibilities with different dependencies.
type UserHandler struct {
	logger        zerolog.Logger
	users         *repository.UserRepository
	refreshTokens *tokenstore.RefreshTokenStore
}

func NewUserHandler(logger zerolog.Logger, users *repository.UserRepository, refreshTokens *tokenstore.RefreshTokenStore) *UserHandler {
	return &UserHandler{logger: logger, users: users, refreshTokens: refreshTokens}
}

type registerRequest struct {
	Username    string  `json:"username"`
	Email       *string `json:"email,omitempty"`
	Password    string  `json:"password"`
	DisplayName string  `json:"display_name"`
}

// registerResponse deliberately excludes the password and its hash —
// there's no reason to ever echo either back, even hashed.
type registerResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	Email       *string `json:"email,omitempty"`
	DisplayName string  `json:"display_name"`
	Rating      int     `json:"rating"`
}

// validatePassword is shared by registration and password reset
// (Sprint 13) — both need the same floor (real brute-force
// resistance) and ceiling (bcrypt silently truncates anything past 72
// bytes, so a longer password would quietly lose its extra length
// rather than actually being enforced).
func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(password) > 72 {
		return errors.New("password must be at most 72 characters")
	}
	return nil
}

func (req registerRequest) validate() error {
	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(username) > 32 {
		return errors.New("username must be between 3 and 32 characters")
	}
	for _, r := range username {
		isLower := r >= 'a' && r <= 'z'
		isUpper := r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		if !(isLower || isUpper || isDigit || r == '_') {
			return errors.New("username may only contain letters, digits, and underscores")
		}
	}

	if err := validatePassword(req.Password); err != nil {
		return err
	}

	if strings.TrimSpace(req.DisplayName) == "" {
		return errors.New("display_name is required")
	}

	if req.Email != nil && strings.TrimSpace(*req.Email) != "" {
		if _, err := mail.ParseAddress(*req.Email); err != nil {
			return errors.New("email is not a valid address")
		}
	}

	return nil
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to hash password")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user := &domain.User{
		Username:     strings.TrimSpace(req.Username),
		Email:        req.Email,
		PasswordHash: &passwordHash,
		DisplayName:  strings.TrimSpace(req.DisplayName),
	}

	created, err := h.users.CreateWithRole(r.Context(), user, domain.DefaultRole)
	switch {
	case errors.Is(err, repository.ErrUsernameTaken):
		writeError(w, http.StatusConflict, "username already taken")
		return
	case errors.Is(err, repository.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email already taken")
		return
	case err != nil:
		h.logger.Error().Err(err).Msg("failed to create user")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, registerResponse{
		ID:          created.ID.String(),
		Username:    created.Username,
		Email:       created.Email,
		DisplayName: created.DisplayName,
		Rating:      created.Rating,
	})
}

// meResponse is the canonical profile shape — a distinct type from
// registerResponse (a creation receipt) even though the fields
// overlap, since the two have different reasons to evolve.
type meResponse struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Email       *string  `json:"email,omitempty"`
	DisplayName string   `json:"display_name"`
	Rating      int      `json:"rating"`
	Roles       []string `json:"roles"`
}

// Me returns the authenticated user's own profile. Must run behind
// middleware.Authenticate — it trusts the request context to already
// hold a valid user ID.
//
// Roles come from the request context (the token's own claims, set by
// Authenticate) rather than a fresh database query — consistent with
// Sprint 11's design: this endpoint shows exactly what the current
// session believes about you, which may lag a real promotion/demotion
// by up to AccessTokenTTL, same as every other authorization check in
// the system. Everything else (rating, display name) genuinely can
// change between logins, so those come from a real, fresh read.
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	roles, _ := middleware.RolesFromContext(r.Context())

	user, err := h.users.GetByID(r.Context(), userID)
	if errors.Is(err, repository.ErrUserNotFound) {
		// A valid token for a user who no longer exists/is deleted —
		// treat it the same as "not authenticated," not a server error.
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to load profile")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, meResponse{
		ID:          user.ID.String(),
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Rating:      user.Rating,
		Roles:       roles,
	})
}

// Deactivate soft-deletes the user identified by the {id} URL
// parameter. Must run behind Authenticate + RequireRole(admin) — it
// trusts the request context to already hold a valid, admin-role
// actor ID.
//
// Refuses to let an admin deactivate their own account: nothing about
// deactivation itself is exclusive to other users, but a lone admin
// who accidentally locked themselves out (no reactivation endpoint
// exists yet, and a self-deactivation would also revoke their own
// active session below) would have no way back in. This is a cheap,
// unambiguous guard, not a judgment call worth debating per-request.
//
// On success, every refresh token the deactivated user holds is also
// revoked (RefreshTokenStore.RevokeAllForUser, the same mechanism
// Sprint 13's password reset uses) — a deactivated account
// shouldn't get to keep using a session it already had open.
func (h *UserHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if targetID == actorID {
		writeError(w, http.StatusBadRequest, "cannot deactivate your own account")
		return
	}

	if err := h.users.Deactivate(r.Context(), targetID, actorID); err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error().Err(err).Msg("failed to deactivate user")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.refreshTokens.RevokeAllForUser(r.Context(), targetID); err != nil {
		h.logger.Error().Err(err).Msg("failed to revoke sessions after deactivation")
		// Not fatal to the request — the account is already
		// deactivated, which is the security-critical part; a stray
		// still-valid old session is a smaller residual problem than
		// failing the whole deactivation over a Redis hiccup.
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "user deactivated"})
}

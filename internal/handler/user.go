package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/rs/zerolog"

	"noxoj/internal/auth"
	"noxoj/internal/config"
	"noxoj/internal/domain"
	"noxoj/internal/middleware"
	"noxoj/internal/ratelimit"
	"noxoj/internal/repository"
)

type UserHandler struct {
	logger       zerolog.Logger
	users        *repository.UserRepository
	jwtSecret    []byte
	loginLimiter *ratelimit.LoginLimiter
	environment  config.Environment
}

func NewUserHandler(
	logger zerolog.Logger,
	users *repository.UserRepository,
	jwtSecret []byte,
	loginLimiter *ratelimit.LoginLimiter,
	environment config.Environment,
) *UserHandler {
	return &UserHandler{
		logger:       logger,
		users:        users,
		jwtSecret:    jwtSecret,
		loginLimiter: loginLimiter,
		environment:  environment,
	}
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

	if len(req.Password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	// bcrypt silently ignores any input past 72 bytes — reject early
	// instead of letting a long password quietly lose its extra length.
	if len(req.Password) > 72 {
		return errors.New("password must be at most 72 characters")
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

	created, err := h.users.Create(r.Context(), user)
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

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
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
	// exist — comparing against auth.DummyHash() keeps the response
	// time consistent either way. Skipping straight to "no such user"
	// would respond near-instantly, while a real wrong-password check
	// takes bcrypt's ~100ms — a timing difference an attacker could
	// use to enumerate valid usernames without ever seeing an error
	// message say so.
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

	token, err := auth.GenerateAccessToken(user.ID, h.jwtSecret)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to generate access token")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessTokenCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		// Secure requires HTTPS, which local development over plain
		// http://localhost doesn't have — this is a config VALUE
		// differing by environment (like PORT and POSTGRES_HOST
		// before it), not a code path difference: the same code runs
		// everywhere, only this one flag's value changes.
		Secure:   h.environment == config.Production,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(auth.AccessTokenTTL.Seconds()),
	})

	writeJSON(w, http.StatusOK, loginResponse{
		ID:       user.ID.String(),
		Username: user.Username,
	})
}

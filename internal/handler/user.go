package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/rs/zerolog"

	"noxoj/internal/auth"
	"noxoj/internal/domain"
	"noxoj/internal/repository"
)

// UserHandler owns user-resource concerns (creating an account).
// Session/token concerns (login, refresh, logout) live in AuthHandler
// — a different set of responsibilities with different dependencies.
type UserHandler struct {
	logger zerolog.Logger
	users  *repository.UserRepository
}

func NewUserHandler(logger zerolog.Logger, users *repository.UserRepository) *UserHandler {
	return &UserHandler{logger: logger, users: users}
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

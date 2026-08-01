package tokenstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// PasswordResetTTL is deliberately short relative to RefreshTokenStore's
// TTL — a reset link is a single sensitive action, not an ongoing
// session, so it should stop being valid well before someone would
// plausibly still need it but long enough to actually click an email
// link.
const PasswordResetTTL = 1 * time.Hour

var ErrPasswordResetTokenNotFound = errors.New("password reset token not found or already used")

const passwordResetKeyPrefix = "pwreset:"

type PasswordResetTokenStore struct {
	client *redis.Client
}

func NewPasswordResetTokenStore(client *redis.Client) *PasswordResetTokenStore {
	return &PasswordResetTokenStore{client: client}
}

// Issue generates a new random reset token for userID and returns it
// — the caller is responsible for getting it to the user (for now,
// logging it; see AuthHandler.RequestPasswordReset). Reuses the same
// generateToken helper as RefreshTokenStore.Issue: both need the same
// property, a token an attacker can't guess or reproduce.
func (s *PasswordResetTokenStore) Issue(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("generating password reset token: %w", err)
	}

	if err := s.client.Set(ctx, passwordResetKeyPrefix+token, userID.String(), PasswordResetTTL).Err(); err != nil {
		return "", fmt.Errorf("storing password reset token: %w", err)
	}

	return token, nil
}

// Consume looks up token and atomically deletes it (GETDEL) — same
// one-time-use guarantee as RefreshTokenStore.Consume, for the same
// reason: a reset link that could be used twice is a reset link that
// could be used by whoever saw it first, not necessarily its intended
// recipient.
func (s *PasswordResetTokenStore) Consume(ctx context.Context, token string) (uuid.UUID, error) {
	val, err := s.client.GetDel(ctx, passwordResetKeyPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, ErrPasswordResetTokenNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("consuming password reset token: %w", err)
	}

	userID, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parsing stored user ID: %w", err)
	}
	return userID, nil
}

// Package tokenstore manages refresh tokens in Redis.
package tokenstore

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// TTL is how long an unused refresh token stays valid. Long-lived
// relative to the 15-minute access token (Sprint 9) — that's the
// whole point of splitting them — but it's not indefinite, and it's
// enforced by Redis itself (key expiry), not application code that
// could forget to check an expiry field.
const TTL = 7 * 24 * time.Hour

var ErrTokenNotFound = errors.New("refresh token not found or already used")

const keyPrefix = "refresh:"

type RefreshTokenStore struct {
	client *redis.Client
}

func NewRefreshTokenStore(client *redis.Client) *RefreshTokenStore {
	return &RefreshTokenStore{client: client}
}

// Issue generates a new random refresh token for userID, stores it,
// and returns the token string to hand to the client.
func (s *RefreshTokenStore) Issue(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}

	if err := s.client.Set(ctx, keyPrefix+token, userID.String(), TTL).Err(); err != nil {
		return "", fmt.Errorf("storing refresh token: %w", err)
	}

	return token, nil
}

// Consume looks up token and atomically deletes it in the same Redis
// round-trip (GETDEL) — a token can only ever be consumed once. This
// is what makes rotation actually safe: without atomicity, two
// concurrent refresh requests using the same token (e.g. a stolen one
// being raced against the legitimate client) could both succeed
// before either delete lands. With GETDEL, only one request can ever
// win.
func (s *RefreshTokenStore) Consume(ctx context.Context, token string) (uuid.UUID, error) {
	val, err := s.client.GetDel(ctx, keyPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, ErrTokenNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("consuming refresh token: %w", err)
	}

	userID, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parsing stored user ID: %w", err)
	}
	return userID, nil
}

// Revoke deletes token without returning it — used by logout, where
// the caller doesn't need the user ID back.
func (s *RefreshTokenStore) Revoke(ctx context.Context, token string) error {
	return s.client.Del(ctx, keyPrefix+token).Err()
}

// generateToken produces a cryptographically random 32-byte token,
// base64url-encoded. crypto/rand, never math/rand — math/rand is
// deterministic from its seed and predictable; anything used as a
// credential must come from a source an attacker can't guess or
// reproduce.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

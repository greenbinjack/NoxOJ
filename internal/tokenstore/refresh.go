// Package tokenstore manages short-lived, single-use credentials in
// Redis — refresh tokens (this file) and password reset tokens
// (passwordreset.go). Both follow the same shape (random token ->
// user ID, atomically consumed on use, expiring on its own via Redis
// key TTL) for the same reason: neither should be forgeable,
// replayable, or need manual cleanup.
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

// userTokensKeyPrefix indexes the set of currently-outstanding token
// strings issued to a given user, purely so RevokeAllForUser (Sprint
// 13) can find them — the token keys themselves (keyPrefix) remain
// the single source of truth for whether a token is actually valid.
const userTokensKeyPrefix = "refresh:byuser:"

type RefreshTokenStore struct {
	client *redis.Client
}

func NewRefreshTokenStore(client *redis.Client) *RefreshTokenStore {
	return &RefreshTokenStore{client: client}
}

// Issue generates a new random refresh token for userID, stores it,
// and returns the token string to hand to the client. Also records
// the token in userID's index set (re-extending that set's own TTL to
// match) so a later RevokeAllForUser can find every token this user
// currently holds without scanning the whole keyspace.
func (s *RefreshTokenStore) Issue(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}

	if err := s.client.Set(ctx, keyPrefix+token, userID.String(), TTL).Err(); err != nil {
		return "", fmt.Errorf("storing refresh token: %w", err)
	}

	setKey := userTokensKeyPrefix + userID.String()
	if err := s.client.SAdd(ctx, setKey, token).Err(); err != nil {
		return "", fmt.Errorf("indexing refresh token: %w", err)
	}
	if err := s.client.Expire(ctx, setKey, TTL).Err(); err != nil {
		return "", fmt.Errorf("setting refresh token index expiry: %w", err)
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

	// Best-effort index cleanup — the token is already unusable from
	// the GetDel above regardless of whether this succeeds, so a
	// failure here isn't reported: worst case a stale token string
	// lingers in the set until RevokeAllForUser tries (harmlessly) to
	// delete an already-gone key.
	s.client.SRem(ctx, userTokensKeyPrefix+val, token)

	return userID, nil
}

// Revoke deletes token — used by logout. Looks the value up first
// (rather than a blind DEL) so it can also drop the token from its
// owner's index set; same best-effort cleanup reasoning as Consume.
func (s *RefreshTokenStore) Revoke(ctx context.Context, token string) error {
	val, err := s.client.GetDel(ctx, keyPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}
	s.client.SRem(ctx, userTokensKeyPrefix+val, token)
	return nil
}

// RevokeAllForUser invalidates every refresh token currently issued
// to userID — used after a password reset (Sprint 13), where a
// changed password should also kick out any session an attacker might
// already hold, not just block future logins. Reads the index set
// built by Issue rather than scanning the whole keyspace.
func (s *RefreshTokenStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	setKey := userTokensKeyPrefix + userID.String()

	tokens, err := s.client.SMembers(ctx, setKey).Result()
	if err != nil {
		return fmt.Errorf("listing refresh tokens for user: %w", err)
	}
	if len(tokens) == 0 {
		return nil
	}

	keys := make([]string, len(tokens))
	for i, t := range tokens {
		keys[i] = keyPrefix + t
	}
	if err := s.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("revoking refresh tokens: %w", err)
	}

	return s.client.Del(ctx, setKey).Err()
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

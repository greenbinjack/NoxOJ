// These tests need a real Redis reachable at localhost:6379 (matches
// docker-compose.yml / .env.example defaults).
package tokenstore

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func testResetStore(t *testing.T) *PasswordResetTokenStore {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("unexpected error connecting to redis: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return NewPasswordResetTokenStore(client)
}

func TestPasswordResetIssueAndConsume(t *testing.T) {
	store := testResetStore(t)
	ctx := context.Background()
	userID := uuid.New()

	token, err := store.Issue(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error issuing: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty token")
	}

	gotID, err := store.Consume(ctx, token)
	if err != nil {
		t.Fatalf("unexpected error consuming: %v", err)
	}
	if gotID != userID {
		t.Errorf("expected user ID %s, got %s", userID, gotID)
	}
}

func TestPasswordResetConsume_OneTimeUse(t *testing.T) {
	store := testResetStore(t)
	ctx := context.Background()

	token, err := store.Issue(ctx, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error issuing: %v", err)
	}

	if _, err := store.Consume(ctx, token); err != nil {
		t.Fatalf("unexpected error on first consume: %v", err)
	}

	if _, err := store.Consume(ctx, token); err == nil {
		t.Fatal("expected the second consume of the same token to fail")
	}
}

func TestPasswordResetConsume_UnknownToken(t *testing.T) {
	store := testResetStore(t)

	if _, err := store.Consume(context.Background(), "this-token-was-never-issued"); err == nil {
		t.Fatal("expected an error for a token that was never issued")
	}
}

func TestPasswordResetTokens_AreIndependentOfRefreshTokens(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("unexpected error connecting to redis: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	refreshStore := NewRefreshTokenStore(client)
	resetStore := NewPasswordResetTokenStore(client)
	ctx := context.Background()
	userID := uuid.New()

	refreshToken, err := refreshStore.Issue(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error issuing refresh token: %v", err)
	}

	// A refresh token must never be consumable as a password reset
	// token — they share a Redis instance but must not share a
	// keyspace (different prefixes), or a valid refresh token would
	// let its holder reset the password too.
	if _, err := resetStore.Consume(ctx, refreshToken); err == nil {
		t.Fatal("expected a refresh token to be rejected by the password reset store")
	}
}

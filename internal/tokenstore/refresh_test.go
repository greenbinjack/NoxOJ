// These tests need a real Redis reachable at localhost:6379 (matches
// docker-compose.yml / .env.example defaults).
package tokenstore

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func testStore(t *testing.T) *RefreshTokenStore {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("unexpected error connecting to redis: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return NewRefreshTokenStore(client)
}

func TestIssueAndConsume(t *testing.T) {
	store := testStore(t)
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

func TestConsume_OneTimeUse(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	token, err := store.Issue(ctx, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error issuing: %v", err)
	}

	if _, err := store.Consume(ctx, token); err != nil {
		t.Fatalf("unexpected error on first consume: %v", err)
	}

	// Second consume of the same token — this is rotation's whole
	// point: a token is single-use, so reusing an already-rotated
	// one (e.g. a stolen copy) fails.
	if _, err := store.Consume(ctx, token); err == nil {
		t.Fatal("expected the second consume of the same token to fail")
	}
}

func TestConsume_UnknownToken(t *testing.T) {
	store := testStore(t)

	if _, err := store.Consume(context.Background(), "this-token-was-never-issued"); err == nil {
		t.Fatal("expected an error for a token that was never issued")
	}
}

func TestRevoke(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	token, err := store.Issue(ctx, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error issuing: %v", err)
	}

	if err := store.Revoke(ctx, token); err != nil {
		t.Fatalf("unexpected error revoking: %v", err)
	}

	if _, err := store.Consume(ctx, token); err == nil {
		t.Fatal("expected a revoked token to be unusable")
	}
}

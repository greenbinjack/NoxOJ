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

func TestRevoke_UnknownTokenIsANoOp(t *testing.T) {
	store := testStore(t)

	if err := store.Revoke(context.Background(), "this-token-was-never-issued"); err != nil {
		t.Fatalf("expected revoking an unknown token to be a no-op, got error: %v", err)
	}
}

func TestRevokeAllForUser(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	userID := uuid.New()

	// This user holds three concurrently-valid refresh tokens (e.g.
	// logged in from three devices).
	tokenA, err := store.Issue(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error issuing token A: %v", err)
	}
	tokenB, err := store.Issue(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error issuing token B: %v", err)
	}
	tokenC, err := store.Issue(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error issuing token C: %v", err)
	}

	if err := store.RevokeAllForUser(ctx, userID); err != nil {
		t.Fatalf("unexpected error revoking all: %v", err)
	}

	for name, token := range map[string]string{"A": tokenA, "B": tokenB, "C": tokenC} {
		if _, err := store.Consume(ctx, token); err == nil {
			t.Errorf("expected token %s to be revoked, but it was still consumable", name)
		}
	}
}

func TestRevokeAllForUser_DoesNotAffectOtherUsers(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	targetUser := uuid.New()
	otherUser := uuid.New()

	targetToken, err := store.Issue(ctx, targetUser)
	if err != nil {
		t.Fatalf("unexpected error issuing target token: %v", err)
	}
	otherToken, err := store.Issue(ctx, otherUser)
	if err != nil {
		t.Fatalf("unexpected error issuing other token: %v", err)
	}

	if err := store.RevokeAllForUser(ctx, targetUser); err != nil {
		t.Fatalf("unexpected error revoking: %v", err)
	}

	if _, err := store.Consume(ctx, targetToken); err == nil {
		t.Error("expected the target user's token to be revoked")
	}
	if _, err := store.Consume(ctx, otherToken); err != nil {
		t.Errorf("expected the other user's token to remain valid, got error: %v", err)
	}
}

func TestRevokeAllForUser_NoTokensIsANoOp(t *testing.T) {
	store := testStore(t)

	if err := store.RevokeAllForUser(context.Background(), uuid.New()); err != nil {
		t.Fatalf("expected revoking a user with no tokens to be a no-op, got error: %v", err)
	}
}

// See internal/database/database_test.go for the live-Postgres
// prerequisite these tests share.
package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"noxoj/internal/database"
	"noxoj/internal/domain"
)

func testDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := database.Connect(database.Config{
		Host:     "localhost",
		Port:     5432,
		User:     "noxoj",
		Password: "noxoj_dev_password",
		Name:     "noxoj",
	})
	if err != nil {
		t.Fatalf("unexpected error connecting: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUserRepository_Create(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	email := "sprint8-repo-test@example.com"
	user := &domain.User{
		Username:    "sprint8_repo_test",
		Email:       &email,
		DisplayName: "Sprint 8 Repo Test",
	}
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", user.Username) })

	created, err := repo.Create(ctx, user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Error("expected a generated, non-zero ID")
	}
	if created.Rating != 1500 {
		t.Errorf("expected default rating 1500, got %d", created.Rating)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("expected created_at/updated_at to be set by the database")
	}
}

func TestUserRepository_Create_DuplicateUsername(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	username := "sprint8_repo_dup_user"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	if _, err := repo.Create(ctx, &domain.User{Username: username, DisplayName: "First"}); err != nil {
		t.Fatalf("unexpected error on first create: %v", err)
	}

	_, err := repo.Create(ctx, &domain.User{Username: username, DisplayName: "Second"})
	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
}

func TestUserRepository_Create_DuplicateEmail(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	email := "sprint8-repo-dup@example.com"
	t.Cleanup(func() {
		db.MustExec("DELETE FROM users WHERE username IN ('sprint8_repo_dup_a', 'sprint8_repo_dup_b')")
	})

	if _, err := repo.Create(ctx, &domain.User{Username: "sprint8_repo_dup_a", Email: &email, DisplayName: "First"}); err != nil {
		t.Fatalf("unexpected error on first create: %v", err)
	}

	_, err := repo.Create(ctx, &domain.User{Username: "sprint8_repo_dup_b", Email: &email, DisplayName: "Second"})
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestUserRepository_GetByUsername(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	username := "sprint9_repo_getbyusername"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	created, err := repo.Create(ctx, &domain.User{Username: username, DisplayName: "Get Test"})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	got, err := repo.GetByUsername(ctx, username)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, got.ID)
	}
}

func TestUserRepository_GetByUsername_NotFound(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)

	_, err := repo.GetByUsername(context.Background(), "no_such_user_at_all")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestUserRepository_GetByUsername_ExcludesSoftDeleted(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	username := "sprint9_repo_deleted_user"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	if _, err := repo.Create(ctx, &domain.User{Username: username, DisplayName: "Deleted Test"}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	db.MustExec("UPDATE users SET deleted_at = now() WHERE username = $1", username)

	_, err := repo.GetByUsername(ctx, username)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected a soft-deleted user to look like ErrUserNotFound, got %v", err)
	}
}

func TestUserRepository_GetByID(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	username := "sprint12_repo_getbyid"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	created, err := repo.Create(ctx, &domain.User{Username: username, DisplayName: "GetByID Test"})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Username != username {
		t.Errorf("expected username %q, got %q", username, got.Username)
	}
}

func TestUserRepository_GetByID_NotFound(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)

	_, err := repo.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestUserRepository_UpdatePassword(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	username := "sprint13_repo_updatepassword"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	created, err := repo.Create(ctx, &domain.User{Username: username, DisplayName: "UpdatePassword Test"})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	if err := repo.UpdatePassword(ctx, created.ID, "a-new-bcrypt-hash"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("unexpected error re-fetching: %v", err)
	}
	if got.PasswordHash == nil || *got.PasswordHash != "a-new-bcrypt-hash" {
		t.Errorf("expected password hash to be updated, got %v", got.PasswordHash)
	}
}

func TestUserRepository_UpdatePassword_NotFound(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)

	err := repo.UpdatePassword(context.Background(), uuid.New(), "irrelevant-hash")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestUserRepository_UpdatePassword_ExcludesSoftDeleted(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	username := "sprint13_repo_updatepassword_deleted"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", username) })

	created, err := repo.Create(ctx, &domain.User{Username: username, DisplayName: "Deleted Test"})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	db.MustExec("UPDATE users SET deleted_at = now() WHERE id = $1", created.ID)

	err = repo.UpdatePassword(ctx, created.ID, "irrelevant-hash")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected a soft-deleted user's password update to look like ErrUserNotFound, got %v", err)
	}
}

// createTestActor is a stand-in for "some admin" in Deactivate tests
// — a real row is required since audit_log.actor_id has a foreign
// key to users. That FK has no ON DELETE CASCADE (deliberately —
// audit history must survive even if the actor's row is later hard-
// deleted, which the application itself never actually does; only
// this test's own teardown does a real DELETE), so cleanup has to
// clear this actor's audit_log rows first or the user DELETE itself
// would violate the same constraint it's demonstrating.
func createTestActor(t *testing.T, db *sqlx.DB, repo *UserRepository, ctx context.Context, username string) uuid.UUID {
	t.Helper()
	actor, err := repo.Create(ctx, &domain.User{Username: username, DisplayName: "Actor"})
	if err != nil {
		t.Fatalf("setup: unexpected error creating actor: %v", err)
	}
	t.Cleanup(func() {
		db.MustExec("DELETE FROM audit_log WHERE actor_id = $1", actor.ID)
		db.MustExec("DELETE FROM users WHERE username = $1", username)
	})
	return actor.ID
}

func TestUserRepository_Deactivate(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	targetUsername := "sprint14_repo_deactivate_target"
	actorUsername := "sprint14_repo_deactivate_actor"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", targetUsername) })
	actorID := createTestActor(t, db, repo, ctx, actorUsername)

	target, err := repo.Create(ctx, &domain.User{Username: targetUsername, DisplayName: "Target"})
	if err != nil {
		t.Fatalf("unexpected error creating target: %v", err)
	}

	if err := repo.Deactivate(ctx, target.ID, actorID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The user must now look exactly like a nonexistent one to every
	// normal lookup.
	if _, err := repo.GetByID(ctx, target.ID); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("expected the deactivated user to look not-found, got %v", err)
	}

	var count int
	if err := db.Get(&count, `
		SELECT count(*) FROM audit_log
		WHERE actor_id = $1 AND action = $2 AND target_type = $3 AND target_id = $4
	`, actorID, domain.ActionUserDeactivate, domain.AuditTargetUser, target.ID); err != nil {
		t.Fatalf("unexpected error querying audit_log: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly one audit_log entry for this deactivation, got %d", count)
	}
}

func TestUserRepository_Deactivate_NotFound(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	actorID := createTestActor(t, db, repo, ctx, "sprint14_repo_deactivate_notfound_actor")

	err := repo.Deactivate(ctx, uuid.New(), actorID)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestUserRepository_Deactivate_AlreadyDeactivated(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	targetUsername := "sprint14_repo_deactivate_twice"
	t.Cleanup(func() { db.MustExec("DELETE FROM users WHERE username = $1", targetUsername) })
	actorID := createTestActor(t, db, repo, ctx, "sprint14_repo_deactivate_twice_actor")

	target, err := repo.Create(ctx, &domain.User{Username: targetUsername, DisplayName: "Target"})
	if err != nil {
		t.Fatalf("unexpected error creating target: %v", err)
	}

	if err := repo.Deactivate(ctx, target.ID, actorID); err != nil {
		t.Fatalf("unexpected error on first deactivate: %v", err)
	}

	err = repo.Deactivate(ctx, target.ID, actorID)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected deactivating an already-deactivated user to look not-found, got %v", err)
	}
}

func TestUserRepository_Deactivate_NoAuditRowOnFailure(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	actorID := createTestActor(t, db, repo, ctx, "sprint14_repo_deactivate_rollback_actor")

	// Deactivating a nonexistent user fails — this test proves the
	// transaction actually rolls back, not just that the call returns
	// an error: no audit row should exist for it either.
	if err := repo.Deactivate(ctx, uuid.New(), actorID); err == nil {
		t.Fatal("setup: expected deactivating a nonexistent user to fail")
	}

	var count int
	if err := db.Get(&count, `SELECT count(*) FROM audit_log WHERE actor_id = $1`, actorID); err != nil {
		t.Fatalf("unexpected error querying audit_log: %v", err)
	}
	if count != 0 {
		t.Errorf("expected no audit_log entry for a failed deactivation, got %d", count)
	}
}

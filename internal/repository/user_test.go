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

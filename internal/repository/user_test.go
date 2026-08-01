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

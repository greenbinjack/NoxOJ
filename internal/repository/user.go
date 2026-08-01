// Package repository holds NoxOJ's database access layer — real SQL,
// written by hand (see ARCHITECTURE.md §5.1's sqlx-over-GORM decision).
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"noxoj/internal/domain"
)

var (
	ErrUsernameTaken = errors.New("username already taken")
	ErrEmailTaken    = errors.New("email already taken")
)

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

const insertUserQuery = `
	INSERT INTO users (username, email, password_hash, display_name)
	VALUES (:username, :email, :password_hash, :display_name)
	RETURNING id, username, email, password_hash, display_name, rating,
	          created_at, updated_at, is_offline_local, deleted_at
`

// Create inserts a new user and returns the row as the database
// actually stored it (including the generated id, default rating,
// and timestamps) — not just an echo of what was passed in.
//
// Deliberately does NOT check "does this username/email already
// exist" as a separate query first: that check-then-insert pattern
// has a real race under concurrent registrations — two requests could
// both pass the check before either one's insert commits. The
// database's own UNIQUE constraints are the actual source of truth;
// this just translates the resulting error into something the caller
// can act on.
func (r *UserRepository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	rows, err := r.db.NamedQueryContext(ctx, insertUserQuery, user)
	if err != nil {
		return nil, translateUniqueViolation(err)
	}
	defer rows.Close()

	var created domain.User
	if rows.Next() {
		if err := rows.StructScan(&created); err != nil {
			return nil, fmt.Errorf("scanning created user: %w", err)
		}
	}
	return &created, nil
}

// translateUniqueViolation maps Postgres's generic "unique_violation"
// (SQLSTATE 23505) into a specific, callable sentinel error based on
// which constraint actually fired — "users_username_key" and
// "users_email_key" are Postgres's default auto-generated names for
// the UNIQUE constraints on those columns (table_column_key).
func translateUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "users_username_key":
			return ErrUsernameTaken
		case "users_email_key":
			return ErrEmailTaken
		}
	}
	return fmt.Errorf("creating user: %w", err)
}

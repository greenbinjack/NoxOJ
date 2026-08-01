// Package repository holds NoxOJ's database access layer — real SQL,
// written by hand (see ARCHITECTURE.md §5.1's sqlx-over-GORM decision).
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"noxoj/internal/domain"
)

var (
	ErrUsernameTaken = errors.New("username already taken")
	ErrEmailTaken    = errors.New("email already taken")
	ErrUserNotFound  = errors.New("user not found")
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

// CreateWithRole inserts a new user and grants them defaultRole in a
// single transaction — if role assignment fails (e.g. defaultRole is
// misspelled and doesn't exist), the user insert rolls back too, so
// registration can never leave a roleless user behind. Uses the same
// INSERT...SELECT pattern as RoleRepository.AssignRole; duplicated
// rather than shared across repositories because sharing it would
// mean introducing a transaction-spanning-repositories abstraction
// this codebase doesn't otherwise need yet.
func (r *UserRepository) CreateWithRole(ctx context.Context, user *domain.User, defaultRole string) (*domain.User, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	rows, err := tx.NamedQuery(insertUserQuery, user)
	if err != nil {
		return nil, translateUniqueViolation(err)
	}
	var created domain.User
	if rows.Next() {
		if err := rows.StructScan(&created); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning created user: %w", err)
		}
	}
	rows.Close()

	res, err := tx.Exec(assignRoleQuery, created.ID, defaultRole)
	if err != nil {
		return nil, fmt.Errorf("assigning default role: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("assigning default role: role %q does not exist", defaultRole)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return &created, nil
}

const selectByUsernameQuery = `
	SELECT id, username, email, password_hash, display_name, rating,
	       created_at, updated_at, is_offline_local, deleted_at
	FROM users
	WHERE username = $1 AND deleted_at IS NULL
`

// GetByUsername looks up a non-deleted user by username. A
// deactivated account (deleted_at set) is treated identically to a
// nonexistent one — both return ErrUserNotFound — so a soft-deleted
// account can't still log in.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user, selectByUsernameQuery, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("looking up user by username: %w", err)
	}
	return &user, nil
}

const selectByIDQuery = `
	SELECT id, username, email, password_hash, display_name, rating,
	       created_at, updated_at, is_offline_local, deleted_at
	FROM users
	WHERE id = $1 AND deleted_at IS NULL
`

// GetByID looks up a non-deleted user by id — the read behind
// GET /users/me: the authenticated user's ID comes from their
// already-verified token (see internal/middleware), this just fetches
// the current row for it. Same soft-delete exclusion as GetByUsername.
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user, selectByIDQuery, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("looking up user by id: %w", err)
	}
	return &user, nil
}

const updatePasswordQuery = `
	UPDATE users SET password_hash = $1, updated_at = now()
	WHERE id = $2 AND deleted_at IS NULL
`

// UpdatePassword sets a new bcrypt hash for id — used by the
// password-reset flow (Sprint 13). Scoped to non-deleted accounts,
// same as every other by-id lookup; a soft-deleted account can't have
// its password "reset" back into a usable one.
func (r *UserRepository) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	res, err := r.db.ExecContext(ctx, updatePasswordQuery, passwordHash, id)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

const deactivateUserQuery = `
	UPDATE users SET deleted_at = now()
	WHERE id = $1 AND deleted_at IS NULL
`

const insertAuditLogQuery = `
	INSERT INTO audit_log (actor_id, action, target_type, target_id)
	VALUES ($1, $2, $3, $4)
`

// Deactivate soft-deletes id (sets deleted_at) and records an audit
// log entry crediting actorID, in one transaction — same reasoning as
// CreateWithRole (Sprint 7): an action and its audit trail must
// succeed or fail together, since a deactivation with no audit trail
// (or an audit trail for a deactivation that didn't actually persist)
// both defeat the point of keeping one at all.
//
// This is deliberately not generalized into a shared
// "repository-spanning transaction" helper — CreateWithRole made the
// same call for the same reason: that abstraction is only worth
// building once a second, unrelated caller actually needs it too.
//
// Returns ErrUserNotFound if id doesn't exist OR is already
// deactivated — the same "soft-deleted looks not-found" treatment
// GetByUsername/GetByID already use, so deactivating a
// already-deactivated account isn't a special case callers need to
// think about separately.
func (r *UserRepository) Deactivate(ctx context.Context, id uuid.UUID, actorID uuid.UUID) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	res, err := tx.ExecContext(ctx, deactivateUserQuery, id)
	if err != nil {
		return fmt.Errorf("deactivating user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return ErrUserNotFound
	}

	if _, err := tx.ExecContext(ctx, insertAuditLogQuery, actorID, domain.ActionUserDeactivate, domain.AuditTargetUser, id); err != nil {
		return fmt.Errorf("recording audit log entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
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

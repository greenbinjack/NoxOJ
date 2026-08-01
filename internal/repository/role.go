package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

var ErrRoleNotFound = errors.New("role not found")

type RoleRepository struct {
	db *sqlx.DB
}

func NewRoleRepository(db *sqlx.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

const assignRoleQuery = `
	INSERT INTO user_roles (user_id, role_id)
	SELECT $1, id FROM roles WHERE name = $2
	ON CONFLICT (user_id, role_id) DO NOTHING
`

// AssignRole grants roleName to userID. Idempotent — assigning a role
// the user already holds succeeds silently rather than erroring.
//
// The INSERT...SELECT affects zero rows both when the user already
// has the role AND when roleName doesn't exist at all — RowsAffected
// alone can't tell those apart, so this checks the role exists first
// to give a clear, distinct error for a typo'd role name instead of
// a silent no-op that looks like success.
func (r *RoleRepository) AssignRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	var exists bool
	if err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM roles WHERE name = $1)`, roleName); err != nil {
		return fmt.Errorf("checking role exists: %w", err)
	}
	if !exists {
		return ErrRoleNotFound
	}

	if _, err := r.db.ExecContext(ctx, assignRoleQuery, userID, roleName); err != nil {
		return fmt.Errorf("assigning role: %w", err)
	}
	return nil
}

const getRoleNamesQuery = `
	SELECT r.name FROM user_roles ur
	JOIN roles r ON r.id = ur.role_id
	WHERE ur.user_id = $1
	ORDER BY r.name
`

// GetRoleNames returns the names of every role userID currently
// holds — this is what gets embedded in a freshly-issued access
// token's claims (see internal/auth.GenerateAccessToken).
func (r *RoleRepository) GetRoleNames(ctx context.Context, userID uuid.UUID) ([]string, error) {
	names := []string{}
	if err := r.db.SelectContext(ctx, &names, getRoleNamesQuery, userID); err != nil {
		return nil, fmt.Errorf("getting role names: %w", err)
	}
	return names, nil
}

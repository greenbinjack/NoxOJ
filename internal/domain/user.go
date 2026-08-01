// Package domain holds NoxOJ's core data types — the Go-side mirror
// of the database schema, tagged for sqlx to scan query results into.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// User mirrors the users table (see db/migrations and ARCHITECTURE.md §4.2).
// Nullable columns are pointers, not sql.NullString/sql.NullTime — nil
// means "not set," no separate .Valid field to check.
type User struct {
	ID             uuid.UUID  `db:"id"`
	Username       string     `db:"username"`
	Email          *string    `db:"email"`
	PasswordHash   *string    `db:"password_hash"`
	DisplayName    string     `db:"display_name"`
	Rating         int        `db:"rating"`
	CreatedAt      time.Time  `db:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at"`
	IsOfflineLocal bool       `db:"is_offline_local"`
	DeletedAt      *time.Time `db:"deleted_at"`
}

// Role mirrors the roles table — a fixed, platform-defined set,
// seeded by the migration itself rather than created by users.
type Role struct {
	ID   int16  `db:"id"`
	Name string `db:"name"`
}

// UserRole mirrors the user_roles join table linking users to roles
// — the many-to-many relationship between them.
type UserRole struct {
	UserID uuid.UUID `db:"user_id"`
	RoleID int16     `db:"role_id"`
}

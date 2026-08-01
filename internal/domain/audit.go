package domain

import (
	"time"

	"github.com/google/uuid"
)

// AuditLogEntry mirrors the audit_log table — an append-only record
// of an admin-driven action. target_type/target_id are deliberately
// generic (not a users-only foreign key) so future actions on other
// resources (problems, contests, ...) can write into the same table.
type AuditLogEntry struct {
	ID         int64      `db:"id"`
	ActorID    uuid.UUID  `db:"actor_id"`
	Action     string     `db:"action"`
	TargetType string     `db:"target_type"`
	TargetID   *uuid.UUID `db:"target_id"`
	CreatedAt  time.Time  `db:"created_at"`
}

// Audit action names — constants for the same reason role names are
// (Sprint 11): a typo fails to compile instead of silently writing an
// unrecognizable string into a permanent record.
const (
	ActionUserDeactivate = "user.deactivate"
)

// AuditTargetUser is the target_type recorded for actions against a
// user account.
const AuditTargetUser = "user"

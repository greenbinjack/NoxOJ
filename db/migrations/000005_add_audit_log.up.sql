-- Append-only administrative audit trail — starts with user
-- deactivation (Sprint 14), deliberately generic (target_type +
-- target_id rather than a users-only foreign key) so future
-- admin-driven actions on other resources (problems, contests, ...)
-- can write into the same table without another migration.
CREATE TABLE audit_log (
    id           BIGSERIAL PRIMARY KEY,
    actor_id     UUID NOT NULL REFERENCES users(id),
    action       TEXT NOT NULL,
    target_type  TEXT NOT NULL,
    target_id    UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- "What happened to this thing" — the lookup an admin panel or an
-- incident investigation actually needs.
CREATE INDEX idx_audit_log_target ON audit_log (target_type, target_id);

-- "What has this admin done" — the other direction of the same
-- question.
CREATE INDEX idx_audit_log_actor ON audit_log (actor_id);

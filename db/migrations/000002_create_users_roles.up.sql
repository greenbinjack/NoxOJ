-- Case-insensitive text type — usernames/emails shouldn't be treated
-- as distinct just because of letter casing.
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username          CITEXT UNIQUE NOT NULL,
    email             CITEXT UNIQUE,                  -- nullable: offline-mode local accounts may skip email
    password_hash     TEXT,                            -- nullable if OIDC-only account
    display_name      TEXT NOT NULL,
    rating            INTEGER NOT NULL DEFAULT 1500,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    is_offline_local  BOOLEAN NOT NULL DEFAULT false,
    deleted_at        TIMESTAMPTZ
);

CREATE TABLE roles (
    id    SMALLSERIAL PRIMARY KEY,
    name  TEXT UNIQUE NOT NULL
);

CREATE TABLE user_roles (
    user_id  UUID REFERENCES users(id) ON DELETE CASCADE,
    role_id  SMALLINT REFERENCES roles(id),
    PRIMARY KEY (user_id, role_id)
);

-- Roles are a fixed set defined by the platform, not user-created —
-- seeding them here makes them part of the schema's complete state,
-- not something application code has to remember to insert.
INSERT INTO roles (name) VALUES
    ('admin'),
    ('problem_setter'),
    ('contestant'),
    ('judge_operator');

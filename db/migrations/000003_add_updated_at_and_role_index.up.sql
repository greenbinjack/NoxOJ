-- Reusable trigger function: any table with an updated_at column can
-- attach this instead of trusting every future UPDATE statement to
-- remember to set it by hand.
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

ALTER TABLE users ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- user_roles' primary key (user_id, role_id) only serves lookups
-- starting from user_id. "Which users have role X" (an admin panel
-- will need exactly this) had no supporting index at all.
CREATE INDEX idx_user_roles_role_id ON user_roles (role_id);

DROP INDEX IF EXISTS idx_user_roles_role_id;
DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
ALTER TABLE users DROP COLUMN updated_at;
DROP FUNCTION IF EXISTS set_updated_at();

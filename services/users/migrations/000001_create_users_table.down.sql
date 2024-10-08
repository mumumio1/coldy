DROP TRIGGER IF EXISTS update_addresses_updated_at ON addresses;
DROP TRIGGER IF EXISTS update_users_updated_at ON users;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP INDEX IF EXISTS idx_addresses_user_id;
DROP INDEX IF EXISTS idx_users_email;
DROP TABLE IF EXISTS addresses;
DROP TABLE IF EXISTS users;


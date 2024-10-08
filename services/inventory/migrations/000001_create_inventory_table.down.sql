DROP FUNCTION IF EXISTS cleanup_expired_reservations();
DROP TRIGGER IF EXISTS update_reservations_updated_at ON reservations;
DROP TRIGGER IF EXISTS update_inventory_updated_at ON inventory;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP INDEX IF EXISTS idx_reservations_expires_at;
DROP INDEX IF EXISTS idx_reservations_status;
DROP INDEX IF EXISTS idx_reservations_product_id;
DROP INDEX IF EXISTS idx_reservations_reservation_id;
DROP TABLE IF EXISTS reservations;
DROP TABLE IF EXISTS inventory;


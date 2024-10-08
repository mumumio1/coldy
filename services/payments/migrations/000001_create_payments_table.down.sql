DROP TRIGGER IF EXISTS update_payments_updated_at ON payments;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP INDEX IF EXISTS idx_payment_outbox_published;
DROP INDEX IF EXISTS idx_payments_status;
DROP INDEX IF EXISTS idx_payments_user_id;
DROP INDEX IF EXISTS idx_payments_order_id;
DROP TABLE IF EXISTS payment_outbox;
DROP TABLE IF EXISTS payments;
DROP TYPE IF EXISTS payment_method;
DROP TYPE IF EXISTS payment_status;


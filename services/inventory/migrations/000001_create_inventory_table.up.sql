CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS inventory (
    product_id UUID PRIMARY KEY,
    available_quantity INTEGER NOT NULL DEFAULT 0 CHECK (available_quantity >= 0),
    reserved_quantity INTEGER NOT NULL DEFAULT 0 CHECK (reserved_quantity >= 0),
    total_quantity INTEGER NOT NULL DEFAULT 0,
    version INTEGER NOT NULL DEFAULT 1, -- For optimistic locking
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS reservations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    reservation_id VARCHAR(255) UNIQUE NOT NULL, -- Order ID or unique identifier
    product_id UUID NOT NULL REFERENCES inventory(product_id),
    quantity INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- active, committed, released
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_reservations_reservation_id ON reservations(reservation_id);
CREATE INDEX idx_reservations_product_id ON reservations(product_id);
CREATE INDEX idx_reservations_status ON reservations(status);
CREATE INDEX idx_reservations_expires_at ON reservations(expires_at) WHERE status = 'active';

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_inventory_updated_at BEFORE UPDATE ON inventory
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_reservations_updated_at BEFORE UPDATE ON reservations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to clean up expired reservations
CREATE OR REPLACE FUNCTION cleanup_expired_reservations()
RETURNS void AS $$
DECLARE
    expired_reservation RECORD;
BEGIN
    FOR expired_reservation IN
        SELECT reservation_id, product_id, quantity
        FROM reservations
        WHERE status = 'active'
          AND expires_at < CURRENT_TIMESTAMP
    LOOP
        -- Release the expired reservation
        UPDATE inventory
        SET available_quantity = available_quantity + expired_reservation.quantity,
            reserved_quantity = reserved_quantity - expired_reservation.quantity,
            version = version + 1
        WHERE product_id = expired_reservation.product_id;

        -- Mark reservation as released
        UPDATE reservations
        SET status = 'released'
        WHERE reservation_id = expired_reservation.reservation_id;
    END LOOP;
END;
$$ language 'plpgsql';


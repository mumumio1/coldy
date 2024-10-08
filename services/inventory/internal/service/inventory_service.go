package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// InventoryService handles inventory business logic
type InventoryService struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewInventoryService creates a new inventory service
func NewInventoryService(db *sql.DB, logger *zap.Logger) *InventoryService {
	return &InventoryService{
		db:     db,
		logger: logger,
	}
}

// Inventory represents inventory data
type Inventory struct {
	ProductID         string
	AvailableQuantity int32
	ReservedQuantity  int32
	TotalQuantity     int32
	Version           int32
	UpdatedAt         time.Time
}

// ReservationItem represents an item to reserve
type ReservationItem struct {
	ProductID string
	Quantity  int32
}

// ReserveStock reserves stock for an order with optimistic locking
func (s *InventoryService) ReserveStock(ctx context.Context, reservationID string, items []ReservationItem, ttlSeconds int32) error {
	if ttlSeconds <= 0 {
		ttlSeconds = 900 // Default 15 minutes
	}

	expiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second)

	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Reserve each item with optimistic locking
	for _, item := range items {
		// Get current inventory with version (optimistic lock)
		var inventory Inventory
		query := `
			SELECT product_id, available_quantity, reserved_quantity, total_quantity, version, updated_at
			FROM inventory
			WHERE product_id = $1
			FOR UPDATE
		`

		err := tx.QueryRowContext(ctx, query, item.ProductID).Scan(
			&inventory.ProductID,
			&inventory.AvailableQuantity,
			&inventory.ReservedQuantity,
			&inventory.TotalQuantity,
			&inventory.Version,
			&inventory.UpdatedAt,
		)

		if err == sql.ErrNoRows {
			return fmt.Errorf("product %s not found in inventory", item.ProductID)
		}
		if err != nil {
			return fmt.Errorf("failed to get inventory: %w", err)
		}

		// Check if enough stock available
		if inventory.AvailableQuantity < item.Quantity {
			return fmt.Errorf("insufficient stock for product %s: available=%d, requested=%d",
				item.ProductID, inventory.AvailableQuantity, item.Quantity)
		}

		// Update inventory with optimistic locking (version check)
		updateQuery := `
			UPDATE inventory
			SET available_quantity = available_quantity - $1,
			    reserved_quantity = reserved_quantity + $1,
			    version = version + 1,
			    updated_at = CURRENT_TIMESTAMP
			WHERE product_id = $2 AND version = $3
		`

		result, err := tx.ExecContext(ctx, updateQuery, item.Quantity, item.ProductID, inventory.Version)
		if err != nil {
			return fmt.Errorf("failed to update inventory: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}

		// If no rows affected, version mismatch (concurrent update)
		if rowsAffected == 0 {
			return fmt.Errorf("inventory conflict for product %s (concurrent update)", item.ProductID)
		}

		// Create reservation record
		reservationQuery := `
			INSERT INTO reservations (id, reservation_id, product_id, quantity, status, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`

		_, err = tx.ExecContext(ctx, reservationQuery,
			uuid.New().String(),
			reservationID,
			item.ProductID,
			item.Quantity,
			"active",
			expiresAt,
		)

		if err != nil {
			return fmt.Errorf("failed to create reservation: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Info("stock reserved",
		zap.String("reservation_id", reservationID),
		zap.Int("items_count", len(items)),
	)

	return nil
}

// ReleaseStock releases a reservation
func (s *InventoryService) ReleaseStock(ctx context.Context, reservationID string) error {
	return s.updateReservationStatus(ctx, reservationID, "released", func(item ReservationItem) (string, []interface{}) {
		query := `
			UPDATE inventory
			SET available_quantity = available_quantity + $1,
			    reserved_quantity = reserved_quantity - $1,
			    version = version + 1
			WHERE product_id = $2
		`
		return query, []interface{}{item.Quantity, item.ProductID}
	})
}

// CommitStock commits a reservation (converts reserved to sold)
func (s *InventoryService) CommitStock(ctx context.Context, reservationID string) error {
	return s.updateReservationStatus(ctx, reservationID, "committed", func(item ReservationItem) (string, []interface{}) {
		query := `
			UPDATE inventory
			SET reserved_quantity = reserved_quantity - $1,
			    total_quantity = total_quantity - $1,
			    version = version + 1
			WHERE product_id = $2
		`
		return query, []interface{}{item.Quantity, item.ProductID}
	})
}

func (s *InventoryService) updateReservationStatus(
	ctx context.Context,
	reservationID string,
	newStatus string,
	updateFn func(ReservationItem) (string, []interface{}),
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := `
		SELECT product_id, quantity
		FROM reservations
		WHERE reservation_id = $1 AND status = 'active'
	`

	rows, err := tx.QueryContext(ctx, query, reservationID)
	if err != nil {
		return fmt.Errorf("failed to query reservations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []ReservationItem
	for rows.Next() {
		var item ReservationItem
		if err := rows.Scan(&item.ProductID, &item.Quantity); err != nil {
			return fmt.Errorf("failed to scan reservation: %w", err)
		}
		items = append(items, item)
	}
	_ = rows.Close()

	if len(items) == 0 {
		return fmt.Errorf("no active reservations found for %s", reservationID)
	}

	for _, item := range items {
		updateQuery, args := updateFn(item)
		if _, err := tx.ExecContext(ctx, updateQuery, args...); err != nil {
			return fmt.Errorf("failed to update inventory: %w", err)
		}
	}

	statusQuery := `
		UPDATE reservations
		SET status = $1
		WHERE reservation_id = $2 AND status = 'active'
	`

	if _, err = tx.ExecContext(ctx, statusQuery, newStatus, reservationID); err != nil {
		return fmt.Errorf("failed to update reservations: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Info("reservation updated",
		zap.String("reservation_id", reservationID),
		zap.String("status", newStatus),
	)
	return nil
}

// GetInventory retrieves inventory for a product
func (s *InventoryService) GetInventory(ctx context.Context, productID string) (*Inventory, error) {
	query := `
		SELECT product_id, available_quantity, reserved_quantity, total_quantity, version, updated_at
		FROM inventory
		WHERE product_id = $1
	`

	var inventory Inventory
	err := s.db.QueryRowContext(ctx, query, productID).Scan(
		&inventory.ProductID,
		&inventory.AvailableQuantity,
		&inventory.ReservedQuantity,
		&inventory.TotalQuantity,
		&inventory.Version,
		&inventory.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("inventory not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory: %w", err)
	}

	return &inventory, nil
}

// AdjustInventory adjusts inventory (for restocking, damage, etc.)
func (s *InventoryService) AdjustInventory(ctx context.Context, productID string, delta int32, reason string) (*Inventory, error) {
	query := `
		INSERT INTO inventory (product_id, available_quantity, total_quantity)
		VALUES ($1, $2, $2)
		ON CONFLICT (product_id) DO UPDATE
		SET available_quantity = inventory.available_quantity + $2,
		    total_quantity = inventory.total_quantity + $2,
		    version = inventory.version + 1
		RETURNING product_id, available_quantity, reserved_quantity, total_quantity, version, updated_at
	`

	var inventory Inventory
	err := s.db.QueryRowContext(ctx, query, productID, delta).Scan(
		&inventory.ProductID,
		&inventory.AvailableQuantity,
		&inventory.ReservedQuantity,
		&inventory.TotalQuantity,
		&inventory.Version,
		&inventory.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to adjust inventory: %w", err)
	}

	s.logger.Info("inventory adjusted",
		zap.String("product_id", productID),
		zap.Int32("delta", delta),
		zap.String("reason", reason),
	)

	return &inventory, nil
}

// CleanupExpiredReservations cleans up expired reservations
func (s *InventoryService) CleanupExpiredReservations(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "SELECT cleanup_expired_reservations()")
	if err != nil {
		return fmt.Errorf("failed to cleanup expired reservations: %w", err)
	}

	s.logger.Info("expired reservations cleaned up")
	return nil
}

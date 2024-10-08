package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OrderStatus represents the order status
type OrderStatus string

const (
	StatusPending    OrderStatus = "pending"
	StatusConfirmed  OrderStatus = "confirmed"
	StatusPaid       OrderStatus = "paid"
	StatusProcessing OrderStatus = "processing"
	StatusShipped    OrderStatus = "shipped"
	StatusDelivered  OrderStatus = "delivered"
	StatusCancelled  OrderStatus = "canceled"
	StatusRefunded   OrderStatus = "refunded"
)

// Order represents an order entity
type Order struct {
	ID                 string
	UserID             string
	TotalCurrency      string
	TotalAmount        int64
	Status             OrderStatus
	PaymentID          string
	ShippingStreet     string
	ShippingCity       string
	ShippingState      string
	ShippingPostalCode string
	ShippingCountry    string
	Items              []OrderItem
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// OrderItem represents an order item
type OrderItem struct {
	ID                 string
	OrderID            string
	ProductID          string
	ProductName        string
	Quantity           int32
	UnitPriceCurrency  string
	UnitPriceAmount    int64
	TotalPriceCurrency string
	TotalPriceAmount   int64
	CreatedAt          time.Time
}

// OutboxEvent represents an outbox event
type OutboxEvent struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       map[string]interface{}
	Published     bool
	PublishedAt   *time.Time
	CreatedAt     time.Time
}

// OrderRepository handles order data access
type OrderRepository struct {
	db *sql.DB
}

// NewOrderRepository creates a new order repository
func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// CreateWithOutbox creates an order and outbox event in a transaction
func (r *OrderRepository) CreateWithOutbox(ctx context.Context, order *Order, event *OutboxEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Insert order
	orderQuery := `
		INSERT INTO orders (id, user_id, total_currency, total_amount, status, shipping_street, shipping_city, shipping_state, shipping_postal_code, shipping_country)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at
	`

	order.ID = uuid.New().String()
	err = tx.QueryRowContext(ctx, orderQuery,
		order.ID,
		order.UserID,
		order.TotalCurrency,
		order.TotalAmount,
		order.Status,
		order.ShippingStreet,
		order.ShippingCity,
		order.ShippingState,
		order.ShippingPostalCode,
		order.ShippingCountry,
	).Scan(&order.CreatedAt, &order.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to insert order: %w", err)
	}

	// Insert order items
	itemQuery := `
		INSERT INTO order_items (id, order_id, product_id, product_name, quantity, unit_price_currency, unit_price_amount, total_price_currency, total_price_amount)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at
	`

	for i := range order.Items {
		item := &order.Items[i]
		item.ID = uuid.New().String()
		item.OrderID = order.ID

		err = tx.QueryRowContext(ctx, itemQuery,
			item.ID,
			item.OrderID,
			item.ProductID,
			item.ProductName,
			item.Quantity,
			item.UnitPriceCurrency,
			item.UnitPriceAmount,
			item.TotalPriceCurrency,
			item.TotalPriceAmount,
		).Scan(&item.CreatedAt)

		if err != nil {
			return fmt.Errorf("failed to insert order item: %w", err)
		}
	}

	// Insert outbox event
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	outboxQuery := `
		INSERT INTO outbox (id, aggregate_type, aggregate_id, event_type, payload)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`

	event.ID = uuid.New().String()
	event.AggregateID = order.ID

	err = tx.QueryRowContext(ctx, outboxQuery,
		event.ID,
		event.AggregateType,
		event.AggregateID,
		event.EventType,
		payloadJSON,
	).Scan(&event.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to insert outbox event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetByID retrieves an order by ID with items
func (r *OrderRepository) GetByID(ctx context.Context, id string) (*Order, error) {
	orderQuery := `
		SELECT id, user_id, total_currency, total_amount, status, payment_id, shipping_street, shipping_city, shipping_state, shipping_postal_code, shipping_country, created_at, updated_at
		FROM orders
		WHERE id = $1
	`

	var order Order
	var paymentID sql.NullString

	err := r.db.QueryRowContext(ctx, orderQuery, id).Scan(
		&order.ID,
		&order.UserID,
		&order.TotalCurrency,
		&order.TotalAmount,
		&order.Status,
		&paymentID,
		&order.ShippingStreet,
		&order.ShippingCity,
		&order.ShippingState,
		&order.ShippingPostalCode,
		&order.ShippingCountry,
		&order.CreatedAt,
		&order.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	if paymentID.Valid {
		order.PaymentID = paymentID.String
	}

	// Get order items
	itemsQuery := `
		SELECT id, order_id, product_id, product_name, quantity, unit_price_currency, unit_price_amount, total_price_currency, total_price_amount, created_at
		FROM order_items
		WHERE order_id = $1
		ORDER BY created_at
	`

	rows, err := r.db.QueryContext(ctx, itemsQuery, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get order items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var item OrderItem
		err := rows.Scan(
			&item.ID,
			&item.OrderID,
			&item.ProductID,
			&item.ProductName,
			&item.Quantity,
			&item.UnitPriceCurrency,
			&item.UnitPriceAmount,
			&item.TotalPriceCurrency,
			&item.TotalPriceAmount,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan order item: %w", err)
		}
		order.Items = append(order.Items, item)
	}

	return &order, nil
}

// UpdateStatus updates order status with outbox event
func (r *OrderRepository) UpdateStatus(ctx context.Context, orderID string, status OrderStatus, event *OutboxEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Update order status
	query := `
		UPDATE orders
		SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	result, err := tx.ExecContext(ctx, query, status, orderID)
	if err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("order not found")
	}

	// Insert outbox event if provided
	if event != nil {
		payloadJSON, err := json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal event payload: %w", err)
		}

		outboxQuery := `
			INSERT INTO outbox (id, aggregate_type, aggregate_id, event_type, payload)
			VALUES ($1, $2, $3, $4, $5)
		`

		event.ID = uuid.New().String()
		event.AggregateID = orderID

		_, err = tx.ExecContext(ctx, outboxQuery,
			event.ID,
			event.AggregateType,
			event.AggregateID,
			event.EventType,
			payloadJSON,
		)

		if err != nil {
			return fmt.Errorf("failed to insert outbox event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// List retrieves orders with pagination
func (r *OrderRepository) List(ctx context.Context, userID string, status OrderStatus, limit int, cursor string) ([]*Order, string, error) {
	query := `
		SELECT id, user_id, total_currency, total_amount, status, payment_id, shipping_street, shipping_city, shipping_state, shipping_postal_code, shipping_country, created_at, updated_at
		FROM orders
		WHERE user_id = $1
	`

	args := []interface{}{userID}
	argIdx := 2

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	if cursor != "" {
		query += fmt.Sprintf(" AND (created_at, id) < (SELECT created_at, id FROM orders WHERE id = $%d)", argIdx)
		args = append(args, cursor)
		argIdx++
	}

	query += " ORDER BY created_at DESC, id DESC"
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list orders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var orders []*Order
	for rows.Next() {
		var order Order
		var paymentID sql.NullString

		err := rows.Scan(
			&order.ID,
			&order.UserID,
			&order.TotalCurrency,
			&order.TotalAmount,
			&order.Status,
			&paymentID,
			&order.ShippingStreet,
			&order.ShippingCity,
			&order.ShippingState,
			&order.ShippingPostalCode,
			&order.ShippingCountry,
			&order.CreatedAt,
			&order.UpdatedAt,
		)
		if err != nil {
			return nil, "", fmt.Errorf("failed to scan order: %w", err)
		}

		if paymentID.Valid {
			order.PaymentID = paymentID.String
		}

		orders = append(orders, &order)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("rows error: %w", err)
	}

	// Determine next cursor
	var nextCursor string
	if len(orders) > limit {
		nextCursor = orders[limit-1].ID
		orders = orders[:limit]
	}

	return orders, nextCursor, nil
}

// GetUnpublishedEvents retrieves unpublished outbox events
func (r *OrderRepository) GetUnpublishedEvents(ctx context.Context, limit int) ([]*OutboxEvent, error) {
	query := `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, published, published_at, created_at
		FROM outbox
		WHERE published = false
		ORDER BY created_at
		LIMIT $1
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get unpublished events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []*OutboxEvent
	for rows.Next() {
		var event OutboxEvent
		var payloadJSON []byte
		var publishedAt sql.NullTime

		err := rows.Scan(
			&event.ID,
			&event.AggregateType,
			&event.AggregateID,
			&event.EventType,
			&payloadJSON,
			&event.Published,
			&publishedAt,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		if err := json.Unmarshal(payloadJSON, &event.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		if publishedAt.Valid {
			event.PublishedAt = &publishedAt.Time
		}

		events = append(events, &event)
	}

	return events, nil
}

// MarkEventPublished marks an outbox event as published
func (r *OrderRepository) MarkEventPublished(ctx context.Context, eventID string) error {
	query := `
		UPDATE outbox
		SET published = true, published_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark event published: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("event not found")
	}

	return nil
}

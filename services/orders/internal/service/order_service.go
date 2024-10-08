package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mumumio1/coldy/pkg/idempotency"
	"github.com/mumumio1/coldy/services/orders/internal/repository"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// OrderService handles order business logic
type OrderService struct {
	repo        *repository.OrderRepository
	idempotency *idempotency.Store
	logger      *zap.Logger
}

// NewOrderService creates a new order service
func NewOrderService(repo *repository.OrderRepository, redis *redis.Client, logger *zap.Logger) *OrderService {
	return &OrderService{
		repo:        repo,
		idempotency: idempotency.NewStore(redis),
		logger:      logger,
	}
}

// CreateOrderRequest represents a create order request
type CreateOrderRequest struct {
	UserID             string
	Items              []OrderItemRequest
	ShippingStreet     string
	ShippingCity       string
	ShippingState      string
	ShippingPostalCode string
	ShippingCountry    string
}

// OrderItemRequest represents an order item request
type OrderItemRequest struct {
	ProductID   string
	ProductName string
	Quantity    int32
	UnitPrice   Money
}

// Money represents a monetary amount
type Money struct {
	Currency string
	Amount   int64
}

// CreateOrder creates a new order with idempotency
func (s *OrderService) CreateOrder(ctx context.Context, idempotencyKey string, req *CreateOrderRequest) (*repository.Order, bool, error) {
	// Check idempotency
	key := idempotency.GenerateKey(req.UserID, "create_order", idempotencyKey)
	cached, found, err := s.idempotency.Get(ctx, key)
	if err != nil {
		s.logger.Warn("idempotency check failed", zap.Error(err))
	}
	if found {
		s.logger.Info("idempotent request, returning cached result",
			zap.String("user_id", req.UserID),
			zap.String("idempotency_key", idempotencyKey),
		)

		// Unmarshal cached order
		var order repository.Order
		if err := json.Unmarshal(cached.Body, &order); err != nil {
			return nil, false, fmt.Errorf("failed to unmarshal cached order: %w", err)
		}

		return &order, true, nil
	}

	// Calculate total
	var totalAmount int64
	currency := "USD"
	for _, item := range req.Items {
		totalAmount += item.UnitPrice.Amount * int64(item.Quantity)
		if item.UnitPrice.Currency != "" {
			currency = item.UnitPrice.Currency
		}
	}

	// Create order
	order := &repository.Order{
		UserID:             req.UserID,
		TotalCurrency:      currency,
		TotalAmount:        totalAmount,
		Status:             repository.StatusPending,
		ShippingStreet:     req.ShippingStreet,
		ShippingCity:       req.ShippingCity,
		ShippingState:      req.ShippingState,
		ShippingPostalCode: req.ShippingPostalCode,
		ShippingCountry:    req.ShippingCountry,
	}

	// Create order items
	for _, item := range req.Items {
		totalPrice := item.UnitPrice.Amount * int64(item.Quantity)
		order.Items = append(order.Items, repository.OrderItem{
			ProductID:          item.ProductID,
			ProductName:        item.ProductName,
			Quantity:           item.Quantity,
			UnitPriceCurrency:  item.UnitPrice.Currency,
			UnitPriceAmount:    item.UnitPrice.Amount,
			TotalPriceCurrency: item.UnitPrice.Currency,
			TotalPriceAmount:   totalPrice,
		})
	}

	// Create outbox event
	event := &repository.OutboxEvent{
		AggregateType: "order",
		EventType:     "order.created",
		Payload: map[string]interface{}{
			"order_id": order.ID,
			"user_id":  order.UserID,
			"total":    totalAmount,
			"currency": currency,
			"status":   string(order.Status),
			"items":    req.Items,
		},
	}

	// Create order with outbox event in transaction
	if err := s.repo.CreateWithOutbox(ctx, order, event); err != nil {
		return nil, false, fmt.Errorf("failed to create order: %w", err)
	}

	// Cache the result for idempotency
	orderJSON, _ := json.Marshal(order)
	if err := s.idempotency.Set(ctx, key, 200, orderJSON); err != nil {
		s.logger.Warn("failed to cache idempotency result", zap.Error(err))
	}

	s.logger.Info("order created",
		zap.String("order_id", order.ID),
		zap.String("user_id", order.UserID),
		zap.Int64("total", totalAmount),
	)

	return order, false, nil
}

// GetOrder retrieves an order by ID
func (s *OrderService) GetOrder(ctx context.Context, orderID string) (*repository.Order, error) {
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}
	if order == nil {
		return nil, fmt.Errorf("order not found")
	}
	return order, nil
}

// UpdateOrderStatus updates order status
func (s *OrderService) UpdateOrderStatus(ctx context.Context, orderID string, status repository.OrderStatus) error {
	// Create status change event
	event := &repository.OutboxEvent{
		AggregateType: "order",
		EventType:     fmt.Sprintf("order.%s", status),
		Payload: map[string]interface{}{
			"order_id": orderID,
			"status":   string(status),
		},
	}

	if err := s.repo.UpdateStatus(ctx, orderID, status, event); err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	s.logger.Info("order status updated",
		zap.String("order_id", orderID),
		zap.String("status", string(status)),
	)

	return nil
}

// CancelOrder cancels an order
func (s *OrderService) CancelOrder(ctx context.Context, orderID, reason string) error {
	// Get current order
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("failed to get order: %w", err)
	}
	if order == nil {
		return fmt.Errorf("order not found")
	}

	// Check if order can be canceled
	if order.Status == repository.StatusDelivered || order.Status == repository.StatusCancelled {
		return fmt.Errorf("order cannot be canceled in status: %s", order.Status)
	}

	// Create cancellation event
	event := &repository.OutboxEvent{
		AggregateType: "order",
		EventType:     "order.canceled",
		Payload: map[string]interface{}{
			"order_id": orderID,
			"reason":   reason,
		},
	}

	if err := s.repo.UpdateStatus(ctx, orderID, repository.StatusCancelled, event); err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	s.logger.Info("order canceled",
		zap.String("order_id", orderID),
		zap.String("reason", reason),
	)

	return nil
}

// ListOrders lists orders
func (s *OrderService) ListOrders(ctx context.Context, userID string, status repository.OrderStatus, limit int, cursor string) ([]*repository.Order, string, bool, error) {
	orders, nextCursor, err := s.repo.List(ctx, userID, status, limit, cursor)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to list orders: %w", err)
	}

	// Load items for each order
	for _, order := range orders {
		fullOrder, err := s.repo.GetByID(ctx, order.ID)
		if err != nil {
			s.logger.Warn("failed to load order items", zap.Error(err))
			continue
		}
		order.Items = fullOrder.Items
	}

	hasMore := nextCursor != ""
	return orders, nextCursor, hasMore, nil
}

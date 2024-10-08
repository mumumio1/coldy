package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mumumio1/coldy/pkg/circuitbreaker"
	"github.com/mumumio1/coldy/pkg/idempotency"
	"github.com/mumumio1/coldy/services/payments/internal/provider"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// PaymentService handles payment business logic
type PaymentService struct {
	db             *sql.DB
	provider       provider.PaymentProvider
	circuitBreaker *circuitbreaker.CircuitBreaker
	idempotency    *idempotency.Store
	logger         *zap.Logger
}

// NewPaymentService creates a new payment service
func NewPaymentService(
	db *sql.DB,
	provider provider.PaymentProvider,
	redis *redis.Client,
	logger *zap.Logger,
) *PaymentService {
	// Configure circuit breaker for payment provider
	cb := circuitbreaker.New(circuitbreaker.Config{
		MaxFailures:  5,
		Timeout:      10 * time.Second,
		ResetTimeout: 30 * time.Second,
	})

	// Log circuit breaker state changes
	cb.OnStateChange(func(from, to circuitbreaker.State) {
		logger.Warn("circuit breaker state changed",
			zap.String("from", stateString(from)),
			zap.String("to", stateString(to)),
		)
	})

	return &PaymentService{
		db:             db,
		provider:       provider,
		circuitBreaker: cb,
		idempotency:    idempotency.NewStore(redis),
		logger:         logger,
	}
}

// CreatePaymentRequest represents a payment creation request
type CreatePaymentRequest struct {
	OrderID       string
	UserID        string
	Amount        int64
	Currency      string
	PaymentMethod string
	CardNumber    string
	CVV           string
}

// Payment represents a payment
type Payment struct {
	ID                    string
	OrderID               string
	UserID                string
	AmountCurrency        string
	AmountValue           int64
	Status                string
	Method                string
	ProviderTransactionID string
	ErrorMessage          string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// CreatePayment creates a new payment with idempotency
func (s *PaymentService) CreatePayment(ctx context.Context, idempotencyKey string, req *CreatePaymentRequest) (*Payment, bool, error) {
	// Check idempotency
	key := idempotency.GenerateKey(req.UserID, "create_payment", idempotencyKey)
	cached, found, err := s.idempotency.Get(ctx, key)
	if err != nil {
		s.logger.Warn("idempotency check failed", zap.Error(err))
	}
	if found {
		s.logger.Info("idempotent payment request",
			zap.String("user_id", req.UserID),
			zap.String("order_id", req.OrderID),
		)

		var payment Payment
		if err := json.Unmarshal(cached.Body, &payment); err != nil {
			return nil, false, fmt.Errorf("failed to unmarshal cached payment: %w", err)
		}

		return &payment, true, nil
	}

	// Create payment record
	payment := &Payment{
		ID:             uuid.New().String(),
		OrderID:        req.OrderID,
		UserID:         req.UserID,
		AmountCurrency: req.Currency,
		AmountValue:    req.Amount,
		Status:         "pending",
		Method:         req.PaymentMethod,
	}

	// Insert payment
	query := `
		INSERT INTO payments (id, order_id, user_id, amount_currency, amount_value, status, method)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`

	err = s.db.QueryRowContext(ctx, query,
		payment.ID,
		payment.OrderID,
		payment.UserID,
		payment.AmountCurrency,
		payment.AmountValue,
		payment.Status,
		payment.Method,
	).Scan(&payment.CreatedAt, &payment.UpdatedAt)

	if err != nil {
		return nil, false, fmt.Errorf("failed to create payment: %w", err)
	}

	// Cache result for idempotency
	paymentJSON, _ := json.Marshal(payment)
	if err := s.idempotency.Set(ctx, key, 200, paymentJSON); err != nil {
		s.logger.Warn("failed to cache idempotency result", zap.Error(err))
	}

	s.logger.Info("payment created",
		zap.String("payment_id", payment.ID),
		zap.String("order_id", payment.OrderID),
	)

	return payment, false, nil
}

// ConfirmPayment confirms a payment by processing with provider
func (s *PaymentService) ConfirmPayment(ctx context.Context, paymentID string) (*Payment, error) {
	// Get payment
	payment, err := s.GetPayment(ctx, paymentID)
	if err != nil {
		return nil, err
	}

	if payment.Status != "pending" {
		return payment, nil // Already processed
	}

	// Update status to processing
	if err := s.updatePaymentStatus(ctx, paymentID, "processing", ""); err != nil {
		return nil, err
	}

	// Process payment with circuit breaker
	var providerResp *provider.ProcessPaymentResponse
	err = s.circuitBreaker.Execute(ctx, func() error {
		var provErr error
		providerResp, provErr = s.provider.ProcessPayment(ctx, &provider.ProcessPaymentRequest{
			OrderID:       payment.OrderID,
			Amount:        payment.AmountValue,
			Currency:      payment.AmountCurrency,
			PaymentMethod: payment.Method,
		})
		return provErr
	})

	if err != nil {
		// Payment failed
		s.logger.Error("payment processing failed",
			zap.String("payment_id", paymentID),
			zap.Error(err),
		)

		if err := s.updatePaymentStatusWithError(ctx, paymentID, "failed", err.Error()); err != nil {
			s.logger.Error("failed to update payment status", zap.Error(err))
		}

		// Publish failure event
		s.publishEvent(ctx, paymentID, "payment.failed", map[string]interface{}{
			"payment_id": paymentID,
			"order_id":   payment.OrderID,
			"error":      err.Error(),
		})

		return nil, fmt.Errorf("payment processing failed: %w", err)
	}

	// Payment succeeded
	if err := s.updatePaymentStatusWithTransaction(ctx, paymentID, "succeeded", providerResp.TransactionID); err != nil {
		return nil, err
	}

	// Publish success event
	s.publishEvent(ctx, paymentID, "payment.succeeded", map[string]interface{}{
		"payment_id":     paymentID,
		"order_id":       payment.OrderID,
		"transaction_id": providerResp.TransactionID,
	})

	s.logger.Info("payment confirmed",
		zap.String("payment_id", paymentID),
		zap.String("transaction_id", providerResp.TransactionID),
	)

	return s.GetPayment(ctx, paymentID)
}

// GetPayment retrieves a payment by ID
func (s *PaymentService) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	query := `
		SELECT id, order_id, user_id, amount_currency, amount_value, status, method, 
		       provider_transaction_id, error_message, created_at, updated_at
		FROM payments
		WHERE id = $1
	`

	var payment Payment
	var transactionID, errorMsg sql.NullString

	err := s.db.QueryRowContext(ctx, query, paymentID).Scan(
		&payment.ID,
		&payment.OrderID,
		&payment.UserID,
		&payment.AmountCurrency,
		&payment.AmountValue,
		&payment.Status,
		&payment.Method,
		&transactionID,
		&errorMsg,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("payment not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	if transactionID.Valid {
		payment.ProviderTransactionID = transactionID.String
	}
	if errorMsg.Valid {
		payment.ErrorMessage = errorMsg.String
	}

	return &payment, nil
}

func (s *PaymentService) updatePaymentStatus(ctx context.Context, paymentID, status, errorMsg string) error {
	query := `
		UPDATE payments
		SET status = $1, error_message = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`

	_, err := s.db.ExecContext(ctx, query, status, errorMsg, paymentID)
	return err
}

func (s *PaymentService) updatePaymentStatusWithError(ctx context.Context, paymentID, status, errorMsg string) error {
	return s.updatePaymentStatus(ctx, paymentID, status, errorMsg)
}

func (s *PaymentService) updatePaymentStatusWithTransaction(ctx context.Context, paymentID, status, transactionID string) error {
	query := `
		UPDATE payments
		SET status = $1, provider_transaction_id = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`

	_, err := s.db.ExecContext(ctx, query, status, transactionID, paymentID)
	return err
}

func (s *PaymentService) publishEvent(ctx context.Context, paymentID, eventType string, payload map[string]interface{}) {
	// Insert into outbox
	payloadJSON, _ := json.Marshal(payload)

	query := `
		INSERT INTO payment_outbox (id, aggregate_type, aggregate_id, event_type, payload)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := s.db.ExecContext(ctx, query,
		uuid.New().String(),
		"payment",
		paymentID,
		eventType,
		payloadJSON,
	)

	if err != nil {
		s.logger.Error("failed to publish event to outbox", zap.Error(err))
	}
}

func stateString(state circuitbreaker.State) string {
	switch state {
	case circuitbreaker.StateClosed:
		return "closed"
	case circuitbreaker.StateOpen:
		return "open"
	case circuitbreaker.StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

package provider

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// PaymentProvider defines the interface for payment providers
type PaymentProvider interface {
	ProcessPayment(ctx context.Context, req *ProcessPaymentRequest) (*ProcessPaymentResponse, error)
	CancelPayment(ctx context.Context, transactionID string) error
	RefundPayment(ctx context.Context, transactionID string, amount int64) (*RefundResponse, error)
}

// ProcessPaymentRequest represents a payment processing request
type ProcessPaymentRequest struct {
	OrderID       string
	Amount        int64
	Currency      string
	PaymentMethod string
	CardNumber    string
	CVV           string
	ExpiryMonth   int
	ExpiryYear    int
}

// ProcessPaymentResponse represents a payment processing response
type ProcessPaymentResponse struct {
	TransactionID string
	Status        string
	Message       string
}

// RefundResponse represents a refund response
type RefundResponse struct {
	RefundID string
	Status   string
}

// MockProvider is a mock payment provider for testing
type MockProvider struct {
	logger      *zap.Logger
	failureRate float64
	delayMs     int
}

// NewMockProvider creates a new mock payment provider
func NewMockProvider(logger *zap.Logger, failureRate float64, delayMs int) *MockProvider {
	return &MockProvider{
		logger:      logger,
		failureRate: failureRate,
		delayMs:     delayMs,
	}
}

func randomFloat() float64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return float64(binary.LittleEndian.Uint64(b[:])) / float64(^uint64(0))
}

// ProcessPayment processes a payment (mock implementation)
func (p *MockProvider) ProcessPayment(ctx context.Context, req *ProcessPaymentRequest) (*ProcessPaymentResponse, error) {
	time.Sleep(time.Duration(p.delayMs) * time.Millisecond)

	if randomFloat() < p.failureRate {
		p.logger.Warn("payment processing failed (simulated)",
			zap.String("order_id", req.OrderID),
		)
		return nil, fmt.Errorf("payment declined by provider")
	}

	// Generate mock transaction ID
	transactionID := fmt.Sprintf("TXN-%d", time.Now().UnixNano())

	p.logger.Info("payment processed successfully (mock)",
		zap.String("order_id", req.OrderID),
		zap.String("transaction_id", transactionID),
		zap.Int64("amount", req.Amount),
	)

	return &ProcessPaymentResponse{
		TransactionID: transactionID,
		Status:        "succeeded",
		Message:       "Payment processed successfully",
	}, nil
}

// CancelPayment cancels a payment (mock implementation)
func (p *MockProvider) CancelPayment(ctx context.Context, transactionID string) error {
	time.Sleep(time.Duration(p.delayMs) * time.Millisecond)

	p.logger.Info("payment canceled (mock)",
		zap.String("transaction_id", transactionID),
	)

	return nil
}

// RefundPayment refunds a payment (mock implementation)
func (p *MockProvider) RefundPayment(ctx context.Context, transactionID string, amount int64) (*RefundResponse, error) {
	time.Sleep(time.Duration(p.delayMs) * time.Millisecond)

	refundID := fmt.Sprintf("REFUND-%d", time.Now().UnixNano())

	p.logger.Info("payment refunded (mock)",
		zap.String("transaction_id", transactionID),
		zap.String("refund_id", refundID),
		zap.Int64("amount", amount),
	)

	return &RefundResponse{
		RefundID: refundID,
		Status:   "succeeded",
	}, nil
}

package outbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mumumio1/coldy/pkg/pubsub"
	"github.com/mumumio1/coldy/services/orders/internal/repository"
	"go.uber.org/zap"
)

// Publisher processes outbox events and publishes to Pub/Sub
type Publisher struct {
	repo      *repository.OrderRepository
	publisher *pubsub.Publisher
	logger    *zap.Logger
	interval  time.Duration
}

// NewPublisher creates a new outbox publisher
func NewPublisher(
	repo *repository.OrderRepository,
	publisher *pubsub.Publisher,
	logger *zap.Logger,
	interval time.Duration,
) *Publisher {
	return &Publisher{
		repo:      repo,
		publisher: publisher,
		logger:    logger,
		interval:  interval,
	}
}

// Start starts the outbox publisher worker
func (p *Publisher) Start(ctx context.Context) error {
	p.logger.Info("starting outbox publisher")

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("stopping outbox publisher")
			return ctx.Err()
		case <-ticker.C:
			if err := p.processEvents(ctx); err != nil {
				p.logger.Error("failed to process events", zap.Error(err))
			}
		}
	}
}

func (p *Publisher) processEvents(ctx context.Context) error {
	// Get unpublished events
	events, err := p.repo.GetUnpublishedEvents(ctx, 100)
	if err != nil {
		return fmt.Errorf("failed to get unpublished events: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	p.logger.Info("processing outbox events", zap.Int("count", len(events)))

	for _, event := range events {
		if err := p.publishEvent(ctx, event); err != nil {
			p.logger.Error("failed to publish event",
				zap.String("event_id", event.ID),
				zap.Error(err),
			)
			continue
		}

		// Mark as published
		if err := p.repo.MarkEventPublished(ctx, event.ID); err != nil {
			p.logger.Error("failed to mark event published",
				zap.String("event_id", event.ID),
				zap.Error(err),
			)
			continue
		}

		p.logger.Info("event published",
			zap.String("event_id", event.ID),
			zap.String("event_type", event.EventType),
		)
	}

	return nil
}

func (p *Publisher) publishEvent(ctx context.Context, event *repository.OutboxEvent) error {
	// Serialize payload
	data, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Deduplication via message ID
	messageID := p.generateMessageID(event.ID)

	// Set attributes
	attrs := map[string]string{
		"event_id":       event.ID,
		"aggregate_type": event.AggregateType,
		"aggregate_id":   event.AggregateID,
		"event_type":     event.EventType,
		"message_id":     messageID,
	}

	// Publish to Pub/Sub
	pubsubMessageID, err := p.publisher.Publish(ctx, event.EventType, data, attrs)
	if err != nil {
		return fmt.Errorf("failed to publish to pubsub: %w", err)
	}

	p.logger.Debug("published to pubsub",
		zap.String("event_id", event.ID),
		zap.String("topic", event.EventType),
		zap.String("message_id", pubsubMessageID),
	)

	return nil
}

// generateMessageID creates message ID from outbox ID
func (p *Publisher) generateMessageID(outboxID string) string {
	hash := sha256.Sum256([]byte(outboxID))
	return hex.EncodeToString(hash[:])
}

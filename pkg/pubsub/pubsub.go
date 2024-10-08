package pubsub

import (
	"context"
	"fmt"
	"sync"

	"cloud.google.com/go/pubsub"
	"go.uber.org/zap"
)

// Publisher wraps Google Cloud Pub/Sub publisher
type Publisher struct {
	client *pubsub.Client
	topics map[string]*pubsub.Topic
	mu     sync.RWMutex
	logger *zap.Logger
}

// NewPublisher creates a new Pub/Sub publisher
func NewPublisher(ctx context.Context, projectID string, logger *zap.Logger) (*Publisher, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub client: %w", err)
	}

	return &Publisher{
		client: client,
		topics: make(map[string]*pubsub.Topic),
		logger: logger,
	}, nil
}

// GetTopic returns or creates a topic
func (p *Publisher) GetTopic(ctx context.Context, topicName string) (*pubsub.Topic, error) {
	p.mu.RLock()
	topic, exists := p.topics[topicName]
	p.mu.RUnlock()

	if exists {
		return topic, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if topic, exists := p.topics[topicName]; exists {
		return topic, nil
	}

	topic = p.client.Topic(topicName)

	// Check if topic exists, create if not
	exists, err := topic.Exists(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check topic existence: %w", err)
	}

	if !exists {
		topic, err = p.client.CreateTopic(ctx, topicName)
		if err != nil {
			return nil, fmt.Errorf("failed to create topic: %w", err)
		}
		p.logger.Info("created topic", zap.String("topic", topicName))
	}

	p.topics[topicName] = topic
	return topic, nil
}

// Publish publishes a message to a topic
func (p *Publisher) Publish(ctx context.Context, topicName string, data []byte, attrs map[string]string) (string, error) {
	topic, err := p.GetTopic(ctx, topicName)
	if err != nil {
		return "", err
	}

	result := topic.Publish(ctx, &pubsub.Message{
		Data:       data,
		Attributes: attrs,
	})

	messageID, err := result.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to publish message: %w", err)
	}

	p.logger.Debug("message published",
		zap.String("topic", topicName),
		zap.String("message_id", messageID),
	)

	return messageID, nil
}

// Close closes the publisher
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, topic := range p.topics {
		topic.Stop()
	}

	return p.client.Close()
}

// Subscriber wraps Google Cloud Pub/Sub subscriber
type Subscriber struct {
	client *pubsub.Client
	logger *zap.Logger
}

// NewSubscriber creates a new Pub/Sub subscriber
func NewSubscriber(ctx context.Context, projectID string, logger *zap.Logger) (*Subscriber, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub client: %w", err)
	}

	return &Subscriber{
		client: client,
		logger: logger,
	}, nil
}

// MessageHandler is a function that handles messages
type MessageHandler func(ctx context.Context, msg *pubsub.Message) error

// Subscribe subscribes to a topic and processes messages
func (s *Subscriber) Subscribe(ctx context.Context, subscriptionName string, handler MessageHandler) error {
	sub := s.client.Subscription(subscriptionName)

	// Check if subscription exists
	exists, err := sub.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check subscription existence: %w", err)
	}

	if !exists {
		return fmt.Errorf("subscription %s does not exist", subscriptionName)
	}

	s.logger.Info("starting subscription", zap.String("subscription", subscriptionName))

	err = sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		s.logger.Debug("received message",
			zap.String("subscription", subscriptionName),
			zap.String("message_id", msg.ID),
		)

		if err := handler(ctx, msg); err != nil {
			s.logger.Error("failed to handle message",
				zap.String("message_id", msg.ID),
				zap.Error(err),
			)
			msg.Nack()
			return
		}

		msg.Ack()
	})

	if err != nil {
		return fmt.Errorf("subscription receive error: %w", err)
	}

	return nil
}

// Close closes the subscriber
func (s *Subscriber) Close() error {
	return s.client.Close()
}

// CreateSubscription creates a new subscription
func (s *Subscriber) CreateSubscription(ctx context.Context, subscriptionName, topicName string) error {
	topic := s.client.Topic(topicName)

	sub, err := s.client.CreateSubscription(ctx, subscriptionName, pubsub.SubscriptionConfig{
		Topic:            topic,
		AckDeadline:      60,  // 60 seconds
		ExpirationPolicy: nil, // Never expire
	})
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	s.logger.Info("created subscription",
		zap.String("subscription", subscriptionName),
		zap.String("topic", topicName),
	)

	_ = sub
	return nil
}

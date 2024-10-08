package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"cloud.google.com/go/pubsub"
	"github.com/mumumio1/coldy/pkg/logger"
	pubsubpkg "github.com/mumumio1/coldy/pkg/pubsub"
	"go.uber.org/zap"
)

const (
	serviceName = "notification"
	version     = "1.0.0"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log, err := logger.NewLogger(serviceName, getEnv("ENV", "development"))
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	log.Info("starting notification service", zap.String("version", version))

	projectID := getEnv("GCP_PROJECT_ID", "coldy-local")
	subscriber, err := pubsubpkg.NewSubscriber(ctx, projectID, log)
	if err != nil {
		return fmt.Errorf("failed to create subscriber: %w", err)
	}
	defer func() { _ = subscriber.Close() }()

	// Subscribe to events
	go func() {
		if err := subscriber.Subscribe(ctx, "order-created-sub", handleOrderCreated(log)); err != nil {
			log.Error("order created subscription failed", zap.Error(err))
		}
	}()

	go func() {
		if err := subscriber.Subscribe(ctx, "payment-succeeded-sub", handlePaymentSucceeded(log)); err != nil {
			log.Error("payment succeeded subscription failed", zap.Error(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info("shutting down...")
	return nil
}

func handleOrderCreated(log *zap.Logger) pubsubpkg.MessageHandler {
	return func(ctx context.Context, msg *pubsub.Message) error {
		log.Info("order created notification",
			zap.String("message_id", msg.ID),
			zap.ByteString("data", msg.Data),
		)
		// Send email/webhook/slack notification
		return nil
	}
}

func handlePaymentSucceeded(log *zap.Logger) pubsubpkg.MessageHandler {
	return func(ctx context.Context, msg *pubsub.Message) error {
		log.Info("payment succeeded notification",
			zap.String("message_id", msg.ID),
			zap.ByteString("data", msg.Data),
		)
		return nil
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

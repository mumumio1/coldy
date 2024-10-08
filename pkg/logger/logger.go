package logger

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey string

const loggerKey contextKey = "logger"

// NewLogger creates a new structured logger
func NewLogger(serviceName, env string) (*zap.Logger, error) {
	var config zap.Config

	if env == "production" {
		config = zap.NewProductionConfig()
		config.EncoderConfig.TimeKey = "timestamp"
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		config = zap.NewDevelopmentConfig()
	}

	config.InitialFields = map[string]interface{}{
		"service": serviceName,
		"env":     env,
	}

	logger, err := config.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	return logger, nil
}

// WithLogger adds logger to context
func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext extracts logger from context
func FromContext(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
		return logger
	}
	// Return a default logger if not found
	logger, _ := zap.NewProduction()
	return logger
}

// WithFields adds fields to logger in context
func WithFields(ctx context.Context, fields ...zap.Field) context.Context {
	logger := FromContext(ctx).With(fields...)
	return WithLogger(ctx, logger)
}

// WithRequestID adds request ID to logger
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return WithFields(ctx, zap.String("request_id", requestID))
}

// WithTraceID adds trace ID to logger
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return WithFields(ctx, zap.String("trace_id", traceID))
}

// WithUserID adds user ID to logger
func WithUserID(ctx context.Context, userID string) context.Context {
	return WithFields(ctx, zap.String("user_id", userID))
}

// GetHostname returns the hostname
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

package middleware

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	RequestIDHeader     = "x-request-id"
	CorrelationIDHeader = "x-correlation-id"
	TraceIDHeader       = "x-trace-id"
	SpanIDHeader        = "x-span-id"
)

// UnaryServerInterceptor returns a gRPC unary server interceptor with logging and tracing
func UnaryServerInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// Extract metadata
		md, _ := metadata.FromIncomingContext(ctx)
		requestID := getMetadataValue(md, RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		correlationID := getMetadataValue(md, CorrelationIDHeader)
		if correlationID == "" {
			correlationID = requestID
		}

		// Add to context
		ctx = metadata.AppendToOutgoingContext(ctx,
			RequestIDHeader, requestID,
			CorrelationIDHeader, correlationID,
		)

		// Create logger with request context
		reqLogger := logger.With(
			zap.String("request_id", requestID),
			zap.String("correlation_id", correlationID),
			zap.String("method", info.FullMethod),
		)

		// Add trace ID if available
		span := trace.SpanFromContext(ctx)
		if span.SpanContext().HasTraceID() {
			traceID := span.SpanContext().TraceID().String()
			reqLogger = reqLogger.With(zap.String("trace_id", traceID))
		}

		reqLogger.Info("gRPC request started")

		// Call handler
		resp, err := handler(ctx, req)

		duration := time.Since(start)

		// Log response
		if err != nil {
			st, _ := status.FromError(err)
			reqLogger.Error("gRPC request failed",
				zap.Duration("duration", duration),
				zap.String("code", st.Code().String()),
				zap.Error(err),
			)
		} else {
			reqLogger.Info("gRPC request completed",
				zap.Duration("duration", duration),
			)
		}

		return resp, err
	}
}

// UnaryClientInterceptor returns a gRPC unary client interceptor with tracing
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Extract metadata from context
		md, _ := metadata.FromOutgoingContext(ctx)

		// Propagate request ID and correlation ID
		requestID := getMetadataValue(md, RequestIDHeader)
		correlationID := getMetadataValue(md, CorrelationIDHeader)

		if requestID != "" || correlationID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx,
				RequestIDHeader, requestID,
				CorrelationIDHeader, correlationID,
			)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor
func StreamServerInterceptor(logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		// Extract metadata
		ctx := ss.Context()
		md, _ := metadata.FromIncomingContext(ctx)
		requestID := getMetadataValue(md, RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		reqLogger := logger.With(
			zap.String("request_id", requestID),
			zap.String("method", info.FullMethod),
		)

		reqLogger.Info("gRPC stream started")

		err := handler(srv, ss)

		duration := time.Since(start)

		if err != nil {
			reqLogger.Error("gRPC stream failed",
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			reqLogger.Info("gRPC stream completed",
				zap.Duration("duration", duration),
			)
		}

		return err
	}
}

// RecoveryInterceptor recovers from panics and returns internal error
func RecoveryInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					zap.String("method", info.FullMethod),
					zap.Any("panic", r),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}

// TracingInterceptor adds OpenTelemetry tracing
func TracingInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		tracer := otel.Tracer(serviceName)

		ctx, span := tracer.Start(ctx, info.FullMethod)
		defer span.End()

		return handler(ctx, req)
	}
}

func getMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

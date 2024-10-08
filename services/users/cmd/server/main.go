package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mumumio1/coldy/pkg/database"
	"github.com/mumumio1/coldy/pkg/logger"
	"github.com/mumumio1/coldy/pkg/middleware"
	"github.com/mumumio1/coldy/pkg/telemetry"
	usersv1 "github.com/mumumio1/coldy/proto/users/v1"
	grpcserver "github.com/mumumio1/coldy/services/users/internal/grpc"
	"github.com/mumumio1/coldy/services/users/internal/repository"
	"github.com/mumumio1/coldy/services/users/internal/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const (
	serviceName = "users"
	version     = "1.0.0"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize logger
	log, err := logger.NewLogger(serviceName, getEnv("ENV", "development"))
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	log.Info("starting users service", zap.String("version", version))

	// Initialize tracing
	tracingEndpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	shutdownTracer, err := telemetry.InitTracer(ctx, serviceName, version, tracingEndpoint)
	if err != nil {
		log.Warn("failed to initialize tracer", zap.Error(err))
	} else {
		defer func() { _ = shutdownTracer(ctx) }()
	}

	// Initialize metrics
	metrics := telemetry.NewMetrics("coldy", serviceName)

	// Initialize database
	dbConfig := database.Config{
		Host:            getEnv("DB_HOST", "localhost"),
		Port:            5432,
		User:            getEnv("DB_USER", "coldy"),
		Password:        getEnv("DB_PASSWORD", "coldy123"),
		Database:        getEnv("DB_NAME", "coldy"),
		SSLMode:         getEnv("DB_SSLMODE", "disable"),
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}

	db, err := database.NewPostgresDB(ctx, dbConfig, log)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Initialize repository and services
	userRepo := repository.NewUserRepository(db)
	jwtSecret := getEnv("JWT_SECRET", "your-secret-key-change-in-production")
	authService := service.NewAuthService(jwtSecret)
	userService := service.NewUserService(userRepo, authService, log)

	// Start gRPC server
	grpcPort := getEnv("GRPC_PORT", "50051")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.RecoveryInterceptor(log),
			middleware.UnaryServerInterceptor(log),
			middleware.TracingInterceptor(serviceName),
		),
		grpc.ChainStreamInterceptor(
			middleware.StreamServerInterceptor(log),
		),
	)

	// Register services
	usersv1.RegisterUserServiceServer(grpcServer, grpcserver.NewServer(userService, log))

	// Register health check
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus(serviceName, grpc_health_v1.HealthCheckResponse_SERVING)

	// Register reflection for development
	if getEnv("ENV", "development") == "development" {
		reflection.Register(grpcServer)
	}

	// Start metrics server
	metricsPort := getEnv("METRICS_PORT", "9090")
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		})
		mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
			if err := database.HealthCheck(r.Context(), db); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("READY"))
		})

		log.Info("starting metrics server", zap.String("port", metricsPort))
		if err := http.ListenAndServe(":"+metricsPort, mux); err != nil {
			log.Error("metrics server failed", zap.Error(err))
		}
	}()

	// Start gRPC server in goroutine
	go func() {
		log.Info("starting gRPC server", zap.String("port", grpcPort))
		if err := grpcServer.Serve(lis); err != nil {
			log.Error("gRPC server failed", zap.Error(err))
		}
	}()

	// Monitor database connection pool
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := database.GetStats(db)
				metrics.DBConnections.Set(float64(stats.InUse))
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info("shutting down gracefully...")

	// Graceful shutdown
	healthServer.SetServingStatus(serviceName, grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// Give time for load balancers to remove this instance
	time.Sleep(5 * time.Second)

	// Stop accepting new connections
	grpcServer.GracefulStop()

	log.Info("server stopped")
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

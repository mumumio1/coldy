package telemetry

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all application metrics
type Metrics struct {
	// RED metrics
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	ErrorsTotal     *prometheus.CounterVec

	// USE metrics
	CPUUsage         prometheus.Gauge
	MemoryUsage      prometheus.Gauge
	DBConnections    prometheus.Gauge
	RedisConnections prometheus.Gauge

	// Business metrics
	BusinessMetrics *prometheus.CounterVec
}

// NewMetrics creates a new metrics instance
func NewMetrics(namespace, subsystem string) *Metrics {
	return &Metrics{
		// RED: Rate, Errors, Duration
		RequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "requests_total",
				Help:      "Total number of requests",
			},
			[]string{"method", "endpoint", "status"},
		),
		RequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "request_duration_seconds",
				Help:      "Request duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "endpoint"},
		),
		ErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "errors_total",
				Help:      "Total number of errors",
			},
			[]string{"method", "endpoint", "error_type"},
		),

		// USE: Utilization, Saturation, Errors
		CPUUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "cpu_usage_percent",
				Help:      "CPU usage percentage",
			},
		),
		MemoryUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "memory_usage_bytes",
				Help:      "Memory usage in bytes",
			},
		),
		DBConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "db_connections_active",
				Help:      "Number of active DB connections",
			},
		),
		RedisConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "redis_connections_active",
				Help:      "Number of active Redis connections",
			},
		),

		// Business metrics
		BusinessMetrics: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "business_events_total",
				Help:      "Total number of business events",
			},
			[]string{"event_type", "status"},
		),
	}
}

// ObserveRequest records request metrics
func (m *Metrics) ObserveRequest(method, endpoint, status string, duration time.Duration) {
	m.RequestsTotal.WithLabelValues(method, endpoint, status).Inc()
	m.RequestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
}

// RecordError records an error
func (m *Metrics) RecordError(method, endpoint, errorType string) {
	m.ErrorsTotal.WithLabelValues(method, endpoint, errorType).Inc()
}

// RecordBusinessEvent records a business event
func (m *Metrics) RecordBusinessEvent(eventType, status string) {
	m.BusinessMetrics.WithLabelValues(eventType, status).Inc()
}

// MetricsMiddleware wraps handlers with metrics collection
func (m *Metrics) MetricsMiddleware(method, endpoint string) func(next func(context.Context) error) func(context.Context) error {
	return func(next func(context.Context) error) func(context.Context) error {
		return func(ctx context.Context) error {
			start := time.Now()
			err := next(ctx)
			duration := time.Since(start)

			status := "success"
			if err != nil {
				status = "error"
				m.RecordError(method, endpoint, "internal_error")
			}

			m.ObserveRequest(method, endpoint, status, duration)
			return err
		}
	}
}

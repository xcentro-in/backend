package metrics

import (
	"context"
	"time"
)

// MetricType represents the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeTimer     MetricType = "timer"
	MetricTypeHistogram MetricType = "histogram"
)

// MetricLabels represents key-value pairs for metric dimensions/labels
type MetricLabels map[string]string

// MetricsProvider defines the interface for metrics collection
// This interface is designed to be implementation-agnostic,
// allowing easy migration between CloudWatch, Prometheus, Datadog, etc.
type MetricsProvider interface {
	// Counter increments a counter metric by the given value
	// name: metric name (e.g., "http.requests", "payment.success")
	// value: increment value (usually 1)
	// labels: optional key-value pairs for dimensions
	Counter(ctx context.Context, name string, value float64, labels MetricLabels) error

	// Gauge sets a gauge metric to the given value
	// name: metric name (e.g., "queue.size", "active.connections")
	// value: current gauge value
	// labels: optional key-value pairs for dimensions
	Gauge(ctx context.Context, name string, value float64, labels MetricLabels) error

	// Timer records a duration/timing metric
	// name: metric name (e.g., "api.latency", "db.query.duration")
	// value: duration in milliseconds
	// labels: optional key-value pairs for dimensions
	Timer(ctx context.Context, name string, value time.Duration, labels MetricLabels) error

	// Histogram records a value in a histogram metric
	// name: metric name (e.g., "request.size", "response.size")
	// value: observed value
	// labels: optional key-value pairs for dimensions
	Histogram(ctx context.Context, name string, value float64, labels MetricLabels) error

	// Flush forces any buffered metrics to be sent immediately
	// Useful for graceful shutdown
	Flush(ctx context.Context) error

	// Close gracefully shuts down the metrics provider
	// Flushes any pending metrics and releases resources
	Close() error
}

// NoOpMetricsProvider is a metrics provider that does nothing
// Useful for testing or when metrics are disabled
type NoOpMetricsProvider struct{}

// Ensure NoOpMetricsProvider implements MetricsProvider
var _ MetricsProvider = (*NoOpMetricsProvider)(nil)

func (n *NoOpMetricsProvider) Counter(ctx context.Context, name string, value float64, labels MetricLabels) error {
	return nil
}

func (n *NoOpMetricsProvider) Gauge(ctx context.Context, name string, value float64, labels MetricLabels) error {
	return nil
}

func (n *NoOpMetricsProvider) Timer(ctx context.Context, name string, value time.Duration, labels MetricLabels) error {
	return nil
}

func (n *NoOpMetricsProvider) Histogram(ctx context.Context, name string, value float64, labels MetricLabels) error {
	return nil
}

func (n *NoOpMetricsProvider) Flush(ctx context.Context) error {
	return nil
}

func (n *NoOpMetricsProvider) Close() error {
	return nil
}

// NewNoOpMetricsProvider creates a new no-op metrics provider
func NewNoOpMetricsProvider() MetricsProvider {
	return &NoOpMetricsProvider{}
}

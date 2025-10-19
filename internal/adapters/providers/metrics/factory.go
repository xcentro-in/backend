package metrics

import (
	"fmt"

	"xcentro.in/internal/core/metrics"
	"xcentro.in/internal/shared/config"
)

// ProviderType represents the type of metrics provider
type ProviderType string

const (
	ProviderCloudWatch ProviderType = "cloudwatch"
	ProviderPrometheus ProviderType = "prometheus" // Future implementation
	ProviderDatadog    ProviderType = "datadog"    // Future implementation
	ProviderNoOp       ProviderType = "noop"       // No-op provider (testing/disabled)
)

// NewMetricsProvider creates a metrics provider based on configuration
func NewMetricsProvider(cfg config.Config) (metrics.MetricsProvider, error) {
	// If CloudWatch is enabled, use CloudWatch
	if cfg.CloudWatch.Enabled {
		return NewCloudWatchMetrics(cfg.CloudWatch)
	}

	// Default to no-op provider (metrics disabled)
	return metrics.NewNoOpMetricsProvider(), nil
}

// NewMetricsProviderByType creates a metrics provider by explicit type
// Useful for testing or when you need to override config
func NewMetricsProviderByType(providerType ProviderType, cfg config.Config) (metrics.MetricsProvider, error) {
	switch providerType {
	case ProviderCloudWatch:
		return NewCloudWatchMetrics(cfg.CloudWatch)
	case ProviderPrometheus:
		// TODO: Implement Prometheus provider
		return nil, fmt.Errorf("prometheus provider not implemented yet")
	case ProviderDatadog:
		// TODO: Implement Datadog provider
		return nil, fmt.Errorf("datadog provider not implemented yet")
	case ProviderNoOp:
		return metrics.NewNoOpMetricsProvider(), nil
	default:
		return nil, fmt.Errorf("unknown metrics provider type: %s", providerType)
	}
}

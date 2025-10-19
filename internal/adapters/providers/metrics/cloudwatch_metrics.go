package metrics

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	awsconfig "xcentro.in/internal/adapters/providers/aws/config"
	"xcentro.in/internal/core/metrics"
	"xcentro.in/internal/shared/config"
)

const (
	// CloudWatch limits
	maxDimensionsPerMetric  = 30   // CloudWatch max dimensions per metric
	maxMetricsPerPutRequest = 1000 // CloudWatch API limit
)

// metricData represents a single metric data point to be sent
type metricData struct {
	name       string
	value      float64
	unit       types.StandardUnit
	labels     metrics.MetricLabels
	timestamp  time.Time
	metricType metrics.MetricType
}

// CloudWatchMetrics implements MetricsProvider for AWS CloudWatch
type CloudWatchMetrics struct {
	client    *cloudwatch.Client
	config    config.CloudWatchConfig
	namespace string

	// Batching
	buffer      []metricData
	bufferMutex sync.Mutex
	ticker      *time.Ticker
	stopCh      chan struct{}
	flushWg     sync.WaitGroup

	// Logging
	logger *log.Logger
}

// Ensure CloudWatchMetrics implements MetricsProvider
var _ metrics.MetricsProvider = (*CloudWatchMetrics)(nil)

// NewCloudWatchMetrics creates a new CloudWatch metrics provider
func NewCloudWatchMetrics(cfg config.CloudWatchConfig) (*CloudWatchMetrics, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("cloudwatch metrics is disabled")
	}

	if cfg.Namespace == "" {
		return nil, fmt.Errorf("cloudwatch namespace is required")
	}

	// Load AWS config using centralized helper
	awsCfg, err := awsconfig.LoadAWSConfig(context.Background(), awsconfig.AWSConfig{
		Region:       cfg.Region,
		Endpoint:     cfg.Endpoint,
		AccessKey:    cfg.AccessKey,
		SecretKey:    cfg.SecretKey,
		SessionToken: cfg.SessionToken,
		Profile:      cfg.Profile,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := cloudwatch.NewFromConfig(awsCfg)

	cw := &CloudWatchMetrics{
		client:    client,
		config:    cfg,
		namespace: cfg.Namespace,
		buffer:    make([]metricData, 0, cfg.BatchSize),
		stopCh:    make(chan struct{}),
		logger:    log.New(log.Writer(), "[CloudWatch] ", log.LstdFlags),
	}

	// Start background batch flusher
	cw.startBatchFlusher()

	return cw, nil
}

// startBatchFlusher starts a background goroutine that periodically flushes buffered metrics
func (cw *CloudWatchMetrics) startBatchFlusher() {
	cw.ticker = time.NewTicker(cw.config.BatchInterval)

	cw.flushWg.Add(1)
	go func() {
		defer cw.flushWg.Done()

		for {
			select {
			case <-cw.ticker.C:
				// Periodic flush
				if err := cw.Flush(context.Background()); err != nil {
					cw.logger.Printf("periodic flush failed: %v", err)
				}
			case <-cw.stopCh:
				// Graceful shutdown
				cw.ticker.Stop()
				if cw.config.FlushOnShutdown {
					if err := cw.Flush(context.Background()); err != nil {
						cw.logger.Printf("shutdown flush failed: %v", err)
					}
				}
				return
			}
		}
	}()
}

// Counter increments a counter metric
func (cw *CloudWatchMetrics) Counter(ctx context.Context, name string, value float64, labels metrics.MetricLabels) error {
	// Async submission via goroutine
	go func() {
		cw.addMetric(metricData{
			name:       name,
			value:      value,
			unit:       types.StandardUnitCount,
			labels:     labels,
			timestamp:  time.Now(),
			metricType: metrics.MetricTypeCounter,
		})
	}()
	return nil
}

// Gauge sets a gauge metric to the given value
func (cw *CloudWatchMetrics) Gauge(ctx context.Context, name string, value float64, labels metrics.MetricLabels) error {
	// Async submission via goroutine
	go func() {
		cw.addMetric(metricData{
			name:       name,
			value:      value,
			unit:       types.StandardUnitNone,
			labels:     labels,
			timestamp:  time.Now(),
			metricType: metrics.MetricTypeGauge,
		})
	}()
	return nil
}

// Timer records a duration metric
func (cw *CloudWatchMetrics) Timer(ctx context.Context, name string, value time.Duration, labels metrics.MetricLabels) error {
	// Async submission via goroutine
	go func() {
		cw.addMetric(metricData{
			name:       name,
			value:      float64(value.Milliseconds()),
			unit:       types.StandardUnitMilliseconds,
			labels:     labels,
			timestamp:  time.Now(),
			metricType: metrics.MetricTypeTimer,
		})
	}()
	return nil
}

// Histogram records a histogram metric
// Note: CloudWatch doesn't have native histogram support
// We log a warning and treat it as a regular value metric
func (cw *CloudWatchMetrics) Histogram(ctx context.Context, name string, value float64, labels metrics.MetricLabels) error {
	// Async submission via goroutine
	go func() {
		cw.logger.Printf("Histogram metric '%s' not fully supported by CloudWatch, storing as value metric", name)
		cw.addMetric(metricData{
			name:       name,
			value:      value,
			unit:       types.StandardUnitNone,
			labels:     labels,
			timestamp:  time.Now(),
			metricType: metrics.MetricTypeHistogram,
		})
	}()
	return nil
}

// addMetric adds a metric to the buffer
// Automatically triggers flush if buffer reaches batch size
func (cw *CloudWatchMetrics) addMetric(m metricData) {
	cw.bufferMutex.Lock()
	defer cw.bufferMutex.Unlock()

	cw.buffer = append(cw.buffer, m)

	// Trigger immediate flush if buffer is full
	if len(cw.buffer) >= cw.config.BatchSize {
		// Flush in background to avoid blocking
		bufferedMetrics := make([]metricData, len(cw.buffer))
		copy(bufferedMetrics, cw.buffer)
		cw.buffer = cw.buffer[:0] // Clear buffer

		go func() {
			if err := cw.sendMetrics(context.Background(), bufferedMetrics); err != nil {
				cw.logger.Printf("failed to send batch: %v", err)
			}
		}()
	}
}

// Flush sends all buffered metrics immediately
func (cw *CloudWatchMetrics) Flush(ctx context.Context) error {
	cw.bufferMutex.Lock()
	if len(cw.buffer) == 0 {
		cw.bufferMutex.Unlock()
		return nil
	}

	// Copy buffer and clear
	bufferedMetrics := make([]metricData, len(cw.buffer))
	copy(bufferedMetrics, cw.buffer)
	cw.buffer = cw.buffer[:0]
	cw.bufferMutex.Unlock()

	return cw.sendMetrics(ctx, bufferedMetrics)
}

// sendMetrics sends metrics to CloudWatch with exponential backoff retry
func (cw *CloudWatchMetrics) sendMetrics(ctx context.Context, metricsData []metricData) error {
	if len(metricsData) == 0 {
		return nil
	}

	// Convert to CloudWatch metric data format
	cwMetrics := make([]types.MetricDatum, 0, len(metricsData))

	for _, m := range metricsData {
		dimensions, err := cw.buildDimensions(m.labels)
		if err != nil {
			cw.logger.Printf("failed to build dimensions for metric '%s': %v", m.name, err)
			continue
		}

		cwMetrics = append(cwMetrics, types.MetricDatum{
			MetricName: aws.String(m.name),
			Value:      aws.Float64(m.value),
			Unit:       m.unit,
			Timestamp:  aws.Time(m.timestamp),
			Dimensions: dimensions,
		})
	}

	if len(cwMetrics) == 0 {
		return nil
	}

	// Split into batches if needed (CloudWatch limit: 1000 metrics per request)
	batches := cw.splitIntoBatches(cwMetrics, maxMetricsPerPutRequest)

	// Send each batch with exponential backoff retry
	for i, batch := range batches {
		if err := cw.sendBatchWithRetry(ctx, batch); err != nil {
			return fmt.Errorf("failed to send batch %d/%d: %w", i+1, len(batches), err)
		}
	}

	cw.logger.Printf("successfully sent %d metrics in %d batch(es)", len(cwMetrics), len(batches))
	return nil
}

// sendBatchWithRetry sends a single batch with exponential backoff retry (up to maxRetries)
func (cw *CloudWatchMetrics) sendBatchWithRetry(ctx context.Context, batch []types.MetricDatum) error {
	var lastErr error

	for attempt := 0; attempt < cw.config.MaxRetries; attempt++ {
		input := &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(cw.namespace),
			MetricData: batch,
		}

		_, err := cw.client.PutMetricData(ctx, input)
		if err == nil {
			// Success
			return nil
		}

		lastErr = err

		// Exponential backoff: 2^attempt * 100ms (100ms, 200ms, 400ms, 800ms, 1600ms)
		backoffDuration := time.Duration(math.Pow(2, float64(attempt))) * 100 * time.Millisecond
		cw.logger.Printf("attempt %d/%d failed for batch of %d metrics: %v (retrying in %v)",
			attempt+1, cw.config.MaxRetries, len(batch), err, backoffDuration)

		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled: %w", ctx.Err())
		case <-time.After(backoffDuration):
			// Continue to next retry
		}
	}

	return fmt.Errorf("failed after %d retries: %w", cw.config.MaxRetries, lastErr)
}

// buildDimensions converts labels to CloudWatch dimensions
// Validates dimension count (max 30 per metric)
func (cw *CloudWatchMetrics) buildDimensions(labels metrics.MetricLabels) ([]types.Dimension, error) {
	if len(labels) == 0 {
		return nil, nil
	}

	if len(labels) > maxDimensionsPerMetric {
		cw.logger.Printf("ERROR: metric has %d dimensions (max %d), truncating excess dimensions",
			len(labels), maxDimensionsPerMetric)
		// Don't fail, just log and truncate
	}

	dimensions := make([]types.Dimension, 0, len(labels))
	count := 0

	for key, value := range labels {
		if count >= maxDimensionsPerMetric {
			break
		}
		dimensions = append(dimensions, types.Dimension{
			Name:  aws.String(key),
			Value: aws.String(value),
		})
		count++
	}

	return dimensions, nil
}

// splitIntoBatches splits metrics into smaller batches
func (cw *CloudWatchMetrics) splitIntoBatches(metrics []types.MetricDatum, batchSize int) [][]types.MetricDatum {
	var batches [][]types.MetricDatum

	for i := 0; i < len(metrics); i += batchSize {
		end := i + batchSize
		if end > len(metrics) {
			end = len(metrics)
		}
		batches = append(batches, metrics[i:end])
	}

	return batches
}

// Close gracefully shuts down the metrics provider
func (cw *CloudWatchMetrics) Close() error {
	close(cw.stopCh)
	cw.flushWg.Wait()
	return nil
}

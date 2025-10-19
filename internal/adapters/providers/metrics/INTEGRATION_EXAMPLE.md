# Integration Example

Step-by-step guide to integrate CloudWatch metrics into your XCentro backend application.

## Step 1: Update Environment Variables

Add to `.env`:

```bash
# CloudWatch Metrics Configuration
CLOUDWATCH_ENABLED=true
CLOUDWATCH_NAMESPACE=XCentro/Backend
CLOUDWATCH_REGION=us-east-1
CLOUDWATCH_BATCH_INTERVAL=30s
CLOUDWATCH_BATCH_SIZE=20
CLOUDWATCH_MAX_RETRIES=5
CLOUDWATCH_FLUSH_ON_SHUTDOWN=true

# Optional: Static credentials (if not using IAM roles)
# CLOUDWATCH_ACCESS_KEY=your_access_key
# CLOUDWATCH_SECRET_KEY=your_secret_key
```

## Step 2: Initialize Metrics Provider in `cmd/api/main.go`

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "xcentro.in/internal/adapters/providers/metrics"
    "xcentro.in/internal/shared/config"
)

func main() {
    // Load configuration
    cfg := config.MustLoad("")

    // Initialize metrics provider
    metricsProvider, err := metrics.NewMetricsProvider(cfg)
    if err != nil {
        log.Fatalf("failed to initialize metrics provider: %v", err)
    }
    log.Println("Metrics provider initialized successfully")

    // Setup graceful shutdown
    defer func() {
        if err := metricsProvider.Close(); err != nil {
            log.Printf("failed to close metrics provider: %v", err)
        }
    }()

    // Pass metricsProvider to services/handlers
    // Example: initialize services with metrics
    paymentService := NewPaymentService(cfg, metricsProvider)
    userService := NewUserService(cfg, metricsProvider)

    // Start HTTP server
    // ...

    // Wait for shutdown signal
    waitForShutdown(metricsProvider)
}

func waitForShutdown(metricsProvider core.MetricsProvider) {
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    <-sigChan
    log.Println("Shutdown signal received, flushing metrics...")

    // Flush pending metrics before exit
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := metricsProvider.Flush(ctx); err != nil {
        log.Printf("failed to flush metrics: %v", err)
    }

    log.Println("Metrics flushed, shutting down...")
}
```

## Step 3: Add Metrics to Payment Service

`internal/services/payment_service.go`:

```go
package services

import (
    "context"
    "fmt"
    "time"

    "xcentro.in/internal/core/metrics"
)

type PaymentService struct {
    gateway         PaymentGateway
    metricsProvider metrics.MetricsProvider
}

func NewPaymentService(gateway PaymentGateway, metricsProvider metrics.MetricsProvider) *PaymentService {
    return &PaymentService{
        gateway:         gateway,
        metricsProvider: metricsProvider,
    }
}

func (s *PaymentService) ProcessPayment(ctx context.Context, req PaymentRequest) (*PaymentResponse, error) {
    start := time.Now()

    // Record incoming payment request
    s.metricsProvider.Counter(ctx, "payment.requests", 1, metrics.MetricLabels{
        "gateway": req.Gateway,
        "method":  req.PaymentMethod,
    })

    // Process payment
    resp, err := s.gateway.Charge(ctx, req)
    duration := time.Since(start)

    // Record processing duration
    s.metricsProvider.Timer(ctx, "payment.processing.duration", duration, metrics.MetricLabels{
        "gateway": req.Gateway,
        "method":  req.PaymentMethod,
    })

    if err != nil {
        // Record failure
        s.metricsProvider.Counter(ctx, "payment.failures", 1, metrics.MetricLabels{
            "gateway": req.Gateway,
            "reason":  classifyError(err),
        })
        return nil, err
    }

    // Record success
    s.metricsProvider.Counter(ctx, "payment.success", 1, metrics.MetricLabels{
        "gateway":  req.Gateway,
        "method":   req.PaymentMethod,
        "currency": req.Currency,
    })

    // Record payment amount (histogram)
    s.metricsProvider.Histogram(ctx, "payment.amount", float64(req.Amount), metrics.MetricLabels{
        "gateway":  req.Gateway,
        "currency": req.Currency,
    })

    return resp, nil
}

func classifyError(err error) string {
    // Classify error for metrics
    switch {
    case errors.Is(err, ErrInsufficientFunds):
        return "insufficient_funds"
    case errors.Is(err, ErrCardDeclined):
        return "card_declined"
    case errors.Is(err, ErrGatewayTimeout):
        return "gateway_timeout"
    default:
        return "unknown"
    }
}
```

## Step 4: Add HTTP Metrics Middleware

`internal/middleware/metrics_middleware.go`:

```go
package middleware

import (
    "fmt"
    "time"

    "github.com/labstack/echo/v4"
    "xcentro.in/internal/core/metrics"
)

func MetricsMiddleware(metricsProvider metrics.MetricsProvider) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            start := time.Now()
            req := c.Request()

            // Process request
            err := next(c)

            // Calculate duration
            duration := time.Since(start)
            status := c.Response().Status

            // Record HTTP request metrics
            labels := metrics.MetricLabels{
                "method": req.Method,
                "path":   c.Path(),
                "status": fmt.Sprintf("%d", status),
            }

            // Request counter
            metricsProvider.Counter(req.Context(), "http.requests", 1, labels)

            // Request latency
            metricsProvider.Timer(req.Context(), "http.latency", duration, labels)

            // Request size
            if req.ContentLength > 0 {
                metricsProvider.Histogram(req.Context(), "http.request.size", float64(req.ContentLength), labels)
            }

            // Response size
            responseSize := c.Response().Size
            if responseSize > 0 {
                metricsProvider.Histogram(req.Context(), "http.response.size", float64(responseSize), labels)
            }

            // Error tracking
            if err != nil {
                metricsProvider.Counter(req.Context(), "http.errors", 1, metrics.MetricLabels{
                    "method": req.Method,
                    "path":   c.Path(),
                    "type":   "handler_error",
                })
            }

            return err
        }
    }
}
```

Register middleware in your Echo server:

```go
e := echo.New()
e.Use(middleware.MetricsMiddleware(metricsProvider))
```

## Step 5: Add Database Metrics

`internal/adapters/database/postgres/user_repository.go`:

```go
package postgres

import (
    "context"
    "time"

    "xcentro.in/internal/core/metrics"
)

type UserRepository struct {
    db              *pgxpool.Pool
    metricsProvider metrics.MetricsProvider
}

func NewUserRepository(db *pgxpool.Pool, metricsProvider metrics.MetricsProvider) *UserRepository {
    return &UserRepository{
        db:              db,
        metricsProvider: metricsProvider,
    }
}

func (r *UserRepository) GetUserByID(ctx context.Context, userID string) (*User, error) {
    start := time.Now()

    query := `SELECT id, email, name FROM user_table WHERE id = $1`
    var user User
    err := r.db.QueryRow(ctx, query, userID).Scan(&user.ID, &user.Email, &user.Name)

    // Record query duration
    duration := time.Since(start)
    r.metricsProvider.Timer(ctx, "db.query.duration", duration, metrics.MetricLabels{
        "table":     "user_table",
        "operation": "get_user_by_id",
    })

    if err != nil {
        r.metricsProvider.Counter(ctx, "db.errors", 1, metrics.MetricLabels{
            "table": "user_table",
            "type":  "query_error",
        })
        return nil, err
    }

    r.metricsProvider.Counter(ctx, "db.queries", 1, metrics.MetricLabels{
        "table":     "user_table",
        "operation": "get_user_by_id",
        "status":    "success",
    })

    return &user, nil
}
```

## Step 6: Add Business Logic Metrics

`internal/core/business_rules/fee_calculator.go`:

```go
package business_rules

import (
    "context"
    "time"

    "xcentro.in/internal/core/metrics"
)

type FeeCalculator struct {
    metricsProvider metrics.MetricsProvider
}

func (f *FeeCalculator) CalculateFee(ctx context.Context, amount int64, context string) (int64, error) {
    start := time.Now()

    // Calculate fee
    fee := f.calculateFeeInternal(amount, context)

    // Record calculation time
    duration := time.Since(start)
    f.metricsProvider.Timer(ctx, "fee.calculation.duration", duration, metrics.MetricLabels{
        "context": context,
    })

    // Record fee amount
    f.metricsProvider.Histogram(ctx, "fee.amount", float64(fee), metrics.MetricLabels{
        "context": context,
    })

    return fee, nil
}
```

## Step 7: Monitor Metrics in CloudWatch

### AWS Console

1. Open AWS CloudWatch Console
2. Navigate to **Metrics** → **All metrics**
3. Select namespace: `XCentro/Backend`
4. Browse available metrics:
   - `payment.success`
   - `payment.failures`
   - `http.requests`
   - `db.query.duration`
   - etc.

### Create Alarms

```bash
# Example: Alert when payment failures exceed 10 per minute
aws cloudwatch put-metric-alarm \
    --alarm-name "HighPaymentFailureRate" \
    --alarm-description "Alert when payment failures > 10/min" \
    --namespace "XCentro/Backend" \
    --metric-name "payment.failures" \
    --statistic "Sum" \
    --period 60 \
    --evaluation-periods 1 \
    --threshold 10 \
    --comparison-operator "GreaterThanThreshold"
```

## Step 8: Create CloudWatch Dashboard

Example dashboard JSON:

```json
{
  "widgets": [
    {
      "type": "metric",
      "properties": {
        "metrics": [
          ["XCentro/Backend", "http.requests", {"stat": "Sum"}],
          [".", "payment.success", {"stat": "Sum"}],
          [".", "payment.failures", {"stat": "Sum"}]
        ],
        "period": 300,
        "stat": "Sum",
        "region": "us-east-1",
        "title": "Payment Metrics"
      }
    },
    {
      "type": "metric",
      "properties": {
        "metrics": [
          ["XCentro/Backend", "http.latency", {"stat": "Average"}],
          [".", "db.query.duration", {"stat": "Average"}],
          [".", "payment.processing.duration", {"stat": "Average"}]
        ],
        "period": 300,
        "stat": "Average",
        "region": "us-east-1",
        "title": "Latency Metrics",
        "yAxis": {
          "left": {
            "label": "Milliseconds"
          }
        }
      }
    }
  ]
}
```

## Testing with Localstack

```bash
# Start localstack
docker run -d -p 4566:4566 \
    -e SERVICES=cloudwatch \
    localstack/localstack

# Update .env
export CLOUDWATCH_ENABLED=true
export CLOUDWATCH_ENDPOINT=http://localhost:4566
export CLOUDWATCH_REGION=us-east-1

# Run application
go run ./cmd/api

# Query metrics (localstack)
aws --endpoint-url=http://localhost:4566 cloudwatch list-metrics \
    --namespace "XCentro/Backend" \
    --region us-east-1
```

## Common Metrics to Track

### HTTP/API Metrics
- `http.requests` - Total requests
- `http.latency` - Response time
- `http.errors` - Error count
- `http.request.size` - Request body size
- `http.response.size` - Response body size

### Payment Metrics
- `payment.requests` - Payment attempts
- `payment.success` - Successful payments
- `payment.failures` - Failed payments
- `payment.processing.duration` - Processing time
- `payment.amount` - Payment amounts

### Database Metrics
- `db.queries` - Query count
- `db.query.duration` - Query execution time
- `db.errors` - Database errors
- `db.active_connections` - Active connections

### Business Metrics
- `voucher.created` - Vouchers created
- `voucher.finalized` - Vouchers finalized
- `payout.initiated` - Payouts initiated
- `payout.completed` - Payouts completed
- `kyc.completed` - KYC completions

## Summary

✅ **Interface-based design** - Easy to swap CloudWatch for Prometheus
✅ **Async submission** - Non-blocking metrics collection
✅ **Batching** - Reduced CloudWatch API costs
✅ **Exponential retry** - Resilient to transient failures
✅ **Graceful shutdown** - No metric loss on application exit
✅ **Thread-safe** - Safe for concurrent use
✅ **Dimension validation** - Prevents CloudWatch errors

Your application is now fully instrumented with CloudWatch metrics! 🚀

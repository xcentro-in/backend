# XCentro Metrics Guide

## Overview

This guide documents the metrics implemented in the XCentro backend for monitoring payment processing, gateway health, and system performance. All metrics are sent to AWS CloudWatch (configurable for other providers like Prometheus).

## Metrics Infrastructure

### Provider
- **Current**: AWS CloudWatch
- **Future**: Prometheus, Datadog (interface-based design allows easy migration)
- **Implementation**: `/internal/core/metrics/metrics.go`
- **CloudWatch Client**: `/internal/adapters/providers/metrics/cloudwatch_metrics.go`

### Configuration

Set these environment variables in your `.env` file:

```bash
# Required
CLOUDWATCH_ENABLED=true
CLOUDWATCH_REGION=us-east-1

# Optional (with defaults)
CLOUDWATCH_NAMESPACE=XCentro/Backend
CLOUDWATCH_BATCH_INTERVAL=30s
CLOUDWATCH_BATCH_SIZE=20
CLOUDWATCH_MAX_RETRIES=5
CLOUDWATCH_FLUSH_ON_SHUTDOWN=true

# AWS Credentials (if not using IAM roles)
CLOUDWATCH_ACCESS_KEY=your_access_key
CLOUDWATCH_SECRET_KEY=your_secret_key
```

### Metric Types

1. **Counter**: Incremental values (e.g., request counts, payment success/failures)
2. **Gauge**: Point-in-time values (e.g., queue depth, active connections)
3. **Timer**: Duration measurements (e.g., API latency, gateway response time)
4. **Value**: Send individual values, CloudWatch calculates statistics (Average, p50, p95, p99)

**IMPORTANT**: CloudWatch does NOT support native histograms like Prometheus. Instead, send individual values and CloudWatch automatically calculates percentiles.

### How to Access CloudWatch Metrics

**⚠️ Important**: CloudWatch Metrics does NOT support SQL queries. Use these methods instead:

#### 1. **CloudWatch Console** (Recommended for Visualization)
```
AWS Console → CloudWatch → Metrics → All metrics → XCentro/Backend
```
- Browse metrics by dimension (gateway, payment_method, etc.)
- Create graphs with multiple metrics
- Use **Metric Math** for calculations like `(m1/m2)*100`
- Build dashboards with widgets

#### 2. **AWS CLI** (For Programmatic Access)
```bash
aws cloudwatch get-metric-statistics \
  --namespace "XCentro/Backend" \
  --metric-name "payment.success" \
  --start-time 2025-11-08T00:00:00Z \
  --end-time 2025-11-08T23:59:59Z \
  --period 300 \
  --statistics Sum
```

#### 3. **CloudWatch API** (For Custom Integrations)
Use AWS SDK to fetch metrics programmatically in your code.

**Note**: CloudWatch **Logs** Insights (different service) does support SQL-like queries, but not CloudWatch **Metrics**.

---

## Implemented Metrics

### 1. HTTP Metrics (All API Endpoints)

**Location**: `/internal/middleware/metrics_middleware.go`

| Metric Name | Type | Labels | Description |
|------------|------|--------|-------------|
| `http.request` | Counter | method, path, status | Count of HTTP requests |
| `http.request.duration` | Timer | method, path, status | Request processing time |
| `http.errors.4xx` | Counter | method, path, status_code | Client error count |
| `http.errors.5xx` | Counter | method, path, status_code | Server error count |
| `http.request.size` | Counter (value) | method, path | Request body size in bytes |
| `http.response.size` | Counter (value) | method, path | Response body size in bytes |

**How to Query in CloudWatch Console**:

1. **Average Response Time**:
   - Go to: AWS Console → CloudWatch → Metrics → All metrics
   - Select: `XCentro/Backend` → `http.request.duration`
   - Dimensions: method=POST, path=/v1/process_payment
   - Statistic: Average

2. **P95 Latency**:
   - Select metric: `http.request.duration`
   - Statistic: p95 (from dropdown)
   - Period: 5 minutes

3. **Error Rate** (using Metric Math):
   - Metric m1: `http.errors.5xx` (Statistic: Sum)
   - Metric m2: `http.request` (Statistic: Sum)
   - Math expression: `(m1/m2)*100`
   - Label: "Error Rate (%)"

---

### 2. Payment Metrics

**Location**: `/internal/services/payments/payment_service.go`

#### Payment Processing

| Metric Name | Type | Labels | Description |
|------------|------|--------|-------------|
| `payment.initiated` | Counter | payment_method, card_type, card_category, business_id | Payment attempts |
| `payment.success` | Counter | payment_method, gateway, card_type, card_category, business_id | Successful payments |
| `payment.failed` | Counter | payment_method, card_type, card_category, business_id, reason | Failed payments |
| `payment.amount.value` | Counter (value) | payment_method, currency | Payment amounts (in paise) |
| `payment.duration` | Timer | payment_method, gateway, status | Payment processing time |

#### Card Tokenization

| Metric Name | Type | Labels | Description |
|------------|------|--------|-------------|
| `payment.tokenization.initiated` | Counter | card_type, card_category, business_id | Tokenization attempts |
| `payment.tokenization.success` | Counter | card_type, card_category, gateway, business_id | Successful tokenizations |
| `payment.tokenization.failed` | Counter | card_type, card_category, business_id | Failed tokenizations |
| `payment.tokenization.duration` | Timer | gateway, status | Tokenization processing time |

**How to Query in CloudWatch Console**:

1. **Payment Success Rate**:
   - Metric m1: `payment.success` (Statistic: Sum)
   - Metric m2: `payment.initiated` (Statistic: Sum)
   - Math expression: `(m1/m2)*100`
   - Label: "Success Rate (%)"

2. **Average Payment Amount** (in rupees):
   - Metric: `payment.amount.value` (Statistic: Average)
   - Dimensions: payment_method=card
   - Math expression: `m1/100` (convert paise to rupees)
   - Label: "Avg Amount (₹)"

3. **P95 Payment Processing Time**:
   - Metric: `payment.duration`
   - Dimensions: status=success
   - Statistic: p95
   - Period: 5 minutes

4. **Failures by Reason**:
   - Metric: `payment.failed` (Statistic: Sum)
   - Group by: reason dimension
   - View as: Pie chart or Stacked bar

---

### 3. Gateway Health Metrics

**Locations**:
- `/internal/adapters/external/payment/jiopay/client.go`
- `/internal/adapters/external/payment/easebuzz/client.go`

| Metric Name | Type | Labels | Description |
|------------|------|--------|-------------|
| `gateway.request` | Counter | gateway, operation | Gateway API calls |
| `gateway.response_time` | Timer | gateway, operation, status_code | Gateway response time |
| `gateway.error` | Counter | gateway, operation, error_type, [status_code] | Gateway errors |

**Operations**:
- `tokenize` - Card tokenization
- `cryptogram` - Cryptogram generation (JioPay)
- `payment` - Payment initiation

**Error Types**:
- `network_error` - Network/timeout errors
- `read_error` - Response reading errors
- `http_error` - HTTP 4xx/5xx errors

**How to Query in CloudWatch Console**:

1. **Gateway Success Rate**:
   - Metric m1: `gateway.request` (Statistic: Sum, Dimension: gateway=jiopay)
   - Metric m2: `gateway.error` (Statistic: Sum, Dimension: gateway=jiopay)
   - Math expression: `((m1-m2)/m1)*100`
   - Label: "Success Rate (%)"

2. **Gateway P95 Response Time**:
   - Metric: `gateway.response_time`
   - Dimensions: gateway=jiopay, operation=tokenize
   - Statistic: p95
   - Period: 5 minutes

3. **Gateway Errors by Type**:
   - Metric: `gateway.error` (Statistic: Sum)
   - Dimensions: gateway=easebuzz
   - Group by: error_type
   - View as: Stacked bar or Pie chart

4. **Timeout Detection** (response time > 30 seconds):
   - Create CloudWatch Alarm:
   - Metric: `gateway.response_time`
   - Dimension: gateway=jiopay
   - Statistic: Maximum
   - Threshold: > 30000 (milliseconds)
   - Period: 5 minutes

---

## Key Performance Indicators (KPIs)

### 1. Payment Success Rate (CRITICAL)
**Formula**: `(payment.success / payment.initiated) × 100`
**Target**: > 95%
**Alert**: < 90%

### 2. Gateway Health Score (CRITICAL)
**Formula**: `((gateway.request - gateway.error) / gateway.request) × 100`
**Target**: > 98%
**Alert**: < 95%

### 3. API P95 Latency (HIGH)
**Metric**: `p95(http.request.duration)`
**Target**: < 500ms
**Alert**: > 2000ms

### 4. Gateway P95 Response Time (HIGH)
**Metric**: `p95(gateway.response_time)`
**Target**: < 5000ms (5s)
**Alert**: > 30000ms (30s - timeout threshold)

### 5. Error Rate (MEDIUM)
**Formula**: `(http.errors.5xx / http.request) × 100`
**Target**: < 1%
**Alert**: > 5%

---

## CloudWatch Dashboards

### Recommended Dashboard Widgets

#### 1. Payment Overview Dashboard

**Widget 1: Payment Success Rate (Line Graph)**
```
Metric: payment.success, payment.initiated
Math: (m1/m2)*100
Period: 5 minutes
```

**Widget 2: Payment Volume (Stacked Area)**
```
Metrics:
- payment.success (GREEN)
- payment.failed (RED)
Period: 5 minutes
```

**Widget 3: Average Payment Amount (Line Graph)**
```
Metric: payment.amount.value
Statistic: Average
Math: m1/100 (convert paise to rupees)
Period: 1 hour
```

**Widget 4: Payment Failures by Reason (Pie Chart)**
```
Metric: payment.failed
Group By: reason
Period: 1 hour
```

#### 2. Gateway Health Dashboard

**Widget 1: Gateway Response Time P95 (Line Graph)**
```
Metrics:
- gateway.response_time (gateway=jiopay)
- gateway.response_time (gateway=easebuzz)
Statistic: p95
Period: 5 minutes
```

**Widget 2: Gateway Error Rate (Line Graph)**
```
Math: (SUM(gateway.error) / SUM(gateway.request)) * 100
By Gateway
Period: 5 minutes
```

**Widget 3: Gateway Errors by Type (Stacked Bar)**
```
Metric: gateway.error
Group By: error_type, gateway
Period: 15 minutes
```

#### 3. API Performance Dashboard

**Widget 1: API Request Count (Number)**
```
Metric: http.request
Statistic: Sum
Period: 5 minutes
```

**Widget 2: API Latency Percentiles (Line Graph)**
```
Metric: http.request.duration
Statistics: p50, p95, p99
Period: 5 minutes
```

**Widget 3: Error Rates (Stacked Area)**
```
Metrics:
- http.errors.4xx (YELLOW)
- http.errors.5xx (RED)
Period: 5 minutes
```

**Widget 4: Top Slow Endpoints (Bar Chart)**
```
Metric: http.request.duration
Statistic: p95
Group By: path
Period: 15 minutes
```

---

## CloudWatch Alarms

### Critical Alarms

#### 1. Payment Failure Spike
```
Metric: payment.failed
Statistic: Sum
Threshold: > 10 failures
Evaluation: 2 consecutive periods of 5 minutes
Action: Page on-call engineer
```

#### 2. Gateway Timeout
```
Metric: gateway.response_time
Statistic: p95
Threshold: > 30000ms (30s)
Evaluation: 2 consecutive periods of 5 minutes
Action: Switch to backup gateway + page engineer
```

#### 3. Payment Success Rate Drop
```
Metric: (payment.success / payment.initiated) * 100
Threshold: < 90%
Evaluation: 2 consecutive periods of 10 minutes
Action: Page on-call engineer
```

### High Priority Alarms

#### 4. API Error Rate High
```
Metric: http.errors.5xx / http.request
Threshold: > 5%
Evaluation: 3 consecutive periods of 5 minutes
Action: Notify engineering team
```

#### 5. Gateway Error Rate High
```
Metric: gateway.error / gateway.request
Threshold: > 10%
Evaluation: 2 consecutive periods of 5 minutes
Action: Notify engineering team + check gateway status
```

#### 6. API Latency High
```
Metric: http.request.duration
Statistic: p95
Threshold: > 2000ms
Evaluation: 3 consecutive periods of 5 minutes
Action: Notify engineering team
```

---

## Cost Estimation

### CloudWatch Pricing (as of 2025)

**Metrics**: $0.30 per metric/month
**API Calls**: $0.01 per 1,000 PutMetricData requests

### Current Implementation Cost

**Estimated Metrics**: ~50 unique metric/label combinations
**Batching**: 20 metrics/batch, flush every 30s
**API Calls**: ~100 requests/day = 3,000/month

**Monthly Cost**:
- Metrics: 50 × $0.30 = **$15.00**
- API calls: 3,000 × $0.01 / 1,000 = **$0.03**
- **Total**: ~**$15-20/month**

**Cost Optimization**:
1. ✅ Batching enabled (reduces API calls by ~95%)
2. ✅ 30s flush interval (balances freshness vs cost)
3. ⚠️ Limit high-cardinality labels (avoid business_id in some metrics)
4. ⚠️ Use metric filters for derived metrics instead of raw events

---

## Best Practices

### DO ✅

1. **Use descriptive metric names**: `payment.success` not `p.s`
2. **Use consistent naming**: `payment.duration`, `gateway.response_time` (not `payment_duration`, `gatewayResponseTime`)
3. **Use dot notation**: `payment.tokenization.success`
4. **Send values, not pre-aggregated data**: Let CloudWatch calculate percentiles
5. **Use low-cardinality labels**: `gateway`, `payment_method`, `status`
6. **Track both counts and durations**: Success count + duration give full picture

### DON'T ❌

1. **❌ Use high-cardinality labels**: `user_id`, `payment_id`, `transaction_id` (creates millions of metrics = $$$$)
2. **❌ Send PII**: Never include email, phone, names, card numbers
3. **❌ Pre-aggregate**: Don't calculate averages yourself, send raw values
4. **❌ Use histogram buckets**: CloudWatch doesn't support Prometheus-style histograms
5. **❌ Ignore errors**: Always track error metrics, even for "shouldn't happen" cases

### Label Guidelines

**Good Labels** (low cardinality):
- `gateway`: "jiopay", "easebuzz" (2 values)
- `payment_method`: "card", "netbanking", "upi" (3-5 values)
- `card_type`: "visa", "mastercard", "rupay", "amex" (5-10 values)
- `status`: "success", "failed", "pending" (3-5 values)

**Bad Labels** (high cardinality):
- `business_id`: Unique per business (1000s of values)
- `user_id`: Unique per user (100,000s of values)
- `transaction_id`: Unique per transaction (millions of values)
- `ip_address`: Unique per request

**Solution for Business-Level Metrics**: Use separate dashboards filtered by business_id, don't create metrics with business_id labels.

---

## Troubleshooting

### Metrics Not Appearing in CloudWatch

1. **Check configuration**:
   ```bash
   echo $CLOUDWATCH_ENABLED  # Should be "true"
   echo $CLOUDWATCH_REGION   # Should match your AWS region
   ```

2. **Check AWS credentials**:
   ```bash
   aws cloudwatch list-metrics --namespace "XCentro/Backend"
   ```

3. **Check application logs**:
   ```
   grep -i "cloudwatch" /var/log/xcentro/api.log
   grep -i "metrics" /var/log/xcentro/api.log
   ```

4. **Verify metrics provider initialization**:
   - Look for: "CloudWatch metrics provider created successfully"
   - If see: "Metrics provider disabled (using no-op)" - check env vars

### High CloudWatch Costs

1. **Check metric count**:
   ```bash
   aws cloudwatch list-metrics --namespace "XCentro/Backend" | jq '.Metrics | length'
   ```

2. **Identify high-cardinality metrics**:
   ```bash
   aws cloudwatch list-metrics --namespace "XCentro/Backend" | jq '.Metrics | group_by(.MetricName) | map({metric: .[0].MetricName, count: length}) | sort_by(.count) | reverse'
   ```

3. **Solutions**:
   - Remove high-cardinality labels (business_id, user_id)
   - Increase batch interval (30s → 60s)
   - Reduce metric granularity (combine similar metrics)

### Slow Performance / High Latency

1. **Check async submission**: Metrics should be sent via goroutines (non-blocking)
2. **Check batch size**: Default 20, increase to 50 if seeing delays
3. **Monitor buffer**: If buffer fills up frequently, increase batch size or reduce interval

---

## Next Steps (Future Implementation)

### Tier 2 Metrics (Week 2-3)

1. **Database Connection Pool**:
   ```go
   metrics.Gauge(ctx, "db.connections.active", float64(activeConns), nil)
   metrics.Gauge(ctx, "db.connections.idle", float64(idleConns), nil)
   ```

2. **Webhook Processing**:
   ```go
   metrics.Counter(ctx, "webhook.received", 1, metrics.MetricLabels{
       "type": "payment_status",
       "gateway": "jiopay",
   })
   ```

3. **Settlement Tracking**:
   ```go
   metrics.Counter(ctx, "settlement.received", 1, metrics.MetricLabels{
       "gateway": "jiopay",
   })
   ```

### Tier 3 Metrics (Month 2+)

1. **Payout Metrics** (in `/internal/services/payout/`)
2. **Voucher Workflow** (in `/internal/services/voucher/`)
3. **Cache Performance** (Redis hit/miss rates)
4. **Worker Job Metrics** (notification worker, etc.)

---

## References

- **Implementation Summary**: `/internal/adapters/providers/metrics/IMPLEMENTATION_SUMMARY.md`
- **Integration Example**: `/internal/adapters/providers/metrics/INTEGRATION_EXAMPLE.md`
- **CloudWatch Docs**: https://docs.aws.amazon.com/cloudwatch/
- **Metrics Interface**: `/internal/core/metrics/metrics.go`
- **CloudWatch Client**: `/internal/adapters/providers/metrics/cloudwatch_metrics.go`

---

## Quick Start Checklist

- [ ] Set `CLOUDWATCH_ENABLED=true` in `.env`
- [ ] Configure AWS credentials (IAM role or access keys)
- [ ] Set `CLOUDWATCH_REGION` to your AWS region
- [ ] Restart application
- [ ] Check logs for "CloudWatch metrics provider created successfully"
- [ ] Wait 2-3 minutes for first batch to flush
- [ ] View metrics in CloudWatch console under namespace "XCentro/Backend"
- [ ] Create dashboard with payment success rate, gateway response time
- [ ] Set up critical alarms (payment failures, gateway timeouts)
- [ ] Monitor costs in AWS billing dashboard

---

**Last Updated**: 2025-11-08
**Version**: 1.0
**Author**: XCentro Engineering Team

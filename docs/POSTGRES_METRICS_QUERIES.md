# PostgreSQL Metrics Queries

This document provides SQL queries to retrieve business metrics directly from PostgreSQL. Use these queries instead of CloudWatch metrics for cost-effective business intelligence.

## Why Use PostgreSQL Instead of CloudWatch?

**Cost Comparison:**
- CloudWatch: ~$0.30/metric/month (adds up quickly with many metrics)
- PostgreSQL: Free (data already stored, just query it)

**When to Use PostgreSQL:**
- **Business Metrics**: Payment counts, amounts, success rates, tokenization stats
- **Historical Analysis**: Trends over time, monthly reports, year-over-year comparisons
- **Complex Queries**: Joins, aggregations, filtering by multiple dimensions
- **Cost-Sensitive Scenarios**: Dashboards, reports, business intelligence

**When to Use CloudWatch:**
- **Performance Monitoring**: API latency, gateway response times, error rates
- **Real-Time Alerting**: System health, SLA violations, downtime detection
- **Infrastructure Metrics**: Server health, database connections, memory usage

---

## Payment Metrics

### 1. Payment Counts by Status

Get total number of payments grouped by status (success, failed, pending, etc.):

```sql
SELECT
    status,
    COUNT(*) as payment_count,
    ROUND(COUNT(*) * 100.0 / SUM(COUNT(*)) OVER (), 2) as percentage
FROM payment_table
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY status
ORDER BY payment_count DESC;
```

**Example Output:**
```
status     | payment_count | percentage
-----------+--------------+-----------
success    |           450 |     75.00
failed     |           120 |     20.00
pending    |            30 |      5.00
```

### 2. Payment Success Rate by Payment Method

Calculate success rate for each payment method (card, netbanking, UPI):

```sql
SELECT
    payment_method,
    COUNT(*) as total_payments,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful_payments,
    ROUND(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as success_rate_percent
FROM payment_table
WHERE created_at >= NOW() - INTERVAL '7 days'
GROUP BY payment_method
ORDER BY total_payments DESC;
```

**Example Output:**
```
payment_method | total_payments | successful_payments | success_rate_percent
--------------+----------------+--------------------+---------------------
card           |           2500 |                1875 |                75.00
netbanking     |           1200 |                 960 |                80.00
upi            |            800 |                 640 |                80.00
```

### 3. Total Payment Amount by Currency

Sum total payment amounts processed:

```sql
SELECT
    currency,
    SUM(amount) as total_amount_paise,
    ROUND(SUM(amount) / 100.0, 2) as total_amount_currency,
    COUNT(*) as payment_count
FROM payment_table
WHERE status = 'success'
    AND created_at >= NOW() - INTERVAL '30 days'
GROUP BY currency
ORDER BY total_amount_paise DESC;
```

**Example Output:**
```
currency | total_amount_paise | total_amount_currency | payment_count
---------+-------------------+----------------------+--------------
INR      |          125000000 |           1250000.00 |         3500
USD      |            5000000 |             50000.00 |          150
```

### 4. Payment Success Rate by Gateway

Compare performance across payment gateways (JioPay vs Easebuzz):

```sql
SELECT
    metadata->>'gateway_used' as gateway,
    COUNT(*) as total_payments,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful_payments,
    ROUND(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as success_rate_percent,
    SUM(CASE WHEN status = 'success' THEN amount ELSE 0 END) as total_successful_amount_paise
FROM payment_table
WHERE metadata ? 'gateway_used'
    AND created_at >= NOW() - INTERVAL '7 days'
GROUP BY metadata->>'gateway_used'
ORDER BY total_payments DESC;
```

**Example Output:**
```
gateway  | total_payments | successful_payments | success_rate_percent | total_successful_amount_paise
---------+----------------+--------------------+---------------------+------------------------------
jiopay   |           1800 |                1440 |                80.00 |                     72000000
easebuzz |            700 |                 490 |                70.00 |                     24500000
```

### 5. Hourly Payment Volume (Last 24 Hours)

Track payment volume by hour for capacity planning:

```sql
SELECT
    DATE_TRUNC('hour', created_at) as hour,
    COUNT(*) as payment_count,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful_payments,
    ROUND(SUM(amount) / 100.0, 2) as total_amount
FROM payment_table
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY DATE_TRUNC('hour', created_at)
ORDER BY hour DESC;
```

### 6. Top 10 Businesses by Payment Volume

Identify highest-value customers:

```sql
SELECT
    business_id,
    COUNT(*) as total_payments,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful_payments,
    ROUND(SUM(CASE WHEN status = 'success' THEN amount END) / 100.0, 2) as total_successful_amount,
    ROUND(SUM(fee_amount) / 100.0, 2) as total_fees_collected
FROM payment_table
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY business_id
ORDER BY total_successful_amount DESC
LIMIT 10;
```

---

## Tokenization Metrics

### 7. Card Tokenization Success Rate

Track tokenization performance by card type:

```sql
SELECT
    metadata->>'card_type' as card_type,
    metadata->>'card_category' as card_category,
    COUNT(*) as total_tokenizations,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful_tokenizations,
    ROUND(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as success_rate_percent
FROM payment_table
WHERE payment_type = 'tokenization'
    AND created_at >= NOW() - INTERVAL '7 days'
GROUP BY metadata->>'card_type', metadata->>'card_category'
ORDER BY total_tokenizations DESC;
```

**Example Output:**
```
card_type | card_category | total_tokenizations | successful_tokenizations | success_rate_percent
----------+--------------+--------------------+-------------------------+---------------------
visa      | credit       |                 450 |                     405 |                90.00
mastercard| debit        |                 300 |                     270 |                90.00
rupay     | credit       |                 150 |                     135 |                90.00
```

### 8. Daily Tokenization Trend

Track tokenization adoption over time:

```sql
SELECT
    DATE(created_at) as date,
    COUNT(*) as total_tokenizations,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful_tokenizations
FROM payment_table
WHERE payment_type = 'tokenization'
    AND created_at >= NOW() - INTERVAL '30 days'
GROUP BY DATE(created_at)
ORDER BY date DESC;
```

---

## Voucher Metrics

### 9. Voucher Status Distribution

Get overview of voucher lifecycle stages:

```sql
SELECT
    status,
    COUNT(*) as voucher_count,
    ROUND(SUM(amount) / 100.0, 2) as total_amount,
    ROUND(COUNT(*) * 100.0 / SUM(COUNT(*)) OVER (), 2) as percentage
FROM voucher_table
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY status
ORDER BY voucher_count DESC;
```

### 10. End-to-End Payment Duration (Full Lifecycle)

**IMPORTANT**: This measures the **actual payment lifecycle** from user's perspective:
- **Start**: Payment initiated (`created_at`)
- **End**: Callback received from gateway (`updated_at`)

This includes: API call → Redirect → 3DS challenge → Bank processing → Gateway callback

```sql
SELECT
    payment_method,
    status,
    COUNT(*) as payment_count,

    -- Average time from initiation to callback
    ROUND(AVG(EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as avg_duration_seconds,

    -- Median (p50) - more representative than average
    ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as median_duration_seconds,

    -- p95 - 95% of payments complete within this time
    ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as p95_duration_seconds,

    -- p99 - slowest 1% (potential issues)
    ROUND(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as p99_duration_seconds,

    -- Min/Max for outlier detection
    ROUND(MIN(EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as min_duration_seconds,
    ROUND(MAX(EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as max_duration_seconds

FROM payment_table
WHERE created_at >= NOW() - INTERVAL '7 days'
    AND updated_at IS NOT NULL
    AND status IN ('success', 'failed')  -- Only completed payments
GROUP BY payment_method, status
ORDER BY payment_method, status;
```

**Example Output**:
```
payment_method | status  | avg_duration | median | p95    | p99    | min  | max
--------------+---------+-------------+--------+--------+--------+------+------
card           | success |       45.30 |  42.10 |  89.50 | 125.00 | 15.2 | 180.0
card           | failed  |       38.20 |  35.50 |  78.30 | 110.00 | 12.5 | 150.0
netbanking     | success |       62.40 |  58.30 | 120.00 | 180.00 | 20.0 | 300.0
```

**Use Cases**:
- **User experience monitoring**: How long do users wait?
- **Gateway SLA tracking**: Which gateway is faster?
- **Business hours analysis**: Do payments take longer during peak hours?
- **Anomaly detection**: Payments taking > 5 minutes might indicate issues

**⚠️ This is DIFFERENT from CloudWatch `payment.duration`**:
- **CloudWatch `payment.duration`**: Measures only API request-response time (~2-5 seconds)
  - Start: `ProcessPayment()` called
  - End: Redirect URL returned
  - Does NOT include user interaction, 3DS, bank processing, callback

- **PostgreSQL end-to-end duration**: Measures actual payment lifecycle (~30s-5min)
  - Start: Payment initiated
  - End: Gateway callback received
  - Includes entire user journey

### 11. Gateway Performance Comparison (End-to-End)

Compare actual gateway performance including all user interactions:

```sql
SELECT
    metadata->>'gateway_used' as gateway,
    payment_method,
    COUNT(*) as total_payments,

    -- Success rate
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful_payments,
    ROUND(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as success_rate_percent,

    -- Duration statistics (only for completed payments)
    ROUND(AVG(EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as avg_duration_seconds,
    ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as median_duration_seconds,
    ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as p95_duration_seconds

FROM payment_table
WHERE created_at >= NOW() - INTERVAL '7 days'
    AND updated_at IS NOT NULL
    AND metadata ? 'gateway_used'
    AND status IN ('success', 'failed')
GROUP BY metadata->>'gateway_used', payment_method
ORDER BY total_payments DESC;
```

**Example Output**:
```
gateway  | payment_method | total | successful | success_rate | avg_duration | median | p95
---------+---------------+-------+-----------+-------------+-------------+--------+------
jiopay   | card          |  1200 |       960 |       80.00 |       42.30 |  38.50 | 85.0
easebuzz | card          |   800 |       640 |       80.00 |       48.20 |  45.10 | 95.0
jiopay   | netbanking    |   500 |       425 |       85.00 |       58.40 |  55.20 | 110.0
```

**Insights**:
- JioPay is faster for card payments (42s avg vs 48s for Easebuzz)
- But both have similar success rates (~80%)
- Netbanking takes longer regardless of gateway (bank-side processing)

### 12. Payment Duration Over Time (Hourly Trend)

Identify peak hours where payments take longer:

```sql
SELECT
    DATE_TRUNC('hour', created_at) as hour,
    payment_method,
    COUNT(*) as payment_count,

    -- Duration statistics
    ROUND(AVG(EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as avg_duration_seconds,
    ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (updated_at - created_at))), 2) as p95_duration_seconds,

    -- Success rate during that hour
    ROUND(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as success_rate_percent

FROM payment_table
WHERE created_at >= NOW() - INTERVAL '24 hours'
    AND updated_at IS NOT NULL
    AND status IN ('success', 'failed')
GROUP BY DATE_TRUNC('hour', created_at), payment_method
ORDER BY hour DESC, payment_method;
```

**Use Case**: Detect if payment processing slows down during peak hours (gateway overload, bank issues)

### 13. Slow Payment Detection (Potential Issues)

Find payments that took unusually long (potential failures, user abandonment):

```sql
SELECT
    id as payment_id,
    business_id,
    payment_method,
    metadata->>'gateway_used' as gateway,
    status,
    amount,
    created_at,
    updated_at,
    ROUND(EXTRACT(EPOCH FROM (updated_at - created_at)), 2) as duration_seconds,

    -- Categorize slowness
    CASE
        WHEN EXTRACT(EPOCH FROM (updated_at - created_at)) > 300 THEN 'Very Slow (>5min)'
        WHEN EXTRACT(EPOCH FROM (updated_at - created_at)) > 120 THEN 'Slow (>2min)'
        WHEN EXTRACT(EPOCH FROM (updated_at - created_at)) > 60 THEN 'Moderate (>1min)'
        ELSE 'Normal'
    END as speed_category

FROM payment_table
WHERE created_at >= NOW() - INTERVAL '24 hours'
    AND updated_at IS NOT NULL
    AND status IN ('success', 'failed')
    AND EXTRACT(EPOCH FROM (updated_at - created_at)) > 120  -- Show only >2 minutes
ORDER BY duration_seconds DESC
LIMIT 50;
```

**Action Items**:
- Payments >5min: Investigate gateway issues, contact support
- Payments >2min: Check if specific card types/banks are slow
- Pattern detection: If many slow payments at same time → gateway downtime

---

## Creating Materialized Views

For frequently-run queries, create materialized views for better performance:

```sql
-- Create materialized view for daily payment summary
CREATE MATERIALIZED VIEW daily_payment_summary AS
SELECT
    DATE(created_at) as date,
    payment_method,
    status,
    COUNT(*) as payment_count,
    SUM(amount) as total_amount_paise,
    SUM(fee_amount) as total_fee_paise,
    ROUND(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as success_rate
FROM payment_table
WHERE created_at >= NOW() - INTERVAL '90 days'
GROUP BY DATE(created_at), payment_method, status;

-- Create index for faster queries
CREATE INDEX idx_daily_payment_summary_date ON daily_payment_summary(date);

-- Refresh materialized view (run this periodically via cron job)
REFRESH MATERIALIZED VIEW daily_payment_summary;

-- Query the materialized view (much faster)
SELECT * FROM daily_payment_summary
WHERE date >= CURRENT_DATE - INTERVAL '7 days'
ORDER BY date DESC, payment_count DESC;
```

---

## Dashboard Query Functions

Create PostgreSQL functions for common dashboard queries:

```sql
-- Function: Get payment summary for a date range
CREATE OR REPLACE FUNCTION get_payment_summary(
    start_date TIMESTAMP,
    end_date TIMESTAMP
)
RETURNS TABLE(
    total_payments BIGINT,
    successful_payments BIGINT,
    failed_payments BIGINT,
    pending_payments BIGINT,
    success_rate NUMERIC,
    total_amount_paise BIGINT,
    total_fee_paise BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        COUNT(*)::BIGINT as total_payments,
        SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END)::BIGINT as successful_payments,
        SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END)::BIGINT as failed_payments,
        SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END)::BIGINT as pending_payments,
        ROUND(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as success_rate,
        SUM(amount)::BIGINT as total_amount_paise,
        SUM(fee_amount)::BIGINT as total_fee_paise
    FROM payment_table
    WHERE created_at >= start_date AND created_at < end_date;
END;
$$ LANGUAGE plpgsql;

-- Usage:
SELECT * FROM get_payment_summary(NOW() - INTERVAL '24 hours', NOW());
```

---

## Performance Tips

### 1. Create Indexes for Common Queries

```sql
-- Index on created_at for time-based queries
CREATE INDEX idx_payment_created_at ON payment_table(created_at);

-- Index on status for filtering
CREATE INDEX idx_payment_status ON payment_table(status);

-- Index on business_id for business-level queries
CREATE INDEX idx_payment_business_id ON payment_table(business_id);

-- Composite index for common filters
CREATE INDEX idx_payment_status_created ON payment_table(status, created_at);

-- JSONB index for metadata queries
CREATE INDEX idx_payment_metadata_gateway ON payment_table USING gin ((metadata->'gateway_used'));
```

### 2. Query Optimization

- **Always use time ranges**: Avoid full table scans by filtering on `created_at`
- **Use indexes**: Design queries that leverage existing indexes
- **Limit results**: Use `LIMIT` for large result sets
- **Use EXPLAIN ANALYZE**: Check query performance before running in production

```sql
-- Check query execution plan
EXPLAIN ANALYZE
SELECT status, COUNT(*)
FROM payment_table
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY status;
```

### 3. Scheduled Refresh

For materialized views, set up automated refresh:

```bash
# Add to crontab (refresh every hour at minute 0)
0 * * * * psql -U xcentro_app -d xcentro -c "REFRESH MATERIALIZED VIEW daily_payment_summary;"
```

---

## Integration with Application

### Option 1: Direct Database Queries

Add a metrics repository for direct PostgreSQL queries:

```go
// internal/adapters/database/postgres/metrics/metrics_repository.go
type MetricsRepository struct {
    db client.DB
}

func (r *MetricsRepository) GetPaymentSummary(ctx context.Context, startDate, endDate time.Time) (*PaymentSummary, error) {
    query := `
        SELECT
            COUNT(*) as total_payments,
            SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful_payments,
            ROUND(SUM(CASE WHEN status = 'success' THEN amount END) / 100.0, 2) as total_amount
        FROM payment_table
        WHERE created_at >= $1 AND created_at < $2
    `
    var summary PaymentSummary
    err := r.db.QueryRow(ctx, query, startDate, endDate).Scan(&summary.TotalPayments, &summary.SuccessfulPayments, &summary.TotalAmount)
    return &summary, err
}
```

### Option 2: REST API Endpoint

Expose metrics through API endpoints:

```go
// GET /v1/metrics/payments?start=2025-01-01&end=2025-01-31
func (h *MetricsHandler) GetPaymentMetrics(c echo.Context) error {
    start := c.QueryParam("start")
    end := c.QueryParam("end")

    summary, err := h.metricsRepo.GetPaymentSummary(ctx, startDate, endDate)
    if err != nil {
        return c.JSON(500, map[string]string{"error": "Failed to fetch metrics"})
    }

    return c.JSON(200, summary)
}
```

---

## Cost-Benefit Analysis

### CloudWatch Metrics (Removed)

**Business metrics we removed from CloudWatch:**
- `payment.initiated` - Counter
- `payment.success` - Counter
- `payment.failed` - Counter
- `payment.amount.value` - Counter
- `payment.tokenization.initiated` - Counter
- `payment.tokenization.success` - Counter
- `payment.tokenization.failed` - Counter
- `gateway.request` - Counter

**Cost savings**: ~8 metrics × $0.30/month = **$2.40/month per business**

With 100 businesses: **$240/month = $2,880/year saved**

### PostgreSQL Queries (Added)

**Cost**: $0 (data already stored, just querying it)

**Performance**: Sub-second response times with proper indexes

**Flexibility**: More powerful queries, joins, complex aggregations

---

## Recommended Dashboard Setup

### 1. Real-Time Operational Dashboard (CloudWatch)
- Gateway response time (p50, p95, p99)
- API endpoint latency
- Error rates (5xx errors)
- Webhook signature failures

### 2. Business Intelligence Dashboard (PostgreSQL)
- Payment volume trends
- Success rates by gateway
- Revenue by business
- Top customers
- Tokenization adoption

This hybrid approach gives you **best of both worlds**: Real-time performance monitoring where it matters + comprehensive business analytics at zero extra cost.

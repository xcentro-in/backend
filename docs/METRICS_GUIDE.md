# XCentro Metrics Guide

## Overview

This guide documents the **cost-optimized** metrics implementation for XCentro backend. We use a hybrid approach:

- **CloudWatch**: Essential performance metrics (API latency, gateway response times, infrastructure errors)
- **PostgreSQL**: Business metrics (payment counts, success rates, amounts, trends)

**Why this approach?**
- ✅ **70% cost reduction**: From ~$15/month to ~$5/month per environment
- ✅ **Better analytics**: PostgreSQL provides more powerful queries for business metrics
- ✅ **Real-time monitoring**: CloudWatch tracks what PostgreSQL cannot (API latency, external gateway performance)
- ✅ **Zero compromise**: All essential observability retained

**See also**: `/docs/POSTGRES_METRICS_QUERIES.md` for comprehensive PostgreSQL alternatives

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

#### Payment Processing (Performance Metrics Only)

**⚠️ Cost-Optimized Approach**: Business metrics (payment counts, amounts, success rates) should be queried from PostgreSQL instead of CloudWatch. See `/docs/POSTGRES_METRICS_QUERIES.md` for SQL examples.

| Metric Name | Type | Labels | Description |
|------------|------|--------|-------------|
| `payment.duration` | Timer | payment_method, gateway, status | **API orchestration latency** (initiation to redirect URL) |
| `payment.tokenization.duration` | Timer | gateway, status | **Tokenization API latency** |

**⚠️ IMPORTANT: What `payment.duration` Measures**

This metric measures **ONLY the API orchestration layer** (~2-5 seconds):
```
START: ProcessPayment() called
  ↓
  Calculate fees
  Create payment record
  Call gateway API
  Receive redirect URL
  ↓
END: Return redirect URL to frontend
```

**This does NOT include:**
- ❌ User redirecting to payment page
- ❌ User completing 3DS challenge
- ❌ Bank processing payment
- ❌ Gateway sending callback
- ❌ Callback handler receiving result

**For end-to-end payment duration (full lifecycle)**, use PostgreSQL query:
```sql
-- Full payment lifecycle: initiation to callback
SELECT AVG(EXTRACT(EPOCH FROM (updated_at - created_at))) as avg_duration_seconds
FROM payment_table
WHERE status IN ('success', 'failed');
```

See Query #10 in `/docs/POSTGRES_METRICS_QUERIES.md` for complete end-to-end duration tracking.

**Why only duration metrics?**
- **API performance monitoring**: Detect if our backend is slowing down
- **Gateway API latency**: Measure external API response times
- **Cannot be measured from PostgreSQL**: Database timestamps don't capture in-memory processing time
- **Essential for alerting**: Detect infrastructure/code performance issues

**Business metrics moved to PostgreSQL** (payment counts, success rates, amounts):
- **Reason**: Data already stored in database, querying PostgreSQL is free
- **Cost savings**: ~$2.40/month per business (~$240/month for 100 businesses)
- **Better analytics**: More powerful queries, joins, historical analysis
- **See**: `/docs/POSTGRES_METRICS_QUERIES.md` for comprehensive SQL examples

**How to Query in CloudWatch Console**:

1. **P95 Payment Processing Time**:
   - Metric: `payment.duration`
   - Dimensions: status=success, payment_method=card
   - Statistic: p95
   - Period: 5 minutes
   - **Alert Threshold**: p95 > 5000ms (5 seconds)

2. **P99 Tokenization Time**:
   - Metric: `payment.tokenization.duration`
   - Dimensions: gateway=jiopay, status=success
   - Statistic: p99
   - Period: 5 minutes
   - **Alert Threshold**: p99 > 10000ms (10 seconds)

3. **Average Processing Time by Gateway**:
   - Metric: `payment.duration`
   - Group by: gateway dimension
   - Statistic: Average
   - Compare: JioPay vs Easebuzz performance

---

### 3. Gateway Health Metrics (Essential Only)

**Locations**:
- `/internal/adapters/external/payment/jiopay/client.go`
- `/internal/adapters/external/payment/easebuzz/client.go`

**⚠️ Cost-Optimized**: Removed `gateway.request` counter (can query from payment_table in PostgreSQL). Keeping only metrics that measure real-time performance.

| Metric Name | Type | Labels | Description |
|------------|------|--------|-------------|
| `gateway.response_time` | Timer | gateway, operation, status_code | Gateway API response time |
| `gateway.error` | Counter | gateway, operation, error_type, [status_code] | Gateway errors (network/timeout only) |

**Operations**:
- `tokenize` - Card tokenization
- `cryptogram` - Cryptogram generation (JioPay)
- `payment` - Payment initiation

**Error Types** (Infrastructure errors only):
- `network_error` - Network/timeout errors (cannot reach gateway)
- `read_error` - Response reading errors
- `http_error` - HTTP 4xx/5xx errors from gateway

**Why these metrics are essential:**
- **gateway.response_time**: Measures external API latency, cannot be measured from PostgreSQL
- **gateway.error**: Real-time alerting on infrastructure failures (different from business logic failures)
- **SLA monitoring**: Track if gateways meet their response time SLAs
- **Vendor comparison**: Compare JioPay vs Easebuzz performance objectively

**How to Query in CloudWatch Console**:

1. **Gateway P95 Response Time** (SLA Monitoring):
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

### 4. Webhook Metrics

**Location**: `/internal/handlers/http/payment/webhook_handler.go`

| Metric Name | Type | Labels | Description |
|------------|------|--------|-------------|
| `webhook.received` | Counter | gateway, type | Webhook requests received from gateway |
| `webhook.signature_failed` | Counter | gateway, type | Webhook signature verification failures |
| `webhook.duplicate` | Counter | gateway, type | Duplicate webhooks (idempotency check) |
| `webhook.processing_duration` | Timer | gateway, type, status | Time to process webhook |

**Webhook Types**:
- `payment_status` - Payment status update webhooks
- `settlement` - Settlement/payout webhooks

**Why This Matters**:
- Webhooks are how gateways notify you of payment status changes
- Webhook failures = missed payment updates = reconciliation issues
- High duplicate rate = gateway retry issues

**How to Query in CloudWatch Console**:

1. **Webhook Success Rate**:
   - Metric m1: `webhook.received` (Statistic: Sum, Dimension: gateway=jiopay, type=payment_status)
   - Metric m2: `webhook.signature_failed` (Statistic: Sum, same dimensions)
   - Math expression: `((m1-m2)/m1)*100`
   - Label: "Webhook Success Rate (%)"

2. **Duplicate Webhook Rate**:
   - Metric m1: `webhook.received` (Statistic: Sum)
   - Metric m2: `webhook.duplicate` (Statistic: Sum)
   - Math expression: `(m2/m1)*100`
   - Label: "Duplicate Rate (%)"
   - Note: Some duplicates are normal (gateway retries)

3. **Webhook Processing Time**:
   - Metric: `webhook.processing_duration`
   - Dimensions: gateway=jiopay, type=payment_status
   - Statistic: p95
   - Target: < 1000ms (1 second)

4. **Signature Failures** (Critical - indicates security issue):
   - Metric: `webhook.signature_failed` (Statistic: Sum)
   - Group by: gateway
   - Alert: > 5 failures in 10 minutes
   - Action: Investigate immediately (could be attack or config issue)

---

### 5. Third-Party API Metrics

**Locations**:
- `/internal/adapters/external/payout/castler/castler_client.go` (Castler)
- `/internal/adapters/providers/authbridge/client.go` (Authbridge)

**⚠️ Why These Metrics Are Critical**:

Unlike payment gateways (which are async), Castler and Authbridge are **synchronous third-party APIs** called during:
- **Onboarding flows**: User/vendor verification, bank account verification
- **Payout operations**: Beneficiary onboarding, transfer creation

**Cannot track failures in PostgreSQL** because:
- ❌ API failures happen BEFORE database records are created
- ❌ Network timeouts leave no trace in database
- ❌ Encryption/decryption errors occur during API call

**Real-time monitoring is essential** for:
- ✅ Detecting third-party API outages immediately
- ✅ Alerting on high error rates during onboarding
- ✅ Measuring third-party API SLAs
- ✅ Identifying slow verification processes

| Metric Name | Type | Labels | Description |
|------------|------|--------|-------------|
| `third_party.response_time` | Timer | provider, operation, status_code | Third-party API response time |
| `third_party.error` | Counter | provider, operation, error_type, [status_code] | Third-party API errors |

#### Castler Operations

**Provider**: `castler`

**Operations**:
- `onboard_beneficiary` - Onboard payee to Castler for payouts
- `create_transfer` - Initiate payout transfer to vendor
- `verify_bank` - Penny drop bank account verification (if used)

**Error Types**:
- `marshal_error` - Request JSON marshaling failure
- `encryption_error` - AES-256-CBC encryption failure
- `network_error` - HTTP request/timeout failure
- `read_error` - Response reading failure
- `decryption_error` - Response decryption failure
- `http_error` - HTTP 4xx/5xx from Castler API
- `parse_error` - Response JSON parsing failure

**Example CloudWatch Queries**:

1. **Castler API P95 Response Time**:
   - Metric: `third_party.response_time`
   - Dimensions: provider=castler, operation=create_transfer
   - Statistic: p95
   - Alert: > 10000ms (10 seconds)

2. **Castler Onboarding Errors**:
   - Metric: `third_party.error`
   - Dimensions: provider=castler, operation=onboard_beneficiary
   - Statistic: Sum
   - Group by: error_type
   - Alert: > 5 errors in 15 minutes

3. **Castler Error Rate**:
   - Metric m1: `third_party.response_time` (Count by provider=castler)
   - Metric m2: `third_party.error` (Sum by provider=castler)
   - Math: `(m2/m1)*100`
   - Label: "Castler Error Rate (%)"

#### Authbridge Operations

**Provider**: `authbridge`

**Operations**:
- `verify_pan` - Basic PAN verification
- `verify_pan_comprehensive` - Comprehensive PAN details
- `verify_pan_aadhaar_link` - PAN-Aadhaar linkage check
- `verify_aadhaar` - Aadhaar verification via DigiLocker
- `verify_gstin` - GST registration verification
- `verify_cin` - Company identification number verification
- `verify_bank` - Bank account verification (penny drop)

**Error Types**: (Same as Castler)
- `marshal_error`, `encryption_error`, `network_error`, `read_error`
- `decryption_error`, `http_error`, `parse_error`

**Example CloudWatch Queries**:

1. **Authbridge KYC Verification Time**:
   - Metric: `third_party.response_time`
   - Dimensions: provider=authbridge, operation=verify_pan
   - Statistic: Average
   - Target: < 5000ms (5 seconds)

2. **Authbridge Verification Failures by Document Type**:
   - Metric: `third_party.error`
   - Dimensions: provider=authbridge
   - Statistic: Sum
   - Group by: operation
   - View as: Bar chart

3. **Bank Verification Success Rate**:
   - Metric m1: Total calls (Count of `third_party.response_time` where operation=verify_bank)
   - Metric m2: Errors (Sum of `third_party.error` where operation=verify_bank)
   - Math: `((m1-m2)/m1)*100`
   - Label: "Bank Verification Success Rate (%)"

4. **Encryption/Decryption Errors** (Critical - indicates config issue):
   - Metric: `third_party.error`
   - Dimensions: provider=authbridge
   - Filter: error_type IN (encryption_error, decryption_error)
   - Statistic: Sum
   - Alert: > 1 error (immediate investigation needed)

**Why Track Both Providers?**

1. **Onboarding Impact**: Failures block user onboarding → revenue loss
2. **SLA Monitoring**: Track if vendors meet their response time SLAs
3. **Vendor Comparison**: Evaluate provider reliability
4. **Early Detection**: Alert on outages before customers complain
5. **Cost Analysis**: Measure API call volume for cost forecasting

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

### 6. Third-Party API Success Rate (HIGH)
**Formula**: `((third_party.calls - third_party.error) / third_party.calls) × 100`
**Target**: > 95%
**Alert**: < 90%
**Why Critical**: Failures block onboarding flows, impacting revenue

### 7. Third-Party API P95 Response Time (MEDIUM)
**Metric**: `p95(third_party.response_time)`
**Target**: < 5000ms (5s) for Authbridge, < 10000ms (10s) for Castler
**Alert**: > 30000ms (30s - timeout threshold)
**Why Monitor**: Slow verification = poor user experience during onboarding

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

#### 4. Third-Party API Monitoring Dashboard

**Widget 1: Third-Party API Response Time Comparison (Line Graph)**
```
Metrics:
- third_party.response_time (provider=castler, operation=onboard_beneficiary)
- third_party.response_time (provider=castler, operation=create_transfer)
- third_party.response_time (provider=authbridge, operation=verify_pan)
- third_party.response_time (provider=authbridge, operation=verify_aadhaar)
Statistic: p95
Period: 5 minutes
```

**Widget 2: Third-Party Error Rate (Line Graph)**
```
Metric m1: third_party.response_time (Count by provider)
Metric m2: third_party.error (Sum by provider)
Math: (m2/m1)*100
Label: "Error Rate (%) by Provider"
Period: 5 minutes
```

**Widget 3: Third-Party Errors by Type (Stacked Bar)**
```
Metric: third_party.error
Group By: error_type, provider
Period: 15 minutes
```

**Widget 4: Authbridge Operations Breakdown (Pie Chart)**
```
Metric: third_party.response_time (provider=authbridge)
Statistic: Count
Group By: operation
Period: 1 hour
Note: Shows which verification types are most used
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

#### 7. Third-Party API Failure Spike
```
Metric: third_party.error
Statistic: Sum
Threshold: > 10 errors
Evaluation: 2 consecutive periods of 5 minutes
Action: Notify engineering team + check provider status
Note: Failures block onboarding flows
```

#### 8. Third-Party API Timeout
```
Metric: third_party.response_time
Statistic: p95
Threshold: > 30000ms (30s)
Evaluation: 2 consecutive periods of 5 minutes
Action: Notify engineering team
Note: May indicate provider outage or network issue
```

---

## Cost Estimation

### CloudWatch Pricing (as of 2025)

**Metrics**: $0.30 per metric/month
**API Calls**: $0.01 per 1,000 PutMetricData requests

### Optimized Implementation Cost

**Before Optimization**: ~50 unique metric/label combinations = **$15/month**
**After Optimization**: ~25-30 unique metric/label combinations = **$8/month**

**Cost Breakdown**:
```
Essential CloudWatch Metrics (Kept):
- http.request.duration          ~3 metrics (GET/POST/PUT × multiple paths)
- http.errors.5xx                ~2 metrics
- payment.duration               ~4 metrics (card/netbanking/UPI × success/failed)
- payment.tokenization.duration  ~2 metrics (success/failed)
- gateway.response_time          ~4 metrics (jiopay/easebuzz × tokenize/payment)
- gateway.error                  ~3 metrics (network/http/read errors)
- webhook.* metrics              ~3 metrics (received, failed, duplicate)
- third_party.response_time      ~7 metrics (castler × 3 ops + authbridge × 4 ops)
- third_party.error              ~4 metrics (castler/authbridge × common errors)
----------------------------------------
Total: ~25-30 metrics × $0.30 = $7.50-9.00/month
API calls: ~150 requests/day = $0.05/month
----------------------------------------
TOTAL: ~$8/month per environment
```

**Business Metrics (Moved to PostgreSQL - $0 cost)**:
- ❌ payment.initiated, payment.success, payment.failed
- ❌ payment.amount.value
- ❌ payment.tokenization.initiated, payment.tokenization.success, payment.tokenization.failed
- ❌ gateway.request

**Savings**: $7/month per environment (~50% reduction)

**What Changed:**
1. ✅ **Removed business counters**: Payment counts, amounts, success rates now queried from PostgreSQL
2. ✅ **Kept performance timers**: API latency, gateway response times (cannot measure from DB)
3. ✅ **Kept infrastructure errors**: Real-time alerting on network/timeout failures
4. ✅ **Added third-party monitoring**: Castler & Authbridge API tracking (+$3.30/month)
5. ✅ **Worth the cost**: Third-party metrics catch failures before database records are created

**See**: `/docs/POSTGRES_METRICS_QUERIES.md` for PostgreSQL alternatives to removed metrics

---

## Best Practices

### DO ✅

1. **Use descriptive metric names**: `payment.duration` not `p.d`
2. **Use consistent naming**: `payment.duration`, `gateway.response_time` (not `payment_duration`, `gatewayResponseTime`)
3. **Use dot notation**: `payment.tokenization.duration`
4. **Send values, not pre-aggregated data**: Let CloudWatch calculate percentiles
5. **Use low-cardinality labels**: `gateway`, `payment_method`, `status`
6. **Prefer PostgreSQL for business metrics**: Payment counts, amounts, success rates → SQL queries (free)
7. **Use CloudWatch for performance metrics**: API latency, external service response times (cannot measure from DB)

### DON'T ❌

1. **❌ Use high-cardinality labels**: `user_id`, `payment_id`, `transaction_id` (creates millions of metrics = $$$$)
2. **❌ Send PII**: Never include email, phone, names, card numbers
3. **❌ Pre-aggregate**: Don't calculate averages yourself, send raw values
4. **❌ Use histogram buckets**: CloudWatch doesn't support Prometheus-style histograms
5. **❌ Ignore errors**: Always track error metrics, even for "shouldn't happen" cases
6. **❌ Duplicate PostgreSQL data**: Don't track business metrics in CloudWatch that can be queried from database
7. **❌ Track counts of database operations**: Payment counts, user signups, etc. → Use SQL queries instead

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

1. ✅ **Third-Party API Metrics** - Implemented (Castler, Authbridge)
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

**Last Updated**: 2025-12-09
**Version**: 1.1
**Author**: XCentro Engineering Team

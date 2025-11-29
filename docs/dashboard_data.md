# 📊 Customer Home Page Metrics - Categorized by Query Complexity

Based on actual PostgreSQL schema exploration, here are metrics organized by implementation difficulty:

---
## 🟢 EASY TO GET

Single table, indexed queries - sub-100ms response

Financial Snapshot

1. Total Payments This Month ✅
```SQL
SELECT COUNT(*), SUM(amount), SUM(fee_amount)
FROM xcentro.payments
WHERE business_id = $1
AND created_at >= date_trunc('month', CURRENT_DATE)
AND status = 'success';
- Index: Add idx_payments_business_created on (business_id, created_at DESC, status)
```
2. Pending Settlement Amount ✅
```sql
SELECT SUM(settlement_amount), COUNT(*)
FROM xcentro.payments
WHERE business_id = $1
AND settlement_status = 'pending';
- Index: idx_payments_settlement_amounts (already exists!)
```
3. Settled But Not Paid Out ✅
```sql
SELECT SUM(net_settlement_amount)
FROM xcentro.payments
WHERE business_id = $1
AND settlement_status = 'settled'
AND payment_id NOT IN (
    SELECT DISTINCT payment_id
    FROM xcentro.payout_transfer_requests
    WHERE status IN ('completed')
);
- Index: idx_payments_settlement_amounts + idx_payout_transfers_payment_status
```
4. Active Payees Count ✅
```sql
SELECT COUNT(*)
FROM xcentro.business_payee_relation
WHERE business_id = $1 AND is_active = TRUE;
- Index: idx_business_payee_relation_active (already exists!)
```
5. Stored Payment Methods ✅
```sql
SELECT
COUNT(*) FILTER (WHERE is_active = TRUE) as active_cards,
COUNT(*) FILTER (WHERE is_failed_tokenization = TRUE) as failed_cards
FROM xcentro.stored_cards
WHERE business_id = $1;
- Index: Add idx_stored_cards_business_active on (business_id, is_active)
```
6. Pending Approvals (Action Items) ✅
```sql
SELECT COUNT(*), SUM(pv.net_amount)
FROM xcentro.voucher_approvals va
JOIN xcentro.payment_vouchers pv ON va.voucher_id = pv.voucher_id
WHERE va.approver_user_id = $1
AND va.action = 'pending';
- Index: idx_voucher_approvals_approver (already exists!)
```
7. Today's Payment Activity ✅
```sql
SELECT
status,
COUNT(*) as count,
SUM(amount) as total
FROM xcentro.payments
WHERE business_id = $1
AND created_at >= CURRENT_DATE
GROUP BY status;
```
8. Payout Status Summary ✅
```sql
SELECT
status,
COUNT(*) as count,
SUM(amount) as total_amount
FROM xcentro.payout_transfer_requests ptr
JOIN xcentro.payment_vouchers pv ON ptr.voucher_id = pv.voucher_id
WHERE pv.business_id = $1
AND ptr.is_deleted = FALSE
GROUP BY status;
- Index: Add composite index on vouchers + transfers
```
9. Failed Payments (Last 7 Days) ✅
```sql
SELECT COUNT(*), SUM(amount)
FROM xcentro.payments
WHERE business_id = $1
AND status = 'failed'
AND created_at >= CURRENT_DATE - INTERVAL '7 days';
```
10. Batch Upload Status ✅
```sql
SELECT
status,
COUNT(*) as batch_count,
SUM(total_amount) as total
FROM xcentro.batch_payments
WHERE business_id = $1
AND created_at >= CURRENT_DATE - INTERVAL '30 days'
GROUP BY status;
- Index: idx_batch_payments_business_id, idx_batch_payments_status (already exist!)
```
---
## 🟡 MODERATE TO FETCH

2-3 table joins, needs materialized views or caching (1-5 second queries)

Analytics & Trends

1. Payment Success Rate (30 Days) 🔶
```sql
SELECT
status,
COUNT(*) as count,
ROUND(COUNT(*) * 100.0 / SUM(COUNT(*)) OVER(), 2) as percentage,
SUM(amount) as total_amount
FROM xcentro.payments
WHERE business_id = $1
AND created_at >= CURRENT_DATE - INTERVAL '30 days'
GROUP BY status;
- Solution: Materialized view refreshed hourly via pg_cron
- Table: analytics.mv_payment_stats_daily
```
2. Top 5 Vendors by Payment Amount 🔶
```sql
SELECT
bpr.display_name,
pm.payee_type,
COUNT(DISTINCT pv.voucher_id) as voucher_count,
SUM(pv.net_amount) as total_paid,
MAX(pv.voucher_date) as last_payment_date
FROM xcentro.payment_vouchers pv
JOIN xcentro.business_payee_relation bpr
ON pv.business_id = bpr.business_id
AND pv.payee_id = bpr.payee_id
AND pv.account_id = bpr.account_id
JOIN xcentro.payee_master pm ON bpr.payee_id = pm.payee_id
WHERE pv.business_id = $1
AND pv.status IN ('posted', 'synced')
GROUP BY bpr.display_name, pm.payee_type
ORDER BY total_paid DESC
LIMIT 5;
- Solution: Cache in Redis (TTL: 1 hour)
```
3. Monthly Spend Trend (Last 6 Months) 🔶
```sql
SELECT
date_trunc('month', created_at) as month,
payment_method,
COUNT(*) as txn_count,
SUM(amount) as total_spend,
SUM(fee_amount + convenience_fee + convenience_fee_gst) as total_fees
FROM xcentro.payments
WHERE business_id = $1
AND status = 'success'
AND created_at >= CURRENT_DATE - INTERVAL '6 months'
GROUP BY month, payment_method
ORDER BY month DESC;
- Solution: Materialized view mv_monthly_payment_stats
```
4. Settlement Timeline Analysis 🔶
```sql
SELECT
AVG(EXTRACT(EPOCH FROM (settled_at - created_at))/3600) as avg_settlement_hours,
PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY (settled_at - created_at)) as median_settlement_time,
MIN(settled_at - created_at) as fastest,
MAX(settled_at - created_at) as slowest
FROM xcentro.payments
WHERE business_id = $1
AND settlement_status = 'settled'
AND settled_at IS NOT NULL;
- Solution: Weekly materialized view
```
5. Payment Method Breakdown 🔶
```sql
SELECT
payment_method,
gateway_used,
COUNT(*) as txn_count,
SUM(amount) as total_amount,
AVG(fee_amount) as avg_fee,
SUM(mdr_amount) as total_mdr
FROM xcentro.payments
WHERE business_id = $1
AND status = 'success'
GROUP BY payment_method, gateway_used
ORDER BY total_amount DESC;
- Solution: Cache in Go with 15-min TTL
```
6. Voucher Status Distribution 🔶
```sql
SELECT
status,
COUNT(*) as voucher_count,
SUM(net_amount) as total_amount,
SUM(tds_amount) as total_tds,
SUM(gst_amount) as total_gst
FROM xcentro.payment_vouchers
WHERE business_id = $1
GROUP BY status;
- Solution: Real-time query (fast with proper indexes)
```
7. Approval Workflow Health 🔶
```sql
SELECT
va.action,
COUNT(*) as count,
AVG(EXTRACT(EPOCH FROM (COALESCE(CURRENT_TIMESTAMP, va.created_at) - va.created_at))/3600) as avg_waiting_hours
FROM xcentro.voucher_approvals va
JOIN xcentro.payment_vouchers pv ON va.voucher_id = pv.voucher_id
WHERE pv.business_id = $1
GROUP BY va.action;
```
8. ERP Sync Health Dashboard 🔶
```sql
SELECT
erp_sync_status,
COUNT(*) as voucher_count,
SUM(net_amount) as total_amount,
AVG(erp_sync_attempts) as avg_attempts
FROM xcentro.payment_vouchers
WHERE business_id = $1
AND status = 'posted'
GROUP BY erp_sync_status;
```
9. Payout Success Rate 🔶
```sql
SELECT
ptr.status,
COUNT(*) as count,
SUM(ptr.amount) as total_amount,
AVG(EXTRACT(EPOCH FROM (ptr.completed_at - ptr.initiated_at))/3600) as avg_completion_hours
FROM xcentro.payout_transfer_requests ptr
JOIN xcentro.payment_vouchers pv ON ptr.voucher_id = pv.voucher_id
WHERE pv.business_id = $1
AND ptr.is_deleted = FALSE
GROUP BY ptr.status;
```
10. Recent Payment Activity Stream 🔶
```sql
SELECT
psc.action,
psc.action_timestamp,
psc.actor_type,
psc.amount,
psc.utr,
p.payment_method,
p.status
FROM xcentro.payment_state_changes psc
JOIN xcentro.payments p ON psc.payment_id = p.payment_id
WHERE p.business_id = $1
ORDER BY psc.action_timestamp DESC
LIMIT 20;
- Index: idx_payment_state_payment (already exists!)
```
---
## 🔴 HARD TO FETCH

Complex analytics requiring ETL pipelines or ML models

Advanced Insights (Requires Data Warehouse + Scheduled Jobs)

1. Cash Flow Forecasting 🚀
- Complexity: Predictive analytics using historical payment patterns
- Requirements:
    - Historical data from payments, payment_vouchers, payout_transfer_requests
    - Time-series analysis of seasonality patterns
    - ML model (Prophet/ARIMA) for forecasting
- Implementation:
> Daily ETL Job (Airflow/Prefect)
> 1. Extract: Last 365 days of payment data
> 2. Feature Engineering: Day of week, month, holidays
> 3. Train Prophet model
> 4. Predict next 30 days cash flow
> 5. Store in analytics.cash_flow_forecast
- Output: "Expected ₹5.2L inflow next week, ₹8.1L payout scheduled"
2. Vendor Payment Behavior Analysis 🚀
- Insight: "You pay Vendor X 5 days before due date on average"
- Query Complexity:
-- Requires joining vouchers + payees + transfers
-- Calculate: invoice_due_date - actual_payout_date
-- Aggregate across vendors, identify patterns
- Implementation: Weekly Spark job → analytics.vendor_payment_metrics
3. Payment Gateway Performance Scorecard 🚀
- Metrics:
    - Success rate by gateway, time of day, amount range
    - Average response time (from metadata JSONB parsing)
    - Cost comparison (MDR analysis)
    - Failure reason categorization
- Complexity:
-- Extract from JSONB metadata field
```sql
SELECT
gateway_used,
metadata->>'response_time_ms' as response_time,
COUNT(*) FILTER (WHERE status = 'success') * 100.0 / COUNT(*) as success_rate
FROM payments
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY gateway_used;
- Implementation: Daily aggregation job → analytics.gateway_performance_daily
```
4. Anomaly Detection (Fraud Prevention) 🚀
- Patterns to Detect:
    - Payment to new vendor > ₹5L (flag for review)
    - Unusual payment time (3 AM on Sunday)
    - Duplicate invoice numbers across vouchers
    - Rapid succession of high-value payments
- Implementation:
> Real-time streaming pipeline (Kafka + Flink)
> OR nightly batch job analyzing payment_state_changes
> Use statistical methods (IQR, z-score) + ML (Isolation Forest)
- Output: Alerts in analytics.fraud_alerts
5. Tax Compliance Dashboard 🚀
- Aggregations:
```sql
SELECT
pv.tds_section,
SUM(pv.tds_amount) as total_tds,
SUM(pv.cgst_amount + pv.sgst_amount + pv.igst_amount) as total_gst,
COUNT(DISTINCT pv.payee_id) as unique_vendors
FROM payment_vouchers pv
WHERE pv.business_id = $1
AND EXTRACT(YEAR FROM pv.voucher_date) = EXTRACT(YEAR FROM CURRENT_DATE)
GROUP BY pv.tds_section;
- Complexity: Cross-referencing with payee_vendor.gstin, generating Form 26AS
- Implementation: Monthly job → analytics.tax_summary_monthly
```
6. Approval Bottleneck Analysis 🚀
- Insight: "Finance approvals take avg 2.3 days, 40% longer than peers"
- Query:
```sql
SELECT
approver_user_id,
COUNT(*) as approvals_pending,
AVG(CURRENT_TIMESTAMP - va.created_at) as avg_pending_duration
FROM voucher_approvals va
WHERE action = 'pending'
GROUP BY approver_user_id;
```
- Implementation: Real-time dashboard + weekly report
7. MDR Optimization Recommendations 🚀
- Analysis:
-- Compare MDR across gateways by payment method
```sql
SELECT
payment_method,
gateway_used,
AVG(mdr_amount * 100.0 / NULLIF(settlement_amount, 0)) as avg_mdr_percent,
COUNT(*) as txn_count
FROM payments
WHERE settlement_status = 'settled'
GROUP BY payment_method, gateway_used;
- Recommendation Engine: "Switch IMPS < ₹5K to Gateway B saves ₹15K/month"
- Implementation: Weekly optimization job
```
8. Reconciliation Health Score 🚀
- 3-Way Match: Payment → Settlement → Payout
- Complexity:
-- Find orphaned records
```sql
SELECT p.payment_id
FROM payments p
LEFT JOIN payment_vouchers pv ON p.payment_id = pv.payment_id
LEFT JOIN payout_transfer_requests ptr ON pv.voucher_id = ptr.voucher_id
WHERE p.settlement_status = 'settled'
AND ptr.transfer_request_id IS NULL;
- Implementation: Daily reconciliation job → analytics.recon_exceptions
```
9. Customer Lifetime Value (CLV) by Payment Method 🚀
- Analysis: Which payment methods lead to higher long-term usage?
- Data Mining: Cohort analysis from payments + business onboarding data
- Implementation: Monthly ETL with customer segmentation
10. Predictive Payout Scheduling 🚀
- Goal: Auto-suggest optimal payout dates based on settlement patterns
- ML Model: Learn from historical settlement_utr, settled_at timestamps
- Output: "Settlement typically completes T+2, schedule payout for Wednesday"

---
📋 Recommended Implementation Plan

Phase 1: Quick Wins (Week 1)

Add missing indexes:
```sql
CREATE INDEX idx_payments_business_created
ON xcentro.payments(business_id, created_at DESC, status);

CREATE INDEX idx_stored_cards_business_active
ON xcentro.stored_cards(business_id, is_active)
WHERE is_active = TRUE;

CREATE INDEX idx_vouchers_business_status
ON xcentro.payment_vouchers(business_id, status);
```
Phase 2: Caching Layer (Week 2)

Implement Go-level caching with Redis:
```javascript
type HomePageMetrics struct {
    TotalPaymentsThisMonth  int64
    PendingSettlement       int64
    ActivePayees            int
    TopVendors              []VendorSummary
    // ... cache for 15 min
}
```

Phase 3: Materialized Views (Week 3)
```sql
CREATE MATERIALIZED VIEW analytics.mv_monthly_payment_stats AS
SELECT
business_id,
date_trunc('month', created_at) as month,
payment_method,
COUNT(*) as txn_count,
SUM(amount) as total_amount
FROM xcentro.payments
WHERE status = 'success'
GROUP BY business_id, month, payment_method;

-- Refresh via pg_cron every hour
SELECT cron.schedule(
'refresh-payment-stats',
'0 * * * *',
$$REFRESH MATERIALIZED VIEW CONCURRENTLY analytics.mv_monthly_payment_stats$$
);
```
Phase 4: Analytics Pipeline (Month 2)

Set up Airflow/Prefect DAGs:
1. Daily Job: Gateway performance, reconciliation
2. Weekly Job: Vendor analysis, MDR optimization
3. Monthly Job: Tax summaries, compliance reports
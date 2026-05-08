# Improvement Plan 01 — Structured Metrics & Observability

**Priority:** High  
**Effort:** Medium (2–3 days)  
**Impact:** Operational visibility, cost tracking, SLA monitoring  

---

## Problem

The worker currently relies entirely on log parsing for operational insight. This means:
- No real-time dashboard of queue depth, throughput, or error rates.
- No automated alerts when the DLQ grows or LLM costs spike.
- Token usage is logged per-document but never aggregated — no way to see total LLM spend over a period.
- Template hit rate (the most important cost-efficiency metric) is not tracked anywhere.
- Extraction duration distribution is invisible without manually mining logs.

---

## Proposed Solution

Expose a **Prometheus metrics endpoint** (or push to StatsD/Datadog) from the worker. Since the worker is a long-running process with no HTTP server, metrics would be pushed via a push gateway or exposed via a lightweight background HTTP thread.

### Metrics to Track

| Metric Name | Type | Labels | Description |
|---|---|---|---|
| `xcentro_docs_processed_total` | Counter | `path`, `status` | Documents completed, by extraction path and final status |
| `xcentro_extraction_duration_seconds` | Histogram | `path` | End-to-end processing time per document |
| `xcentro_llm_tokens_total` | Counter | `type` (prompt/completion) | Total tokens consumed across all LLM calls |
| `xcentro_llm_calls_total` | Counter | `model`, `outcome` | LLM calls by model and outcome (success/quota_error/error) |
| `xcentro_template_hit_rate` | Gauge | — | Rolling ratio: template-only / total docs processed |
| `xcentro_queue_depth` | Gauge | `queue` | Current length of each Redis queue |
| `xcentro_dlq_depth` | Gauge | `queue` | Current length of each DLQ |
| `xcentro_validation_failures_total` | Counter | `check` | Count of each type of validation failure |
| `xcentro_template_gen_total` | Counter | `outcome` | Template generation outcomes (ok / verification_failed / error) |
| `xcentro_ocr_engine_selected` | Counter | `engine` | How often pdftotext vs tesseract wins |

### Implementation Approach

**Option A: Prometheus Push Gateway**
- Add `prometheus_client` to requirements.
- Push metrics to a Prometheus push gateway after each document.
- No HTTP server needed in the worker itself.
- Suitable for containerized environments.

```python
from prometheus_client import CollectorRegistry, Counter, push_to_gateway

registry = CollectorRegistry()
docs_processed = Counter('xcentro_docs_processed_total', 'Documents processed',
                         ['path', 'status'], registry=registry)

# After each document:
docs_processed.labels(path='template', status='REVIEW_REQUIRED').inc()
push_to_gateway('pushgateway:9091', job='python_worker', registry=registry)
```

**Option B: Expose metrics via background HTTP thread**
- Start a background `threading.Thread` running a minimal HTTP server on port 9100.
- Expose `/metrics` endpoint.
- Prometheus scrapes it.
- More aligned with standard Prometheus pull model.

**Recommended:** Option B for Kubernetes deployments (scrape-based), Option A for simpler setups.

### Files to Modify

- `requirements.txt` — add `prometheus-client>=0.20`
- `infrastructure/metrics.py` — new file, defines all metric objects and a `record_extraction()` helper
- `usecases/extraction_service.py` — call `metrics.record_extraction(...)` at the end of `process()`
- `main.py` — start metrics HTTP thread, call `metrics.record_queue_depths(depths)` in the reaper tick

### Acceptance Criteria

- A Grafana dashboard can show: template hit rate, average extraction duration, LLM token spend per day, DLQ depth.
- Alert fires when DLQ depth > 10 for > 5 minutes.
- Alert fires when template hit rate drops below 50% for > 1 hour (indicates template library regression).

---

## Cost/Benefit

The `prometheus-client` library is a 3-file pure-Python package with zero overhead on the hot path. The metrics HTTP thread uses negligible resources. The ROI is immediate operational visibility — without this, the team is flying blind on cost and reliability in production.

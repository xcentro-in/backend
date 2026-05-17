# Improvement Plan 02 — Concurrent Document Processing

**Priority:** Medium  
**Effort:** Medium (2–3 days)  
**Impact:** Throughput, latency under load  

---

## Problem

The worker currently processes documents **sequentially** — one at a time. While it dequeues a task in 5ms, the actual extraction (especially LLM path) takes 3–8 seconds. This means:

- Queue depth spikes during batch uploads (e.g., a business uploads 50 invoices at once).
- Throughput ceiling: ~7–15 documents/minute on the LLM path, ~60–300 docs/minute on the template path.
- `ThreadPoolExecutor` is already imported in `main.py` but is unused.

---

## Current Architecture Note

The code was written with threading in mind — `Invoice2DataExtractor._lock` uses a `threading.Lock` for template cache coherence, and `GeminiLLMExtractor` uses a `threading.BoundedSemaphore` for per-process concurrency control. The database pool (`SimpleConnectionPool`) is also thread-safe. The groundwork is already there.

---

## Proposed Solution

Replace the sequential dequeue loop with a **bounded thread pool**. Each worker thread picks up one document, runs the full pipeline, and releases its slot when done.

### Design

```python
from concurrent.futures import ThreadPoolExecutor, Future

MAX_WORKERS = int(os.getenv("WORKER_CONCURRENCY", "4"))

def process_one(payload, queue, service, settings):
    """Runs in a thread pool worker."""
    doc_id = uuid.UUID(payload["document_id"])
    lock_key = f"xcentro:doc_lock:{doc_id}"
    if not queue.acquire_lock(lock_key, ttl=300):
        queue.enqueue({k: v for k, v in payload.items() if k != "_raw"})
        queue.ack(payload)
        return
    try:
        service.process(doc_id)
        queue.ack(payload)
    except Exception as exc:
        # retry / DLQ logic unchanged
        ...
    finally:
        queue.release_lock(lock_key)

with ThreadPoolExecutor(max_workers=MAX_WORKERS) as pool:
    while not _shutdown:
        payload = queue.dequeue(timeout=5)
        if payload:
            pool.submit(process_one, payload, queue, service, settings)
```

### Concurrency Limits

| Resource | Current Limit | Concurrent Workers |
|---|---|---|
| Gemini API | `_gemini_semaphore` (default 32) | Must be ≥ `MAX_WORKERS` |
| PostgreSQL pool | `maxconn=5` | Increase to `MAX_WORKERS * 2` |
| Redis | Single client | Redis connections are thread-safe; no change needed |
| Temp files | Per-document unique filenames | Already safe (UUID-based paths) |

### Configuration

```dotenv
WORKER_CONCURRENCY=4   # number of parallel document slots
POSTGRES_POOL_MAX=10   # increase to 2× concurrency
GEMINI_MAX_CONCURRENT=4  # keep <= concurrency to avoid quota bursts
```

### Throughput Projections

| Scenario | Sequential | 4 Workers | 8 Workers |
|---|---|---|---|
| All LLM (6s avg) | ~10 docs/min | ~40 docs/min | ~80 docs/min |
| All template (0.3s avg) | ~200 docs/min | ~800 docs/min | limited by DB |
| Mixed 70/30 | ~30 docs/min | ~100 docs/min | ~200 docs/min |

### Files to Modify

- `main.py` — Replace `while` loop with `ThreadPoolExecutor` pattern; add `WORKER_CONCURRENCY` setting
- `infrastructure/config.py` — Add `worker_concurrency: int = 1` and `postgres_pool_max: int = 5`
- `infrastructure/database.py` — Accept configurable `maxconn`
- `infrastructure/template_repository.py` — Accept configurable `maxconn`

### Risks

- **Queue ack safety**: If a thread panics, the task stays in the in-flight list and is recovered by the reaper. No regression.
- **Lock contention**: The distributed lock already prevents double-processing, even across worker instances.
- **LLM quota**: The Gemini semaphore already caps concurrent API calls. Set `GEMINI_MAX_CONCURRENT` ≤ `WORKER_CONCURRENCY`.

### Acceptance Criteria

- 4 concurrent workers process a 50-document batch 3× faster than sequential.
- No documents lost or double-processed under concurrent load.
- Graceful shutdown waits for all in-flight threads to complete before exiting.

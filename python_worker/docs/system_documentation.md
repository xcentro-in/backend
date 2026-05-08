# XCentro Python Extraction Worker — System Documentation

**Version:** 1.0  
**Module:** `backend/python_worker`  
**Purpose:** Asynchronous microservice for extracting structured financial data from voucher documents (Invoices & Purchase Orders)

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Architecture](#2-architecture)
3. [Pipeline: Step-by-Step](#3-pipeline-step-by-step)
4. [Components](#4-components)
5. [Domain Model](#5-domain-model)
6. [Configuration Reference](#6-configuration-reference)
7. [Document Lifecycle & Status Transitions](#7-document-lifecycle--status-transitions)
8. [Extraction Methods](#8-extraction-methods)
9. [Validation Rules](#9-validation-rules)
10. [GST Normalization](#10-gst-normalization)
11. [Template Generation (HITL Loop)](#11-template-generation-hitl-loop)
12. [Reliability & Error Handling](#12-reliability--error-handling)
13. [OCR Engine Selection](#13-ocr-engine-selection)
14. [Security Considerations](#14-security-considerations)
15. [Operational Runbook](#15-operational-runbook)
16. [Testing](#16-testing)
17. [Dependencies](#17-dependencies)

---

## 1. System Overview

The Python Extraction Worker is a background microservice in the XCentro backend. It runs independently of the Go API server, consuming document processing tasks from a Redis queue.

When a user uploads a voucher document (PDF invoice or purchase order) through the XCentro frontend:
1. The Go API saves the file to S3 and records a `QUEUED` document row in PostgreSQL.
2. It pushes a task `{"document_id": "<uuid>"}` onto the `xcentro:extraction_tasks` Redis queue.
3. The Python worker picks up the task, extracts structured data from the PDF, validates it, and writes the result back to PostgreSQL.
4. The Go API serves the extracted `draft_data` to the frontend for human review.

The worker is designed to be **stateless** — it holds no local state between documents. All state lives in PostgreSQL and Redis.

---

## 2. Architecture

The worker follows **Clean Architecture** (also called Hexagonal or Ports-and-Adapters architecture), with a strict dependency rule: outer layers depend on inner layers, never the reverse.

```
┌─────────────────────────────────────────────────────────┐
│  Infrastructure (adapters — concrete implementations)    │
│  ┌──────────┐  ┌────────┐  ┌─────────┐  ┌───────────┐  │
│  │ database │  │ redis  │  │  llm_   │  │ invoice2  │  │
│  │  .py     │  │_client │  │ client  │  │ data_     │  │
│  │ (psycopg)│  │  .py   │  │  .py    │  │ wrapper   │  │
│  └────┬─────┘  └───┬────┘  └────┬────┘  └─────┬─────┘  │
│       │            │            │              │         │
│  ┌────▼────────────▼────────────▼──────────────▼──────┐ │
│  │                 Use Cases                           │ │
│  │    extraction_service.py    validation.py           │ │
│  └────────────────────┬────────────────────────────────┘ │
│                       │                                   │
│  ┌────────────────────▼────────────────────────────────┐ │
│  │                   Domain                             │ │
│  │  entities.py   enums.py   ports.py   exceptions.py  │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                           │
│  main.py  (Composition Root)                             │
└─────────────────────────────────────────────────────────┘
```

### Layer Responsibilities

**Domain** (`domain/`) — Zero external dependencies. Contains:
- `entities.py` — Core data structures: `Document`, `ExtractionResult`, `LineItem`
- `enums.py` — `DocumentStatus`, `DocumentType`, `ProcessedBy`
- `exceptions.py` — Domain-specific exceptions (`ExtractionError`, `LLMExtractionError`, `TemplateMatchError`, `TemplateGenerationError`)
- `ports.py` — Abstract interfaces (ABCs) that infrastructure must implement: `DocumentRepository`, `TemplateExtractor`, `TemplateRepository`, `LLMExtractor`, `FileDownloader`, `TaskQueue`

**Use Cases** (`usecases/`) — Business logic only. Depends only on domain ports:
- `extraction_service.py` — Orchestrates the full pipeline: download → OCR → template → LLM → validate → persist
- `validation.py` — Pure functions for required-field, GSTIN format, and arithmetic checks

**Infrastructure** (`infrastructure/`) — Concrete adapters wired to real systems:
- `config.py` — Pydantic Settings model loading from environment variables
- `database.py` — PostgreSQL document repository (psycopg2 connection pool)
- `template_repository.py` — PostgreSQL template repository
- `template_seeder.py` — Seeds filesystem YAML templates into the DB on startup
- `redis_client.py` — Redis task queue with crash-safe in-flight tracking
- `invoice2data_wrapper.py` — Template-based extraction + template round-trip verifier
- `llm_client.py` — Google Gemini extraction + template generation
- `ocr.py` — Dual-engine OCR selection (pdftotext + Tesseract)
- `file_downloader.py` — S3 file download

**Composition Root** (`main.py`) — Wires all components together, starts the event loop.

---

## 3. Pipeline: Step-by-Step

```
Redis Queue
    │
    │  BRPOP (blocking pop, 5s timeout)
    ▼
[1] Payload validation
    - Checks for `document_id` field
    - Validates UUID format
    - Malformed → Dead Letter Queue

[2] Distributed lock acquisition
    - SET NX EX 300 on key xcentro:doc_lock:<uuid>
    - If locked by another worker → re-enqueue and skip

[3] Database fetch
    - Fetch document row from xcentro.voucher_documents
    - If not found → skip (log warning)
    - If already in terminal status (VALIDATED / REVIEW_REQUIRED / FAILED) → skip (stale task guard)

[4] Status: OCR_PROCESSING
    - Download PDF from S3 to secure temp directory
    - Run OCR quality selection (pdftotext vs Tesseract)
    - Persist raw_ocr to DB

[5] Template extraction attempt
    - Refresh templates from DB (skipped if signature unchanged)
    - Run invoice2data against all loaded templates
    - Pick best match by field count score
    - On TemplateMatchError → proceed to step 7

[6] Validation of template result
    - Run check_required_fields + check_gstin (no arithmetic on template path)
    - If validation passes → set processed_by = INVOICE2DATA, proceed to step 8
    - If validation fails → proceed to step 7 (LLM fallback)

[7] LLM extraction (Gemini)
    - Status → LLM_PROCESSING
    - Upload PDF to Gemini Files API
    - Send extraction prompt with 10 detailed rules
    - Parse JSON response, map to ExtractionResult
    - Log token usage (prompt + completion tokens)

[8] Document type check
    - If doc_type == OTHER or UNKNOWN → status = FAILED, set violation flag, done

[9] GST normalization
    - Apply mutual exclusivity (IGST xor SGST/CGST)
    - Infer IGST or SGST/CGST from total_gst + GSTIN state codes if breakdown missing
    - Apply line item tax_rate defaulting from document-level gst_rate

[10] Full validation
    - check_required_fields + check_gstin + check_arithmetic
    - Failures appended as validation_failures array in draft_data

[11] Persist & finalize
    - Status = REVIEW_REQUIRED (always — human must approve)
    - Write draft_data (normalized extraction), doc_type, processed_by to DB

[12] ACK + lock release
    - Remove task from Redis in-flight list
    - Release distributed lock
```

---

## 4. Components

### main.py — Entry Point / Composition Root

Responsibilities:
- Loads `Settings` from environment
- Instantiates all infrastructure adapters
- Seeds filesystem templates into DB
- Performs startup stale-task reap
- Runs the main consumer loop (`while not _shutdown`)
- Handles OS signals (SIGINT, SIGTERM) for graceful shutdown
- Implements the retry loop: up to `MAX_RETRIES=3` attempts per document before DLQ

**Key constants:**
| Constant | Value | Purpose |
|---|---|---|
| `MAX_RETRIES` | 3 | Attempts per document before DLQ |
| `CONNECTION_RETRY_DELAY` | 5s | Wait before reconnecting on Redis error |
| `STALE_TASK_AGE_SECONDS` | 900 (15 min) | Task heartbeat age before reaping |
| `REAPER_INTERVAL_SECONDS` | 60 | How often the periodic reaper runs |

### ExtractionService — Core Orchestrator

The `ExtractionService` class in `usecases/extraction_service.py` is the heart of the system. It is framework-agnostic (no Flask, no FastAPI, no Redis knowledge) and receives all dependencies via constructor injection.

**Public methods:**
- `process(document_id)` — Full extraction pipeline
- `mark_failed(document_id, reason)` — Terminal failure after retry exhaustion
- `generate_template_for_document(document_id)` — HITL-triggered template generation

**Private helpers:**
- `_try_template(path, doc)` — Attempt invoice2data extraction
- `_try_llm(path, doc)` — Attempt Gemini LLM extraction
- `_finalize(doc, result)` — Validate + persist final result
- `_process_inner(document_id)` → `(path_taken, final_status, llm_usage)` — Returns metrics for logging

### RedisTaskQueue — Queue Consumer

Implements crash-safe at-least-once delivery using Redis lists:

| Redis Key Pattern | Purpose |
|---|---|
| `xcentro:extraction_tasks` | Main work queue |
| `xcentro:template_tasks` | Template generation queue |
| `<queue>:processing` | In-flight list (task moved here after dequeue) |
| `<queue>:processing:last_seen` | Hash of raw→timestamp for heartbeat tracking |
| `<queue>:dlq` | Dead Letter Queue for permanently failed tasks |
| `xcentro:doc_lock:<uuid>` | Per-document distributed lock (SET NX EX) |

**Dequeue flow:** `BRPOP queue` → `LPUSH queue:processing raw` + `HSET last_seen raw now` → process → `LREM queue:processing 1 raw` (ACK)

### GeminiLLMExtractor — LLM Adapter

- Uses `google-genai` SDK with `gemini-2.5-flash` (primary model)
- Supports a configurable fallback model for quota-exhausted scenarios
- `extract()`: Uploads PDF to Gemini Files API, sends extraction prompt, parses JSON
- `generate_template()`: Sends OCR text + validated data + optional historical examples to produce an invoice2data YAML template
- Retries via `tenacity`: up to 3 attempts, exponential backoff (2–30s), skips retry on hard quota exhaustion (429/RPD) — falls through to fallback model instead
- Per-call concurrency capped via `threading.BoundedSemaphore` (default: 32)

### Invoice2DataExtractor — Template Adapter

- Wraps the `invoice2data` library for regex-based template matching
- Templates reloaded from DB on every call, keyed by content SHA-1 hash — no restart needed
- Tries every matching template and selects the one extracting the most fields (field-count score)
- Line items worth 2 points; all other fields 1 point
- Falls back to filesystem templates if DB is empty; falls back to invoice2data built-ins if neither is available

---

## 5. Domain Model

### Document
```python
@dataclass
class Document:
    id: uuid.UUID
    s3_path: str
    doc_type: DocumentType        # INVOICE | PURCHASE_ORDER | OTHER | UNKNOWN
    status: DocumentStatus        # lifecycle state
    raw_ocr: str | None           # raw text extracted by OCR
    raw_extraction: dict | None   # full structured LLM JSON (for audit)
    draft_data: dict | None       # normalized extraction → shown to user for review
    validated_data: dict | None   # human-approved version (post-HITL)
    i2d_template: str | None      # template filename that matched/was generated
    processed_by: ProcessedBy | None  # INVOICE2DATA | LLM
```

### ExtractionResult
```python
@dataclass
class ExtractionResult:
    invoice_number, invoice_date, invoice_due_date, po_number
    vendor_name, vendor_gstin, buyer_gstin
    total_amount, subtotal, gross_amount
    discount, discount_type ("FLAT" | "PERCENTAGE")
    payment_terms
    gst_rate, sgst, cgst, igst, total_gst
    shipping
    line_items: list[LineItem]
    doc_type: DocumentType
    raw: dict                    # original LLM/template output (preserved for audit)
    confidence_scores: dict[str, float]  # per-field 0.0–1.0
```

### LineItem
```python
@dataclass
class LineItem:
    sku: str | None        # HSN code / item code / part number
    description: str | None
    quantity: float | None
    unit_price: float | None
    tax_rate: float | None  # e.g. 18 for 18%
    tax: float | None       # absolute tax amount in currency
    amount: float | None    # pre-tax line total (qty × unit_price)
```

---

## 6. Configuration Reference

All settings are loaded by `infrastructure/config.py` using Pydantic Settings. Environment variables (or `.env` file) are the source of truth.

| Variable | Required | Description |
|---|---|---|
| `POSTGRES_HOST` | Yes | PostgreSQL host |
| `POSTGRES_PORT` | No (default: 5432) | PostgreSQL port |
| `POSTGRES_DB` | Yes | Database name |
| `POSTGRES_USER` | Yes | Database user |
| `POSTGRES_PASSWORD` | Yes | Database password |
| `REDIS_HOST` | Yes | Redis host |
| `REDIS_PORT` | No (default: 6379) | Redis port |
| `REDIS_USER` | No | Redis username (ACL) |
| `REDIS_PASSWORD` | No | Redis password |
| `REDIS_QUEUE` | No (default: `xcentro:extraction_tasks`) | Main extraction queue name |
| `REDIS_TEMPLATE_QUEUE` | No (default: `xcentro:template_tasks`) | Template generation queue name |
| `AWS_ACCESS_KEY_ID` | Yes | S3 access key |
| `AWS_SECRET_ACCESS_KEY` | Yes | S3 secret key |
| `AWS_REGION` | Yes | S3 region |
| `S3_BUCKET_NAME` | Yes | S3 bucket for document storage |
| `GEMINI_API_KEY` | Yes (for LLM path) | Google AI Studio API key |
| `GEMINI_MODEL` | No (default: `gemini-2.5-flash`) | Primary Gemini model |
| `GEMINI_FALLBACK_MODEL` | No | Fallback model on quota exhaustion |
| `I2D_TEMPLATE_DIR` | No (default: `/app/templates`) | Filesystem template directory |
| `TEMP_DIR` | No (default: auto tmpdir) | Temp directory for downloaded files |
| `MAX_DOWNLOAD_BYTES` | No | Max file size cap for S3 downloads |

---

## 7. Document Lifecycle & Status Transitions

```
QUEUED
  │
  ├──► OCR_PROCESSING  (worker picks up task, downloads file)
  │         │
  │         ├──► LLM_PROCESSING  (template failed → LLM fallback)
  │         │         │
  │         │         └──────────────────────┐
  │         │                                │
  │         └──────────────────────────────► REVIEW_REQUIRED  (extraction complete, awaiting human)
  │                                                │
  │                                                └──► VALIDATED  (human approved)
  │                                                          │
  │                                                          └──► [template generation triggered]
  │
  └──► FAILED  (max retries exhausted, or non-invoice document detected)
```

**Terminal statuses** (never overwritten by a stale task): `VALIDATED`, `REVIEW_REQUIRED`, `FAILED`

---

## 8. Extraction Methods

### Method 1: Template Extraction (invoice2data)

Uses YAML regex templates to extract fields from OCR text deterministically.

Template structure (example):
```yaml
issuer: Standard GST Invoice - HSN Table Format
keywords:
  - "HSN/SAC"
  - "TAX INVOICE"

fields:
  invoice_number:
    - '(?si)Invoice\s*No[.:]?\s*(?P<invoice_number>[A-Z0-9][A-Z0-9/_-]+)'
  date:
    - '(?si)Invoice\s*Date[:\s]+(?P<date>\d{1,2}[/-]\d{1,2}[/-]\d{2,4})'
  amount:
    - '(?si)Grand\s*Total[:\s]+(?P<amount>[\d,]+\.\d{2})'

options:
  currency: INR
  decimal_separator: "."
  thousands_separator: ","
  date_formats:
    - "%d/%m/%Y"
    - "%d-%m-%Y"
```

Templates are stored in PostgreSQL (`xcentro.templates`) and automatically loaded by the worker. The filesystem directory (`/app/templates`) is used as a seed source on startup when the DB is empty.

**Template selection when multiple match:** Every matching template is scored by the number of fields it successfully extracts (line items worth 2 points). The highest-scoring template wins. Ties are broken by DB insertion order (deterministic).

### Method 2: LLM Extraction (Google Gemini)

Sends the actual PDF file to the Gemini Files API along with a detailed extraction prompt. The prompt specifies:
- Document classification (INVOICE / PURCHASE_ORDER / OTHER)
- Discount logic (FLAT vs PERCENTAGE)
- Line item rules (multi-row description concatenation, pre-tax amounts, HSN/SKU extraction)
- GST rules (SGST/CGST vs IGST mutual exclusivity, total_gst handling)
- GSTIN extraction rules (15-char, strip whitespace)
- Vendor name extraction rules (legal entity, not branch name)
- Amount arithmetic chain for self-checking
- Confidence scoring instructions

The LLM response is expected to be a single JSON object (no markdown fences). The worker strips markdown fences if present and parses the JSON.

---

## 9. Validation Rules

Defined in `usecases/validation.py`. All checks are pure functions.

### Required Fields

| Document Type | Required Fields |
|---|---|
| INVOICE | `invoice_number`, `invoice_date`, `total_amount`, `vendor_name`, `line_items` |
| PURCHASE_ORDER | `po_number`, `vendor_name`, `total_amount`, `line_items` |

### GSTIN Format

Pattern: `^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z]{1}[A-Z0-9]{1}Z[A-Z0-9]{1}$`

- Exactly 15 characters
- First 2 digits: state code
- Next 5 uppercase letters: PAN company code
- Next 4 digits: PAN entity number
- Next letter: PAN check digit
- Next character: entity number (A-Z or 0-9)
- Penultimate: always `Z`
- Last: checksum character

### Arithmetic Check

```
computed = Σ(item.quantity × item.unit_price + item.tax)  [for items with complete data]
computed -= discount  [flat or computed from percentage × subtotal]
|computed - total_amount| ≤ ₹0.50
```

Note: Items missing `quantity` or `unit_price` are skipped. If all items are incomplete, the check is skipped entirely.

### Routing Logic

| Stage | When | Action |
|---|---|---|
| After template extraction | Required fields OK, GSTIN OK | Accept template result (skip arithmetic check) |
| After template extraction | Any failure | Fall through to LLM |
| After LLM extraction | Required fields OK, GSTIN OK, arithmetic OK | Accept |
| After LLM extraction | Any failure | Append `validation_failures` to draft_data, still accept as REVIEW_REQUIRED |

---

## 10. GST Normalization

Applied in `_normalize_result()` in `extraction_service.py` before any data is persisted.

### Rule 1: GST Type Mutual Exclusivity

Indian GST transactions are either intra-state (SGST + CGST) or inter-state (IGST only):

| Condition | Action |
|---|---|
| IGST > 0 only | Leave as-is; clear SGST/CGST to null |
| SGST/CGST > 0 only | Leave as-is; clear IGST to null |
| Both present | Treat as inter-state; clear SGST/CGST (log warning) |

### Rule 1b: GST Inference from total_gst

When a document only shows a lump-sum GST total without breakdown:
1. Compare first 2 characters of `vendor_gstin` and `buyer_gstin` (Indian state codes).
2. Same state → split `total_gst` equally into CGST and SGST (odd penny goes to SGST).
3. Different states → assign entire `total_gst` to IGST.
4. Missing GSTINs → leave all null (route to human reviewer).

### Rule 2: Line Item Tax Rate Default

If a line item has no `tax_rate` AND at least one other line item already has one → set to `0.0` (item is tax-exempt).

### Rule 3: Line Item Tax Rate Distribution

If NO line item has a `tax_rate` AND the document has a document-level `gst_rate` → distribute `gst_rate` to all line items.

### Rule 4: PO Invoice Number

Purchase orders do not have invoice numbers. If `invoice_number` is present on a PO document, it is cleared.

---

## 11. Template Generation (HITL Loop)

After a document's draft data is validated by a human (`status = VALIDATED`), the Go API enqueues a task on `xcentro:template_tasks`. The worker calls `generate_template_for_document()`.

**Flow:**
1. Load document: requires `raw_ocr` and (`validated_data` or `draft_data`).
2. Skip if document was already extracted by INVOICE2DATA and has a template — no need to regenerate.
3. Fetch up to 5 previously validated documents from the same template (historical examples for the prompt).
4. Call `llm.generate_template(raw_ocr, target_data, historical_examples)` → raw YAML.
5. Post-process: add `(?si)` inline flags to all field regexes (case-insensitive + dot-matches-newline).
6. Round-trip verification: load the template, run it against the source OCR, check that it recovers `invoice_number`, `po_number`, `vendor_gstin`, `buyer_gstin`, and `total_amount`.
7. If verification fails: retry once, giving the LLM its own failed output and the failure reason.
8. If retry also fails: raise `TemplateGenerationError(retryable=False)` → goes to DLQ (no infinite retry).
9. If success: save to `xcentro.templates` via upsert. Template is active immediately.
10. Update `document.i2d_template` with the template filename.

**Template filename convention:** `<vendor_gstin_or_name>_<doc_type>.yml` (sanitized to `[a-z0-9_-]`, max 80 chars).

---

## 12. Reliability & Error Handling

### Retry Behaviour

| Error Type | Retryable | Behaviour |
|---|---|---|
| S3 download failure | Yes | Re-queued with `retry_count + 1` |
| LLM API 503 / transient | Yes | Re-queued; tenacity does internal backoff |
| LLM quota exhausted (429) | Yes (fallback first) | Tries fallback model; then re-queued |
| Template round-trip failure | No | DLQ immediately |
| Document not found in DB | No | Skip (not retried) |
| Max retries (3) exhausted | — | DLQ + `mark_failed()` |

### Dead Letter Queue

Failed tasks are written to `<queue>:dlq` as JSON records:
```json
{
  "payload": { "document_id": "...", "retry_count": 3 },
  "reason": "max retries exhausted: LLM API error: ...",
  "failed_at": 1714567890
}
```

The document is also marked `FAILED` in PostgreSQL (unless it had already reached `VALIDATED` or `REVIEW_REQUIRED`).

### Graceful Shutdown

On SIGINT or SIGTERM:
1. `_shutdown = True` is set.
2. The current document processing completes normally.
3. The queue connection and database pool are closed.
4. The worker exits.

No documents are abandoned mid-processing because the shutdown flag is checked between tasks, not during a task.

---

## 13. OCR Engine Selection

### Engines

| Engine | Library | Best For |
|---|---|---|
| pdftotext | poppler-utils (`invoice2data.input.pdftotext`) | Digital-native PDFs with embedded text layer |
| Tesseract | tesseract-ocr (`invoice2data.input.tesseract`) | Scanned PDFs, photographed documents |

### Quality Scoring

Each OCR output is scored by `_score_text_quality()`:

```
score = 0
score += min(length / 400, 1.5)          # longer text is better, up to a cap
score += min(token_count / 40, 1.5)      # more words is better
score += alnum_ratio * 2.0               # higher ratio of alphanumeric chars
score += min(keyword_hits * 0.25, 1.5)   # invoice domain keywords (GSTIN, total, etc.)
score -= min(replacement_chars * 0.3, 1.5)  # Unicode replacement char penalty
# Penalties for very short/sparse output:
if length < 80: score -= 0.75
if token_count < 12: score -= 0.75
if alnum_ratio < 0.45: score -= 0.75
```

### Selection Logic

1. Run pdftotext.
2. If `score >= 2.2`: use pdftotext (good text layer → Tesseract not needed).
3. If `score < 2.2`: run Tesseract.
4. If Tesseract score ≥ pdftotext score + 0.25: use Tesseract.
5. Otherwise: keep pdftotext.

---

## 14. Security Considerations

### Path Traversal Protection (Templates)
LLM-generated template filenames are derived from `vendor_gstin` or `vendor_name` — both are attacker-controlled values (extracted from an uploaded PDF). Before writing to disk, the filename component is:
1. Lowercased and stripped to `[a-z0-9_-]` only (no dots, no slashes, no spaces).
2. Capped at 80 characters.
3. The resolved `filepath` is asserted to be inside `template_root` via `relative_to()` — any path traversal attempt raises `ExtractionError` and the write is aborted.

### Prompt Injection Defense
OCR text is untrusted (it was extracted from a user-uploaded file). When sent to the LLM for template generation, it is fenced:
```
<<<BEGIN_UNTRUSTED_OCR>>>
{raw_ocr}
<<<END_UNTRUSTED_OCR>>>
```
The prompt instructs the model to treat everything within the fences as data only, not instructions. Additionally, OCR input is capped at 40,000 characters to limit the attacker's window.

### Secret Management
All credentials are loaded exclusively from environment variables via Pydantic Settings. No credentials are hardcoded or committed. The `.env.example` file shows the expected variable names with placeholder values.

### File Size Limits
S3 downloads are bounded by `MAX_DOWNLOAD_BYTES` to prevent memory exhaustion from maliciously large files.

---

## 15. Operational Runbook

### Starting the Worker

```bash
# Local development
cd backend/python_worker
cp .env.example .env  # fill in credentials
python -m venv venv && source venv/bin/activate
pip install -r requirements.txt
python main.py
```

```bash
# Docker
docker compose build python_worker
docker compose up -d python_worker
docker logs -f xcentro-python-worker
```

### Checking Queue Depth

```bash
redis-cli -h <host> -p <port> llen xcentro:extraction_tasks
redis-cli -h <host> -p <port> llen xcentro:template_tasks
```

### Inspecting the Dead Letter Queue

```bash
redis-cli -h <host> lrange xcentro:extraction_tasks:dlq 0 -1
```

### Replaying a DLQ Task

Manually push the `document_id` back to the source queue after investigating:
```bash
redis-cli lpush xcentro:extraction_tasks '{"document_id":"<uuid>"}'
# Also reset status in PostgreSQL:
UPDATE xcentro.voucher_documents SET status = 'QUEUED' WHERE id = '<uuid>';
```

### Checking the In-Flight List

```bash
redis-cli lrange xcentro:extraction_tasks:processing 0 -1
redis-cli hgetall xcentro:extraction_tasks:processing:last_seen
```

### Viewing Templates in DB

```sql
SELECT name, issuer, length(content) AS yaml_size, updated_at
FROM xcentro.templates
ORDER BY updated_at DESC;
```

### Scaling

The worker is stateless. Multiple instances can run in parallel — the distributed lock (`xcentro:doc_lock:<uuid>`) prevents any document from being processed by two workers simultaneously.

---

## 16. Testing

Tests live in `python_worker/tests/` and are run with pytest:

```bash
cd backend/python_worker
pytest -v
```

### Test Coverage

| Module | Test File | What's Covered |
|---|---|---|
| `usecases/validation.py` | `tests/usecases/test_validation.py` | Required fields (invoice + PO), GSTIN format, arithmetic (exact, tolerance, discounts, edge cases), run_all_checks aggregation |
| `usecases/extraction_service.py` | `tests/usecases/test_extraction_service_normalize.py` | All 4 GST normalization rules, GSTIN-based inference, line item tax rate defaulting |
| `usecases/extraction_service.py` | `tests/usecases/test_extraction_service_logging.py` | Log event emission for extraction path tracking |
| `infrastructure/llm_client.py` | `tests/infrastructure/test_llm_client.py` | JSON parsing, field mapping, confidence scores |
| `infrastructure/invoice2data_wrapper.py` | `tests/infrastructure/test_invoice2data_wrapper.py` | Template scoring, regex flag injection |
| `infrastructure/redis_client.py` | `tests/infrastructure/test_redis_client.py` | Dequeue/ack, DLQ, reaper, lock — using fakeredis |
| `infrastructure/ocr.py` | `tests/infrastructure/test_ocr.py` | Quality scoring, engine selection |
| `infrastructure/database.py` | `tests/infrastructure/test_database.py` | DB adapter |
| `infrastructure/config.py` | `tests/infrastructure/test_config.py` | Settings loading |
| `infrastructure/file_downloader.py` | `tests/infrastructure/test_file_downloader.py` | S3 download |
| `infrastructure/template_repository.py` | `tests/infrastructure/test_template_repository.py` | DB template upsert/fetch |

Tests use `fakeredis` for Redis and mock objects for external services — no real infrastructure needed.

---

## 17. Dependencies

### Production

| Package | Version | Purpose |
|---|---|---|
| `psycopg2-binary` | 2.9.10 | PostgreSQL adapter |
| `redis` | 5.2.1 | Redis client |
| `invoice2data` | 0.4.5 | Template-based PDF extraction |
| `google-genai` | 1.10.0 | Google Gemini LLM client |
| `boto3` | >=1.34,<2 | AWS S3 client |
| `tenacity` | >=8.2,<10 | Retry logic with backoff |
| `pydantic` / `pydantic-settings` | 2.10.6 / 2.8.1 | Configuration and type validation |
| `python-dotenv` | 1.0.1 | .env file loading |

### System Dependencies (must be installed separately)

| Tool | Purpose |
|---|---|
| `poppler-utils` (pdftotext) | Text extraction from PDF text layers |
| `tesseract-ocr` | OCR for scanned/image PDFs |

### Test Only

| Package | Purpose |
|---|---|
| `pytest` | Test runner |
| `pytest-asyncio` | Async test support |
| `fakeredis` | In-memory Redis fake for tests |

### Important Note on setuptools

`requirements.txt` pins `setuptools>=75.0.0,<81` because `invoice2data` depends on `pkg_resources`, which was removed in `setuptools` 82+.

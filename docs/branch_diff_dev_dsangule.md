# Branch Diff: `dev-dsangule` vs `dev` (merge base)

**Branch:** `dev-dsangule`  
**Compared against:** merge base with `dev` (`0802aa8`)  
**Total scope:** 6 commits · 47 files changed · **+6,012 lines / −10 lines**  
**Date range:** Initial extraction worker → PO-Invoice matching engine  

---

## Table of Contents

1. [Commit History](#1-commit-history)
2. [High-Level Summary](#2-high-level-summary)
3. [New Database Schema (Migration 000060)](#3-new-database-schema-migration-000060)
4. [Python Extraction Worker](#4-python-extraction-worker)
5. [Go Backend — Domain Layer](#5-go-backend--domain-layer)
6. [Go Backend — Document Upload Service & Handler](#6-go-backend--document-upload-service--handler)
7. [Go Backend — Document Review Service (HITL)](#7-go-backend--document-review-service-hitl)
8. [Go Backend — PO-Invoice Matching Engine](#8-go-backend--po-invoice-matching-engine)
9. [Go Backend — Payee Resolution Service](#9-go-backend--payee-resolution-service)
10. [Go Backend — Repository Layer](#10-go-backend--repository-layer)
11. [Go Backend — HTTP Handlers & Routes](#11-go-backend--http-handlers--routes)
12. [Infrastructure & DevOps](#12-infrastructure--devops)
13. [Payee Schema Changes (Quick Vendor Creation)](#13-payee-schema-changes-quick-vendor-creation)
14. [File Change Summary Table](#14-file-change-summary-table)

---

## 1. Commit History

| # | Hash | Message |
|---|---|---|
| 1 | `e8c18fc` | implement extraction worker, Redis queue, and backend updates |
| 2 | `9ae94c9` | added apis for listing invoice, po and voucher_documents tables |
| 3 | `260cb3b` | fix voucher saving logic and improve extraction results |
| 4 | `10c3b56` | Clean up environment configs and fix worker JSON serialization error |
| 5 | `6483372` | added template generation and matching |
| 6 | `c2fd5bb` | feat: implement PO-invoice matching engine and enhance document extraction accuracy |

---

## 2. High-Level Summary

This branch introduces the complete **Voucher Document Intelligence** feature — a full-stack system that:

1. Accepts uploaded PDF invoices and purchase orders via a new Go API endpoint.
2. Queues them for asynchronous processing via Redis.
3. A new Python microservice extracts structured financial data (template OCR → LLM fallback).
4. A Human-in-the-Loop (HITL) review UI allows a user to correct and validate extracted data.
5. On validation, the Go backend persists typed records to `invoice` / `purchase_orders` tables.
6. A deterministic **PO-Invoice Matching Engine** automatically matches invoices against purchase orders using PO number lookup + line-item SKU/description comparison.
7. A **Payee Resolution Service** looks up existing vendors by GSTIN or name during review.
8. A **Quick Vendor Creation** flow lets users create vendors without bank verification directly from the document review screen.

---

## 3. New Database Schema (Migration 000060)

**File:** `internal/adapters/database/postgres/migrations/sql/000060_create_extraction_tables.up.sql`  
**+236 lines**

Six new tables are created, all under the `xcentro` schema:

### `xcentro.voucher_documents`
Tracks every uploaded document through its extraction lifecycle.

| Column | Type | Notes |
|---|---|---|
| `id` | UUID (uuidv7) | Primary key |
| `s3_path` | TEXT | S3 object key for the uploaded file |
| `doc_type` | TEXT | `INVOICE` \| `PURCHASE_ORDER` \| `UNKNOWN` |
| `status` | TEXT | Lifecycle state (see document status enum below) |
| `raw_ocr` | TEXT | Raw text extracted by pdftotext or Tesseract |
| `raw_extraction` | JSONB | Full structured LLM JSON output (audit trail) |
| `draft_data` | JSONB | Normalized extraction shown for human review |
| `i2d_template` | TEXT | Template filename that matched or was generated |
| `processed_by` | TEXT | `INVOICE2DATA` \| `LLM` |
| `validated_data` | JSONB | Human-approved corrected data |
| `file_hash` | TEXT | SHA-256 hex; unique index for duplicate detection |

Indexes: `status`, unique partial on `file_hash`.

**Document status lifecycle:**
```
QUEUED → OCR_PROCESSING → LLM_PROCESSING → REVIEW_REQUIRED → VALIDATED
                                                             → FAILED
```

### `xcentro.invoice`
Typed invoice records created from validated document data.

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | Primary key |
| `inv_id` | TEXT | Invoice number string |
| `type` | invoice_type_enum | SALES \| PURCHASE \| PROFORMA \| CREDIT_NOTE |
| `bid` | UUID FK → business | Business that owns this invoice |
| `payee_id` | UUID FK → payee_master | Resolved vendor payee |
| `inv_date` / `inv_due_date` | DATE | Invoice and payment due dates |
| `cgst`, `igst`, `sgst` | DECIMAL(10,2) | Individual GST components |
| `total_amount` | DECIMAL(20,2) | Gross total before discounts |
| `net_amount` | DECIMAL(20,2) | Amount due (total − discount − TDS) |
| `gst_type` | TEXT | `INTER_STATE` \| `INTRA_STATE` |
| `po_number` | TEXT | PO reference for matching |
| `po_id` | UUID FK → purchase_orders | Set after successful match |
| `match_status` | TEXT | `PENDING_MATCH` \| `MATCHED` \| `ORPHAN_INVOICE` \| `MISMATCH` |
| `vendor_gstin` / `buyer_gstin` | TEXT | Raw GSTINs from the document |
| `document_id` | UUID FK → voucher_documents | Source document |

Indexes: `document_id`, `po_number`, `match_status`, `gst_type`, `po_id`.

### `xcentro.purchase_orders`
Typed purchase order records, with a running remaining balance.

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | Primary key |
| `document_id` | UUID FK → voucher_documents | Source document (NOT NULL) |
| `bid` | UUID FK → business | Owner business |
| `po_number` | TEXT | PO number string |
| `total_amount` | DECIMAL(20,2) | Original PO value |
| `remaining_balance` | DECIMAL(20,2) | Decremented as invoices are matched |
| `po_status` | TEXT | `OPEN` \| `PARTIALLY_INVOICED` \| `CLOSED` |

Indexes: `document_id`, `po_number`, `po_status`.

### `xcentro.line_items`
Line items for both invoices and purchase orders (shared table with CHECK constraint).

| Column | Type | Notes |
|---|---|---|
| `purchase_order_id` | UUID FK → purchase_orders ON DELETE CASCADE | Null if invoice item |
| `invoice_id` | UUID FK → invoice ON DELETE CASCADE | Null if PO item |
| `sku` | TEXT | HSN code, item code, or part number |
| `description` | TEXT | Line item description |
| `quantity` | DECIMAL(12,4) | Ordered quantity |
| `qty_received` | DECIMAL(12,4) DEFAULT 0 | Updated by matching engine as invoices match |
| `unit_price` | BIGINT | In paise (1/100 of ₹) |
| `tax_rate` | DECIMAL(5,2) | GST % for this line |
| `tax_amount` | BIGINT | Absolute tax in paise |
| `total_amount` | BIGINT | Line total in paise |

Constraint: exactly one of `purchase_order_id` or `invoice_id` must be non-null.  
Indexes: `purchase_order_id`, `invoice_id`, `sku`.

### `xcentro.match_results`
Immutable audit trail of every matching attempt.

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | Primary key |
| `invoice_id` | UUID FK → invoice ON DELETE CASCADE | Invoice being matched |
| `purchase_order_id` | UUID FK → purchase_orders ON DELETE SET NULL | PO matched against (nullable for orphans) |
| `match_status` | TEXT | Final outcome |
| `match_details` | JSONB | Full diagnosis: unmatched items, mismatches, reason |

### `xcentro.templates` (referenced by worker)
Stores LLM-generated invoice2data YAML templates. Created by the Python worker's template generation flow and loaded back at extraction time.

---

## 4. Python Extraction Worker

**Location:** `backend/python_worker/`  
**New files:** 20 files · **+1,300+ lines** across domain, infrastructure, and use cases  
**Architecture:** Clean Architecture (Domain → Use Cases → Infrastructure)

### 4.1 Domain Layer (`domain/`)

**`entities.py`** — Core data structures:
- `LineItem` — one line from an invoice/PO (sku, description, qty, unit_price, tax_rate, tax, amount)
- `ExtractionResult` — canonical extraction output (20+ fields covering all invoice/PO data)
- `Document` — aggregate root mirroring the `voucher_documents` DB row

**`enums.py`** — `DocumentStatus`, `DocumentType`, `ProcessedBy`

**`exceptions.py`** — `ExtractionError`, `LLMExtractionError`, `TemplateMatchError`, `TemplateGenerationError` (with `retryable` flag)

**`ports.py`** — Abstract interfaces (ABCs):
- `DocumentRepository` — `get_by_id`, `update`, `get_validated_by_template`
- `TemplateRepository` — `list_all`, `save`, `get_by_name`
- `TemplateExtractor` — `extract(file_path)`
- `LLMExtractor` — `extract(file_path)`, `generate_template(ocr, data, ...)`
- `FileDownloader` — `download(url, dest_dir)`
- `TaskQueue` — `dequeue`, `enqueue`, `ack`, `dead_letter`, `reap_stale`, `acquire_lock`

### 4.2 Infrastructure Layer (`infrastructure/`)

**`llm_client.py`** (`GeminiLLMExtractor`) — Google Gemini integration:
- Sends PDF file directly to Gemini Files API for multimodal extraction
- Detailed 10-rule extraction prompt covering: document classification, discount logic, line items (SKU/HSN extraction), GST breakdown (SGST/CGST/IGST mutual exclusivity), GSTIN extraction, vendor name rules, date formats, PO-specific rules, amount arithmetic chain, confidence scores
- `generate_template()` — given OCR text + validated data, produces an `invoice2data` YAML template
  - Accepts `historical_examples` (up to 5 prior validated docs) to improve template accuracy
  - Accepts `prior_attempt` + `prior_error` for one retry with self-correction feedback
  - Prompt injection defense: OCR fenced in `<<<BEGIN_UNTRUSTED_OCR>>>` / `<<<END_UNTRUSTED_OCR>>>`
  - Detailed 12-rule template generation prompt covering: layout-family thinking, keyword selection, regex anchor rules, character class requirements, self-check loop
- Tenacity retry (3 attempts, exponential backoff 2–30s), quota-exhausted detection bypasses retries and tries fallback model immediately
- Token usage logged per call (prompt + completion + total)
- Configurable concurrency semaphore (default 32 concurrent Gemini calls)

**`invoice2data_wrapper.py`** (`Invoice2DataExtractor`) — Template-based extraction:
- Loads templates from PostgreSQL DB (hot-reload, no restart needed) with SHA-1 content signature cache
- Runs all matching templates, scores each by field count (line items = 2pts, others = 1pt), picks highest
- `postprocess_template_regexes()` — injects `(?si)` inline flags into all field regexes (case-insensitive + DOTALL)
- `verify_template_roundtrip()` — verifies a generated template recovers its source values from the same OCR before saving

**`ocr.py`** — Dual-engine OCR selection:
- Runs pdftotext first; scores output on length, tokens, alnum ratio, domain keyword hits, replacement chars
- Falls back to Tesseract if score < 2.2; picks best of the two by score + 0.25 margin

**`redis_client.py`** (`RedisTaskQueue`) — Crash-safe queue:
- `BRPOP` → `LPUSH :processing` + heartbeat hash (at-least-once delivery)
- `reap_stale()` — moves expired in-flight tasks back to source queue
- `acquire_lock` / `release_lock` — `SET NX EX` distributed locking per document
- `dead_letter()` — failed tasks written to `<queue>:dlq` with reason + timestamp
- `depths()` — returns `LLEN` for each queue (used for monitoring logs)

**`database.py`** (`PostgresDocumentRepository`) — psycopg2 connection pool:
- `get_by_id`, `update`, `get_validated_by_template`
- `SimpleConnectionPool` (min 1, max 5), broken-connection recycling

**`template_repository.py`** (`PostgresTemplateRepository`) — upsert templates to `xcentro.templates`

**`template_seeder.py`** — Seeds filesystem YAML templates into DB on worker startup

**`file_downloader.py`** (`S3FileDownloader`) — Downloads PDF from S3 to a local temp dir; respects `max_bytes` cap

**`config.py`** — Pydantic Settings model with all required env variables

### 4.3 Use Cases Layer (`usecases/`)

**`extraction_service.py`** (`ExtractionService`):

Full pipeline:
1. Fetch document from DB; skip if terminal status (idempotency guard)
2. Download PDF from S3
3. Run dual-engine OCR; persist `raw_ocr`
4. Try template extraction → validate (required fields + GSTIN, no arithmetic)
5. If template fails or validation fails → LLM fallback
6. If `doc_type` is `OTHER` / `UNKNOWN` → mark `FAILED` with violation flag
7. Apply GST normalization rules (mutual exclusivity, inference from state codes, line item tax_rate defaulting, PO invoice_number clearing)
8. Run full validation (+ arithmetic check)
9. Persist `draft_data`, `doc_type`, `processed_by`, `i2d_template`; status → `REVIEW_REQUIRED`

Template generation (`generate_template_for_document`):
1. Skip if document was already extracted by INVOICE2DATA with an existing template
2. Fetch up to 5 historical validated examples from the same template
3. Generate YAML via LLM → postprocess → round-trip verify
4. On verification failure: retry once with failure feedback injected into prompt
5. If retry fails: raise `TemplateGenerationError(retryable=False)` → DLQ
6. On success: upsert to `xcentro.templates`; update `document.i2d_template`

**`validation.py`** — Pure validation functions:
- `check_required_fields()` — Invoices need: `invoice_number`, `invoice_date`, `total_amount`, `vendor_name`, `line_items`. POs need: `po_number`, `vendor_name`, `total_amount`, `line_items`
- `check_gstin()` — Validates 15-char Indian GSTIN regex `^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z][A-Z0-9]Z[A-Z0-9]$`
- `check_arithmetic()` — Verifies `Σ(qty × unit_price + tax) − discount + shipping ≈ total_amount` (±₹0.50)

### 4.4 Entry Point (`main.py`)

- Composition root: wires all adapters and injects into `ExtractionService`
- Seeds filesystem templates into DB at startup
- Runs stale task reaper at startup and every 60s
- Logs queue depths every 60s
- Handles SIGINT/SIGTERM for graceful shutdown (finishes current document before exit)
- Retry loop: up to 3 attempts; re-queues retryable failures; DLQ + `mark_failed()` on exhaustion
- Distributed lock per document prevents duplicate concurrent processing
- Distinguishes extraction tasks (`REDIS_QUEUE`) from template-generation tasks (`REDIS_TEMPLATE_QUEUE`)

### 4.5 Templates (`templates/`)

Five `invoice2data` YAML templates shipped with the worker as seeds:
- `27aatcm5865f1zu_invoice.yml` — Invoice template for GSTIN `27AATCM5865F1ZU`
- `27aatcm5865f1zu_purchase_order.yml` — PO template for same vendor
- `33gsptn0372g1zc_purchase_order.yml` — PO template for GSTIN `33GSPTN0372G1ZC`
- `superstore_template.yml` — Generic superstore invoice format
- `tax_invoice.yml` — Generic tax invoice format

### 4.6 Infrastructure Files

**`Dockerfile.python_worker`** — Multi-stage Docker build:
- Builder stage: Python 3.12-slim + pip install into `/opt/venv`
- Runtime stage: installs `tesseract-ocr` + `poppler-utils` system deps; copies venv; runs as non-root `appuser`

**`python_worker/.env.example`** — Documents all required env vars (Postgres, Redis, S3, Gemini)

---

## 5. Go Backend — Domain Layer

### 5.1 `internal/core/voucher/voucher_document_entity.go` (new, +62 lines)

Defines Go types mirroring the Python worker's status/type model:

**`DocumentStatus` string enum constants:**
- `DocStatusQueued`, `DocStatusOCRProcessing`, `DocStatusPendingLLM`, `DocStatusLLMProcessing`
- `DocStatusReviewRequired`, `DocStatusValidated`, `DocStatusFailed`

**`DocumentType` string enum constants:**
- `DocTypeInvoice`, `DocTypePurchaseOrder`, `DocTypeUnknown`

**Invoice match status constants:**
- `InvoiceMatchPending`, `InvoiceMatchMatched`, `InvoiceMatchOrphan`, `InvoiceMatchMismatch`

**PO status constants:**
- `POStatusOpen`, `POStatusPartiallyInvoiced`, `POStatusClosed`

**`VoucherDocument` struct** — Go representation of `voucher_documents` row (id, s3_path, doc_type, status, raw_ocr, raw_extraction, draft_data, i2d_template, processed_by, validated_data, file_hash, uploaded_at)

### 5.2 `internal/core/voucher/extracted_entities.go` (new, +157 lines)

**`ExtractedInvoice`** — Full typed struct for `invoice` table rows, including match fields (`MatchStatus`, `POID`, `PONumber`)

**`PurchaseOrder`** — Typed struct for `purchase_orders` with `RemainingBalance` and `POStatus`

**`LineItemDetail`** — Typed struct for `line_items` (amounts stored in paise as int64)

**`MatchResult`** — Audit record linking invoice → PO with `MatchDetails` JSONB

**Paginated list filter types:** `DocumentListFilter`, `ExtractedInvoiceListFilter`, `PurchaseOrderListFilter`, `MatchResultListFilter`

**`PaginatedResponse[T]`** — Generic paginated wrapper (Items, Total, Page, PageSize, TotalPages)

**`MatchResultSummary`** — Denormalized join view (match_results + invoice + purchase_orders) for the frontend match results table: includes both PO and invoice amounts, variance calculation, remaining balance, PO status

### 5.3 `internal/core/voucher/voucher_repository.go` (extended, +87 lines)

New interface methods added to `VoucherRepository`:

**Document management:**
- `CreateVoucherDocument(ctx, doc)` — Insert new document row
- `GetDocumentByFileHash(ctx, hash)` — Duplicate detection via SHA-256
- `GetVoucherDocumentByID(ctx, docID)` — Fetch document by primary key
- `UpdateVoucherDocumentValidation(ctx, docID, status, validatedData)` — Post-HITL status + data update
- `GetValidatedDocumentsByTemplate(ctx, templateName, limit)` — For LLM template gen historical examples

**Typed record creation:**
- `CreateInvoiceFromExtraction(ctx, docID, data)` → `uuid.UUID` — Persists validated invoice + line items transactionally
- `CreatePurchaseOrder(ctx, docID, data)` → `uuid.UUID` — Persists validated PO + line items transactionally
- `GetBusinessIDByGSTIN(ctx, gstin)` → `string` — Resolves business from buyer GSTIN

**Listing/pagination:**
- `ListVoucherDocuments(ctx, filter)` → `([]*VoucherDocument, int64, error)`
- `ListExtractedInvoices(ctx, filter)` → `([]*ExtractedInvoice, int64, error)`
- `ListPurchaseOrders(ctx, filter)` → `([]*PurchaseOrder, int64, error)`
- `ListMatchResults(ctx, filter)` → `([]*MatchResultSummary, int64, error)`

**Matching engine support:**
- `GetInvoiceByID`, `GetPurchaseOrderByPONumber`
- `GetLineItemsByPurchaseOrderID`, `GetLineItemsByInvoiceID`
- `UpdateInvoiceMatchStatus`, `UpdateInvoicePOID`
- `UpdatePurchaseOrderBalance`
- `UpdateLineItemQtyReceived`
- `CreateMatchResult`
- `GetOrphanInvoicesByPONumber`, `GetMatchResultsByInvoiceID`

---

## 6. Go Backend — Document Upload Service & Handler

### `internal/services/voucher/document_upload_service.go` (+225 lines)

Handles the document ingestion lifecycle:

1. **Duplicate detection** — SHA-256 hash of the uploaded file content; if a document with the same hash already exists, returns the existing document ID (idempotent upload)
2. **S3 upload** — Streams the file to S3 at `voucher-documents/<business_id>/<uuid>/<filename>`
3. **DB record creation** — Inserts `voucher_documents` row with status `QUEUED`
4. **Redis enqueue** — Pushes `{"document_id": "<uuid>"}` to `xcentro:extraction_tasks`
5. **Presigned URL generation** (`GetDocumentURL`) — Generates a time-limited S3 presigned URL for frontend document preview

### `internal/handlers/http/voucher/document_upload_handler.go` (+118 lines)

`POST /vouchers/upload`:
- Accepts `multipart/form-data` with `file` field
- Validates file size and MIME type (must be `application/pdf`)
- Extracts `business_id` from JWT context
- Calls `DocumentUploadService.UploadDocument()`
- Returns `{"document_id": "<uuid>", "status": "QUEUED"}`

`GET /vouchers/:document_id/url`:
- Returns presigned S3 URL for the document PDF (used by HITL review frontend to show the original document)

---

## 7. Go Backend — Document Review Service (HITL)

### `internal/services/voucher/document_review_service.go` (+492 lines)

Core HITL (Human-in-the-Loop) orchestration service. Handles three major flows:

#### 7.1 Draft Retrieval

`GetDocumentDraft(ctx, docID)` — Returns the `VoucherDocument` with `draft_data` for display.

`GetDocumentDraftWithResolution(ctx, docID, headerBusinessID)` — Enriched version that also:
- Runs `PayeeResolutionService.ResolvePayee()` on the vendor name + GSTIN from draft_data
- Resolves `business_id` from the header claim or by querying `GetBusinessIDByGSTIN()` on the buyer GSTIN
- Returns `DocumentDraftWithResolution{Document, PayeeResolution, BusinessResolution}`

#### 7.2 Document Validation (`ValidateDocument`)

The critical path after a human reviews and corrects the draft:

1. Validate input: document must exist and be in `REVIEW_REQUIRED` status
2. Merge human corrections into `validated_data`
3. Persist validated data + status → `VALIDATED` to `voucher_documents`
4. Branch on document type:
   - **INVOICE** → `CreateInvoiceFromExtraction()` → get `invoiceID` → trigger `MatchingEngine.RunMatchingForInvoice()` asynchronously
   - **PURCHASE_ORDER** → `CreatePurchaseOrder()` → `GetOrphanInvoicesByPONumber()` → re-trigger matching for any previously orphaned invoices that referenced this PO number
5. Enqueue template generation task to `xcentro:template_tasks` (Python worker generates a reusable template)
6. Returns the created record ID and matching status

#### 7.3 Quick Vendor Creation

`QuickCreateVendor(ctx, req)` — Creates a minimal payee without GSTIN verification or bank account:

1. If GSTIN provided and valid: check if a vendor with this GSTIN already exists → reuse existing payeeID
2. If existing payee + existing relation for this business → return existing relation (idempotent)
3. Otherwise, in a transaction:
   - Create `payee_master` row (type=VENDOR, is_active=true)
   - Create `payee_vendor` row (minimal — no GST verification)
   - Create `business_payee_relation` with `account_id = NULL` (bank account added later via standard onboarding)
4. Returns `{relation_id, payee_id}`

---

## 8. Go Backend — PO-Invoice Matching Engine

### `internal/services/voucher/matching_engine.go` (+400 lines)

A deterministic, synchronous matching algorithm triggered after invoice validation.

#### 8.1 Entry Point: `RunMatchingForInvoice(ctx, invoiceID)`

Full flow:
1. **Idempotency check** — if invoice is not `PENDING_MATCH`, skip (already processed)
2. **Orphan check** — if invoice has no `po_number`, immediately mark `ORPHAN_INVOICE` and record audit entry
3. **PO lookup (cache-first)** — checks Redis cache (`xcentro:po_cache:<business_id>:<po_number>`, TTL 5min); falls back to DB via `GetPurchaseOrderByPONumber()`
4. **Orphan if PO not found** — mark `ORPHAN_INVOICE` (no audit record for this case)
5. **Fetch line items** — `GetLineItemsByInvoiceID()` and `GetLineItemsByPurchaseOrderID()`
6. **Header amount check** — `invoice.net_amount ≤ po.remaining_balance` (within 1% tolerance)
7. **Line item matching** — `matchLineItems(invItems, poItems)`
8. **Apply results**:
   - **`MATCHED`**: update invoice match_status + po_id FK; decrement PO remaining_balance; update PO status (`OPEN` → `PARTIALLY_INVOICED` → `CLOSED`); update line item `qty_received`; invalidate PO cache
   - **`MISMATCH`**: update match_status + write audit record with full mismatch details

#### 8.2 Amount Matching: `matchAmount(invoiceAmt, poBalance)`

```
|invoice_amount - po_balance| / po_balance ≤ PriceVarianceThreshold (1%)
```
Also accepts if `invoice_amount ≤ po_balance` (partial invoice).

#### 8.3 Line Item Matching: `matchLineItems(invItems, poItems)`

Two-phase matching:

**Phase 1 — SKU Match (`SKU_MATCH`):**
- For each invoice item with a non-empty SKU, look for a PO item with the same SKU (case-insensitive, whitespace-collapsed)
- Checks quantity and unit_price within 1% tolerance
- Records mismatches separately (qty or price variance)

**Phase 2 — Description Similarity (`DESCRIPTION_SIMILARITY`):**
- For invoice items unmatched by SKU, attempts fuzzy description matching
- Normalizes both strings (lower-case, collapse whitespace, strip punctuation)
- Computes word overlap: `2 × |intersection| / (|A| + |B|)` (Dice coefficient)
- Threshold: similarity ≥ 0.5 (configurable)
- Checks quantity and price within 1% tolerance

**Result:** `LineItemMatchResult{FullyMatched, MatchedPairs, QtyMismatches, PriceVariances, UnmatchedInvoice}`

`FullyMatched = true` only when all invoice line items are matched with no qty/price mismatches.

#### 8.4 PO Cache

- Key: `xcentro:po_cache:<business_id>:<po_number>` (URL-safe, lowercased PO number)
- TTL: 5 minutes
- Stores serialized `PurchaseOrder` JSON
- Invalidated after successful match (balance changed)
- Cache miss silently falls through to DB; cache errors are non-fatal

---

## 9. Go Backend — Payee Resolution Service

### `internal/services/voucher/payee_resolution_service.go` (+175 lines)

Resolves vendors and businesses from extracted document data during HITL review.

#### `ResolvePayee(ctx, vendorName, gstin)`

Two-step resolution with confidence levels:

1. **Exact GSTIN match** (HIGH confidence) — `vendorRepo.FindByGSTIN(ctx, gstin)`; returns `PayeeResolution{PayeeID, VERIFIED, HIGH, "gstin", suggestedData}` where `suggestedData` includes legal_name and gst_status
2. **Fuzzy name match** (LOW confidence) — `vendorRepo.FindByLegalNameFuzzy(ctx, vendorName)`; uses DB ILIKE; returns `NOT_FOUND` if no match, `VERIFIED/LOW` if matched

#### `ResolveBusinessID(ctx, headerBusinessID, buyerGSTIN, getBusinessByGSTIN)`

Resolves the business that owns this document:
1. If `headerBusinessID` is present → `ResolvedVia: "header"` (JWT claim is authoritative)
2. Otherwise → look up business by `buyer_gstin`; `ResolvedVia: "gstin_match"` or `"unresolved"`

**Types returned:**
- `PayeeResolution{PayeeID, Status, Confidence, MatchedVia, SuggestedData}`
- `BusinessResolution{BusinessID, ResolvedVia}`

---

## 10. Go Backend — Repository Layer

### `internal/adapters/database/postgres/voucher/voucher_repository.go` (+1,091 lines)

All 30+ new interface methods implemented in `PostgresVoucherRepository`. Key implementation details:

**`CreateInvoiceFromExtraction`** — Transactional insert: parses JSONB `draft_data`/`validated_data` map into typed columns, converts amounts from float to paise (int64 × 100), inserts invoice row, then calls `insertLineItems()` for each line item.

**`CreatePurchaseOrder`** — Same pattern as above for POs.

**`insertLineItems`** — Shared helper that accepts `data["line_items"]` array and inserts each row into `xcentro.line_items` with exactly one of `purchase_order_id` or `invoice_id` non-null.

**Helper converters:**
- `toFloat64(v)` — safe interface{} → float64
- `toPaise(v)` — float64 → int64 (× 100)
- `parseOptionalUUID(v)` — interface{} → *uuid.UUID
- `optionalString(v)` — interface{} → *string

**`getLineItemsByFilter`** — Shared implementation for fetching line items; includes a fallback to JSONB `draft_data` / `validated_data` for legacy rows where typed columns were 0 due to a now-fixed field-name mismatch.

**`ListMatchResults`** — Complex JOIN query across `match_results`, `invoice`, and `purchase_orders` tables returning a denormalized `MatchResultSummary` with computed `Variance = invoice_amount − po_amount`.

### `internal/adapters/database/postgres/payee/business_payee_relation_repository.go` (+61 lines)

**`FindRelationByBusinessAndPayee(ctx, businessID, payeeID)`** — New method; used by `QuickCreateVendor` to check for duplicate relations before creating a new one.

### `internal/adapters/database/postgres/payee/payee_vendor_repository.go` (+18 lines)

**`FindByLegalNameFuzzy(ctx, name)`** — New method; uses `ILIKE` query for fuzzy payee name resolution.

---

## 11. Go Backend — HTTP Handlers & Routes

### `internal/handlers/http/voucher/document_review_handler.go` (+488 lines)

`DocumentReviewHandler` exposes 10 HTTP endpoints:

| Method | Path | Handler Method | Description |
|---|---|---|---|
| POST | `/vouchers/vendors/quick-create` | `QuickCreateVendor` | Create minimal vendor without bank verification |
| GET | `/vouchers/match-results` | `ListMatchResults` | Paginated joined invoice + PO match view |
| GET | `/vouchers/documents` | `ListDocuments` | Paginated list of voucher documents |
| GET | `/vouchers/invoices` | `ListInvoices` | Paginated list of extracted invoices |
| GET | `/vouchers/invoices/:invoice_id/match-result` | `GetMatchResult` | Match result for a specific invoice |
| GET | `/vouchers/invoices/:invoice_id/line-items` | `GetInvoiceLineItems` | Line items for an invoice |
| GET | `/vouchers/purchase-orders/:po_id/line-items` | `GetPurchaseOrderLineItems` | Line items for a PO |
| GET | `/vouchers/purchase-orders` | `ListPurchaseOrders` | Paginated list of POs |
| GET | `/vouchers/:document_id/draft` | `GetDocumentDraft` | Extracted draft data + payee resolution |
| GET | `/vouchers/:document_id/status` | `GetDocumentStatus` | Polling endpoint for extraction status |
| POST | `/vouchers/:document_id/validate` | `ValidateDocument` | Submit human-corrected data → create typed records + trigger matching |

### `internal/handlers/http/voucher/routes.go` (extended, +18 lines)

`RegisterVoucherRoutes` signature updated to accept `uploadHandler *DocumentUploadHandler` and `reviewHandler *DocumentReviewHandler`. New upload and all 11 review endpoints registered under `/vouchers/*`.

### `cmd/api/routes.go` (+20 lines)

`registerV1VoucherRoutes` updated:
- Initializes S3 client for document uploads
- Constructs `DocumentUploadService` + `DocumentUploadHandler`
- Constructs payee repositories, `MatchingEngine`, `DocumentReviewService`, `DocumentReviewHandler`
- Passes both new handlers into `RegisterVoucherRoutes`

---

## 12. Infrastructure & DevOps

### `Dockerfile.python_worker` (new, +45 lines)

Multi-stage build for the Python worker:
- **Builder stage:** Python 3.12-slim, installs all `requirements.txt` into `/opt/venv`
- **Runtime stage:** Python 3.12-slim, installs `tesseract-ocr` + `poppler-utils`, copies venv, runs as non-root `appuser (uid=1000)`
- `ENTRYPOINT ["python", "main.py"]`

### `deployments/docker/docker-compose.yml` (+29 lines)

New `python_worker` service added:
- Builds from `Dockerfile.python_worker`
- Container name: `xcentro-python-worker`
- `restart: unless-stopped`
- `depends_on: postgres, redis`
- Loads env from `python_worker/.env`
- Overrides `POSTGRES_HOST=postgres` and Redis host for internal Docker networking

### `.gitignore` (+5 lines)

Added: `python_worker/.env`, `python_worker/__pycache__/`, `python_worker/*.pyc`, `*.egg-info/`

---

## 13. Payee Schema Changes (Quick Vendor Creation)

Migration 000060 also alters existing tables to support vendor creation without full verification:

**`xcentro.payee_vendor`:**
- `gstin` column made **nullable** (was NOT NULL)
- `legal_name` column made nullable
- `gst_status` column made nullable

**`xcentro.business_payee_relation`:**
- `account_id` column made **nullable** (was NOT NULL)
- Old composite unique constraint `(business_id, payee_id, account_id)` dropped
- Two new partial unique indexes replace it:
  - `business_payee_relation_bpa_unique` — `(business_id, payee_id, account_id) WHERE account_id IS NOT NULL`
  - `business_payee_relation_bp_pending_unique` — `(business_id, payee_id) WHERE account_id IS NULL`
- FK on `payment_vouchers` that depended on the old unique constraint is dropped first

**Rationale:** Allows a vendor to be created during document review (where only name/GSTIN is known) and have bank account details added later through the standard payee onboarding flow.

---

## 14. File Change Summary Table

| File / Area | Lines Added | Lines Removed | Category |
|---|---|---|---|
| `python_worker/infrastructure/llm_client.py` | 405 | 0 | New: LLM adapter |
| `python_worker/usecases/extraction_service.py` | 362 | 0 | New: Orchestrator |
| `internal/services/voucher/document_review_service.go` | 492 | 0 | New: HITL service |
| `internal/handlers/http/voucher/document_review_handler.go` | 488 | 0 | New: HTTP handler |
| `internal/services/voucher/matching_engine.go` | 400 | 0 | New: Matching engine |
| `internal/adapters/database/postgres/voucher/voucher_repository.go` | 1,091 | 0 | New: Repository impl |
| `internal/adapters/database/postgres/migrations/sql/000060_create_extraction_tables.up.sql` | 236 | 0 | New: DB migration |
| `python_worker/main.py` | 184 | 0 | New: Worker entry point |
| `internal/services/voucher/payee_resolution_service.go` | 175 | 0 | New: Payee resolution |
| `python_worker/infrastructure/redis_client.py` | 77 | 0 | New: Queue adapter |
| `python_worker/usecases/validation.py` | 122 | 0 | New: Validation logic |
| `python_worker/infrastructure/database.py` | 104 | 0 | New: DB adapter |
| `python_worker/README.md` | 100 | 0 | New: Documentation |
| `python_worker/infrastructure/invoice2data_wrapper.py` | 163 | 0 | New: Template extractor |
| `internal/core/voucher/extracted_entities.go` | 157 | 0 | New: Domain entities |
| `internal/services/voucher/document_upload_service.go` | 225 | 0 | New: Upload service |
| `python_worker/infrastructure/llm_client.py` (prompts) | 350+ | 0 | New: Extraction + template gen prompts |
| `internal/core/voucher/voucher_document_entity.go` | 62 | 0 | New: Domain types |
| `internal/core/voucher/voucher_repository.go` | 87 | 0 | Extended: Interface |
| `internal/handlers/http/voucher/routes.go` | 18 | 2 | Extended: Routes |
| `cmd/api/routes.go` | 20 | 1 | Extended: Composition root |
| `Dockerfile.python_worker` | 45 | 0 | New: Container build |
| `deployments/docker/docker-compose.yml` | 29 | 0 | Extended: Service added |
| Templates (5 YAML files) | 201 | 0 | New: Seed templates |
| `.gitignore` | 5 | 0 | Extended |
| **TOTAL** | **~6,012** | **~10** | |

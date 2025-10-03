# Payment Voucher System - Implementation Plan

## Document Purpose
This document preserves the complete context, design decisions, and implementation roadmap for the Payment Voucher System. It serves as the single source of truth for understanding the product flow, technical architecture, and future development phases.

**Created**: 2025-10-02
**Status**: Phase 1 Complete (Database Schema)
**Next Phase**: Phase 2 (Domain Layer)

---

## Table of Contents
1. [Product Context](#product-context)
2. [Core Product Flow](#core-product-flow)
3. [Database Schema](#database-schema)
4. [Implementation Phases](#implementation-phases)
5. [Design Decisions](#design-decisions)
6. [API Endpoints](#api-endpoints)
7. [External Integrations](#external-integrations)
8. [Testing Strategy](#testing-strategy)

---

## Product Context

### What is a Payment Voucher?
A **payment voucher** is an accounting document that serves as the bridge between payment execution and ERP synchronization. It contains:
- Payment details (amount, tax, TDS)
- Payee information (vendor/employee/GST entity/biller)
- Invoice reference
- General ledger codes for accounting
- ERP sync status

### Core Product Journey
1. **Business Onboarding** → Business entity created in system
2. **Payee Management** → Add vendors, employees, GST entities, billers (with GST verification + bank penny drop)
3. **Payment Sources** → Add bank accounts, cards to wallet (with verification)
4. **Payment Initiation** → Select payee, enter amount/invoice details
5. **Voucher Creation** → **DRAFT voucher created** when user clicks "Next" (Page 1 → Page 2 transition)
6. **Payment Execution** → Select payment source, complete payment
7. **Voucher Finalization** → Voucher status updated to `finalized` after payment success
8. **ERP Synchronization** → Background workers sync voucher to ERP systems (Zoho Books, Tally, SAP, etc.)

---

## Core Product Flow

### Two Payment Flows

#### Flow 1: Quick Payment (MVP Focus)
**Use Case**: User wants to pay quickly without uploading invoice

**Steps**:
1. Select payee from list
2. Enter payment details manually:
   - Amount
   - Invoice number (mandatory)
   - Description
   - Expense category
3. Click **"Next"** → **DRAFT voucher created** (status: `draft`)
4. Page 2: Select payment source (bank account/card/UPI)
5. Click **"Pay"** → Payment processed
6. On success → Voucher status updated to `finalized`
7. Background worker picks up voucher and syncs to ERP

**Abandonment Handling**:
- If user closes browser after clicking "Next", voucher stays in `draft`
- System can send reminders for incomplete payments
- Draft vouchers can be resumed later

#### Flow 2: Bill-Based Payment (Future Enhancement)
**Use Case**: User uploads merchant invoice for OCR parsing

**Steps**:
1. Upload invoice PDF
2. OCR parses invoice (Google Vision / AWS Textract)
3. System auto-fills:
   - Amount
   - Invoice number
   - Invoice date
   - Line items with HSN codes
   - Tax breakdown (CGST/SGST/IGST)
4. User reviews and edits if needed
5. Click **"Next"** → **DRAFT voucher created** (with OCR metadata)
6. Rest of flow identical to Quick Payment

**Multi-Bill Payment**:
- User can select 2-3 invoices together
- Click "Pay All" → **One payment transaction, multiple vouchers**
- All vouchers share the same `payment_id`
- Example:
  - Invoice A: ₹10,000 → Voucher 1 (payment_id: ABC123)
  - Invoice B: ₹15,000 → Voucher 2 (payment_id: ABC123)
  - Invoice C: ₹5,000 → Voucher 3 (payment_id: ABC123)
  - Gateway sees: Single ₹30,000 transaction

---

## Database Schema

### Table 1: `payment_vouchers`
**Purpose**: Core voucher record linking payment to accounting/ERP

**Key Fields**:
- **Primary Key**: `voucher_id` (UUID)
- **Unique Identifier**: `voucher_number` (e.g., PV-2024-00001)
- **Business Context**: `business_id`, `payee_id`, `account_id` (composite FK)
- **Invoice Reference**: `invoice_number`, `invoice_date`, `invoice_due_date`
- **Amounts** (all in paise):
  - `gross_amount`: Total before deductions
  - `net_amount`: Amount actually paid
  - `gst_amount`, `cgst_amount`, `sgst_amount`, `igst_amount`
  - `tds_amount`, `fee_amount`, `discount_amount`
- **Tax Details**: `gst_rate`, `tds_rate`, `tds_section` (first-class columns for queries)
- **Status**: `draft` → `finalized` → `posted` → `synced`
- **Payment Link**: `payment_id` (NULL when draft)
- **ERP Integration**:
  - `erp_sync_status`: `not_synced` → `pending` → `synced` / `failed`
  - `erp_reference_id`: ERP system's voucher ID
  - `gl_expense_account`, `gl_bank_account`, `cost_center`
- **JSONB Fields**:
  - `financial_details`: Line items, HSN codes, custom fields
  - `attachments`: Invoice PDFs, supporting documents
  - `erp_metadata`: Vendor-specific ERP fields (Zoho, Tally, SAP)

**Composite Foreign Key**:
```sql
FOREIGN KEY (business_id, payee_id, account_id)
    REFERENCES business_payee_relation(business_id, payee_id, account_id)
```
This ensures the payee-account combination is valid for this business.

**Indexes** (optimized for ERP workers):
- `idx_vouchers_erp_sync_status`: Fast queries for `WHERE erp_sync_status = 'not_synced'`
- `idx_vouchers_date_range`: Date-based accounting period queries
- `idx_vouchers_fiscal_period`: Fiscal year/period reporting

### Table 2: `voucher_approvals`
**Purpose**: Multi-level approval workflow (separate from voucher table)

**Key Fields**:
- `approval_id`: Primary key
- `voucher_id`: FK to voucher
- `approval_level`: 1, 2, 3 for multi-tier approvals
- `approver_user_id`: Who can approve
- `approver_role`: Manager, Finance Head, etc.
- `action`: `pending`, `approved`, `rejected`
- `remarks`: Approval/rejection comments

**Note**: Approval happens at **payment level** (before payment execution), not voucher level. RBAC enforces who can approve payments.

### Table 3: `erp_sync_queue`
**Purpose**: Async queue for background workers to process ERP sync

**Key Fields**:
- `queue_id`: Primary key
- `voucher_id`: FK to voucher
- `sync_direction`: `push` (to ERP) or `pull` (from ERP)
- `queue_status`: `pending` → `processing` → `completed` / `failed`
- `retry_count`, `max_retries`, `next_retry_at`: Retry logic
- `request_payload`, `response_payload`: JSONB for debugging
- `error_code`, `error_message`: Failure details

**Worker Query**:
```sql
SELECT * FROM erp_sync_queue
WHERE queue_status IN ('pending', 'failed')
  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
ORDER BY created_at
LIMIT 100;
```

---

## Implementation Phases

### ✅ Phase 1: Database Schema (COMPLETED)
**Status**: Completed on 2025-10-02

**Deliverables**:
- ✅ Migration file: `000004_create_voucher_tables.up.sql`
- ✅ Rollback migration: `000004_create_voucher_tables.down.sql`
- ✅ Documentation: This file

**Tables Created**:
- `payment_vouchers` (with indexes)
- `voucher_approvals`
- `erp_sync_queue`

**How to Run**:
```bash
go run ./cmd/migrate -action=up
```

---

### Phase 2: Domain Layer (Core Business Logic)
**Priority**: High (Next Phase)

**Directory**: `internal/core/voucher/`

**Files to Create**:
1. `voucher_entity.go`:
   ```go
   type Voucher struct {
       VoucherID       uuid.UUID
       VoucherNumber   string  // PV-2024-00001
       VoucherDate     time.Time
       FiscalYear      string  // FY2024-25
       AccountingPeriod string // 2024-04
       BusinessID      uuid.UUID
       PayeeID         uuid.UUID
       AccountID       uuid.UUID
       InvoiceNumber   string
       GrossAmount     int64   // paise
       NetAmount       int64   // paise
       Status          VoucherStatus
       PaymentID       *uuid.UUID  // NULL when draft
       ERPSyncStatus   ERPSyncStatus
       // ... other fields
   }

   type VoucherStatus string
   const (
       StatusDraft     VoucherStatus = "draft"
       StatusFinalized VoucherStatus = "finalized"
       StatusPosted    VoucherStatus = "posted"
       StatusSynced    VoucherStatus = "synced"
   )

   type ERPSyncStatus string
   const (
       ERPNotSynced ERPSyncStatus = "not_synced"
       ERPPending   ERPSyncStatus = "pending"
       ERPSynced    ERPSyncStatus = "synced"
       ERPFailed    ERPSyncStatus = "failed"
   )
   ```

2. `voucher_repository.go`:
   ```go
   type VoucherRepository interface {
       CreateDraft(ctx context.Context, voucher *Voucher) error
       FinalizeVoucher(ctx context.Context, voucherID uuid.UUID, paymentID uuid.UUID) error
       GetByID(ctx context.Context, voucherID uuid.UUID) (*Voucher, error)
       GetByPaymentID(ctx context.Context, paymentID uuid.UUID) ([]*Voucher, error)
       ListByBusiness(ctx context.Context, businessID uuid.UUID, filters VoucherFilters) ([]*Voucher, error)
       UpdateERPStatus(ctx context.Context, voucherID uuid.UUID, status ERPSyncStatus) error
   }

   type VoucherFilters struct {
       Status        *VoucherStatus
       ERPSyncStatus *ERPSyncStatus
       DateFrom      *time.Time
       DateTo        *time.Time
       FiscalYear    *string
       PayeeID       *uuid.UUID
   }
   ```

3. `voucher_service.go`:
   ```go
   type VoucherService interface {
       GenerateVoucherNumber(ctx context.Context, businessID uuid.UUID, fiscalYear string) (string, error)
       ValidateVoucherData(ctx context.Context, voucher *Voucher) error
       CalculateFiscalYear(date time.Time) string
       CalculateAccountingPeriod(date time.Time) string
   }
   ```

**Business Rules**:
- Voucher number format: `PV-YYYY-NNNNN` (sequential per fiscal year per business)
- Fiscal year calculation: Apr 1 - Mar 31 (Indian financial year)
- Accounting period: `YYYY-MM` format
- Validation: `net_amount = gross_amount - tds_amount - discount_amount + gst_amount`

---

### Phase 3: Repository Implementation
**Priority**: High

**Directory**: `internal/adapters/database/postgres/voucher/`

**Files to Create**:
1. `voucher_repository.go`:
   - Implement `VoucherRepository` interface using pgx
   - SQL queries for CRUD operations
   - Transaction support for multi-voucher creation

**Key Queries**:

**Create Draft Voucher**:
```sql
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id, invoice_number,
    gross_amount, net_amount, gst_amount, tds_amount,
    status, created_by_user_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'draft', $14)
RETURNING voucher_id;
```

**Finalize Voucher**:
```sql
UPDATE xcentro.payment_vouchers
SET status = 'finalized',
    payment_id = $2,
    updated_at = NOW()
WHERE voucher_id = $1 AND status = 'draft';
```

**Get Vouchers Pending ERP Sync**:
```sql
SELECT * FROM xcentro.payment_vouchers
WHERE business_id = $1
  AND status = 'finalized'
  AND erp_sync_status IN ('not_synced', 'failed')
ORDER BY voucher_date DESC;
```

---

### Phase 4: Service Layer (Orchestration)
**Priority**: High

**Directory**: `internal/services/voucher/`

**Files to Create**:
1. `voucher_service.go`:
   ```go
   type VoucherServiceImpl struct {
       voucherRepo VoucherRepository
       paymentRepo PaymentRepository
       payeeRepo   PayeeRepository
   }

   func (s *VoucherServiceImpl) CreateDraftVoucher(ctx context.Context, req CreateDraftVoucherRequest) (*Voucher, error) {
       // 1. Validate payee-account relationship exists
       // 2. Generate voucher number
       // 3. Calculate fiscal year and accounting period
       // 4. Validate amounts
       // 5. Create draft voucher
       // 6. Return voucher with status "draft"
   }

   func (s *VoucherServiceImpl) FinalizeVoucher(ctx context.Context, voucherID uuid.UUID, paymentID uuid.UUID) error {
       // 1. Verify payment exists and is successful
       // 2. Update voucher status to "finalized"
       // 3. Link voucher to payment
       // 4. Add to ERP sync queue (if auto-sync enabled)
   }

   func (s *VoucherServiceImpl) CreateMultiVouchers(ctx context.Context, paymentID uuid.UUID, bills []BillVoucherRequest) ([]*Voucher, error) {
       // For multi-bill payments: Create multiple vouchers with same payment_id
       // Use transaction to ensure atomicity
   }
   ```

**Request/Response Models**:
```go
type CreateDraftVoucherRequest struct {
    BusinessID      uuid.UUID
    PayeeID         uuid.UUID
    AccountID       uuid.UUID
    InvoiceNumber   string
    InvoiceDate     time.Time
    GrossAmount     int64  // paise
    GSTAmount       int64
    TDSAmount       int64
    NetAmount       int64
    ExpenseCategory string
    Description     string
    UserID          uuid.UUID  // created_by
}

type BillVoucherRequest struct {
    InvoiceNumber string
    Amount        int64
    // ... other bill-specific fields
}
```

---

### Phase 5: HTTP Handlers
**Priority**: Medium

**Directory**: `internal/handlers/http/`

**Files to Create/Modify**:
1. `voucher_handler.go`:
   ```go
   type VoucherHandler struct {
       voucherService VoucherService
   }

   func (h *VoucherHandler) CreateDraftVoucher(c echo.Context) error
   func (h *VoucherHandler) GetVoucher(c echo.Context) error
   func (h *VoucherHandler) ListVouchers(c echo.Context) error
   func (h *VoucherHandler) FinalizeVoucher(c echo.Context) error
   ```

2. Update `cmd/api/routes.go`:
   ```go
   v1.POST("/vouchers/draft", voucherHandler.CreateDraftVoucher)
   v1.GET("/vouchers", voucherHandler.ListVouchers)
   v1.GET("/vouchers/:id", voucherHandler.GetVoucher)
   v1.PUT("/vouchers/:id/finalize", voucherHandler.FinalizeVoucher)
   ```

**API Examples**: See [API Endpoints](#api-endpoints) section below.

---

### Phase 6: Payment Integration
**Priority**: High (Must be done with Phase 5)

**Files to Modify**:
1. `internal/services/payments/payment_orchestrator.go`:
   - After successful payment, call `voucherService.FinalizeVoucher()`
   - Update voucher with `payment_id`

**Flow**:
```go
func (p *PaymentOrchestrator) ProcessPayment(ctx context.Context, req PaymentRequest) (*PaymentResponse, error) {
    // ... existing payment logic ...

    if paymentStatus == "success" {
        // Finalize voucher(s) linked to this payment
        if req.VoucherID != nil {
            err := p.voucherService.FinalizeVoucher(ctx, *req.VoucherID, paymentID)
            if err != nil {
                // Log error but don't fail payment
                p.logger.Errorf("Failed to finalize voucher: %v", err)
            }
        }
    }

    return response, nil
}
```

---

### Phase 7: ERP Sync Workers (Background Jobs)
**Priority**: Medium (Can be done after MVP)

**Directory**: `internal/workers/erp_sync/`

**Files to Create**:
1. `erp_sync_worker.go`:
   ```go
   type ERPSyncWorker struct {
       voucherRepo VoucherRepository
       queueRepo   ERPSyncQueueRepository
       erpClient   ERPClient  // Interface for ERP integrations
   }

   func (w *ERPSyncWorker) Start(ctx context.Context) {
       ticker := time.NewTicker(1 * time.Minute)
       for {
           select {
           case <-ticker.C:
               w.processQueue(ctx)
           case <-ctx.Done():
               return
           }
       }
   }

   func (w *ERPSyncWorker) processQueue(ctx context.Context) {
       // 1. Get pending items from erp_sync_queue
       // 2. For each item:
       //    a. Update status to "processing"
       //    b. Call ERP API
       //    c. On success: Update voucher.erp_sync_status = "synced"
       //    d. On failure: Increment retry_count, schedule next retry
       // 3. Exponential backoff for retries
   }
   ```

2. `erp_client_interface.go`:
   ```go
   type ERPClient interface {
       PushVoucher(ctx context.Context, voucher *Voucher) (*ERPResponse, error)
       PullVoucher(ctx context.Context, erpReferenceID string) (*Voucher, error)
   }

   type ZohoBooksClient struct { /* ... */ }
   type TallyClient struct { /* ... */ }
   // ... other ERP implementations
   ```

**Retry Strategy**:
- Retry 1: After 1 minute
- Retry 2: After 5 minutes
- Retry 3: After 15 minutes
- Max retries: 3
- After max retries: Move to manual review queue

---

### Phase 8: Bill Upload & OCR (Future Enhancement)
**Priority**: Low (Post-MVP)

**Directory**: `internal/services/ocr/`

**Files to Create**:
1. `ocr_service.go`:
   ```go
   type OCRService interface {
       ParseInvoice(ctx context.Context, pdfURL string) (*ParsedInvoice, error)
   }

   type ParsedInvoice struct {
       InvoiceNumber string
       InvoiceDate   time.Time
       TotalAmount   int64
       GSTAmount     int64
       LineItems     []LineItem
       VendorGSTIN   string
       // ... other fields
   }
   ```

2. `google_vision_client.go` or `aws_textract_client.go`:
   - Implementation of OCR service using cloud provider APIs

**API Endpoint**:
```bash
POST /v1/bills/upload
```

**Flow**:
1. User uploads invoice PDF
2. Store PDF in S3/Cloud Storage
3. Call OCR service to parse
4. Return parsed data to frontend
5. User reviews and edits
6. Click "Next" → Create draft voucher with OCR metadata

---

## Design Decisions

### 1. Why Composite Foreign Key?
**Question**: "doesn't the business_payee_relation already have account_id?"

**Answer**: The composite FK ensures **data integrity**:
```sql
FOREIGN KEY (business_id, payee_id, account_id)
    REFERENCES business_payee_relation(business_id, payee_id, account_id)
```

This guarantees:
- The payee exists for this business
- The account belongs to this payee
- The business has authorized this payee-account combination

Without composite FK, someone could insert:
- Valid `business_id`
- Valid `payee_id`
- Valid `account_id`
- But the combination might not exist in `business_payee_relation`

**Example**:
- Business A → Payee C → Account 1 ✅
- Business B → Payee C → Account 2 ✅
- Business A → Payee C → Account 2 ❌ (Not in relation table)

Composite FK prevents the last scenario.

---

### 2. Why Separate Approval Table?
**Original Design**: Approval fields in `payment_vouchers` table

**User Feedback**: "I think an approval table separately makes more sense"

**Rationale**:
- Multi-level approvals (Level 1: Manager, Level 2: Finance Head, Level 3: CFO)
- Approval history tracking
- Flexible workflow configuration per business
- Easier to query "pending approvals for user X"

**Outcome**: Created `voucher_approvals` table

---

### 3. Why Columns for Tax/TDS/GL?
**Alternative**: Store all in JSONB `financial_details`

**User Feedback**: "If things critical to ERP are in JSON, will that not hamper the easy discoverability which SQL query offers"

**Rationale**:
- ERP workers need fast queries: `WHERE erp_sync_status = 'not_synced'`
- Accounting reports need: `WHERE fiscal_year = 'FY2024-25' AND tds_section = '194C'`
- JSONB queries are slower and less intuitive
- Tax compliance requires structured data

**Outcome**: Hybrid approach
- **Columns**: Tax amounts, TDS details, GL codes, ERP status (worker queries)
- **JSONB**: Line items, attachments, vendor-specific ERP fields (flexible data)

---

### 4. Why Draft-First Status?
**Original Assumption**: Voucher created after payment success

**User Clarification**: "voucher is created when user clicks 'Next' (before payment)"

**Rationale**:
- User flow: Page 1 (details) → Click Next → **Draft voucher** → Page 2 (payment source)
- If user abandons: Draft voucher exists for reminders
- If payment fails: Voucher stays draft (can retry)
- If payment succeeds: Voucher finalized

**Outcome**: Status flow = `draft` → `finalized` → `posted` → `synced`

---

### 5. Why No Payment Timestamps in Voucher?
**User Feedback**: "payment table entries are supposed to capture all this info of initiation, reconciliation, completion etc. Doesn't make sense here"

**Rationale**:
- `payments` table already has:
  - `initiated_at`
  - `completed_at`
  - `reconciled_at`
- Voucher can join on `payment_id` to get these timestamps
- Avoid data duplication (Single Source of Truth principle)

**Outcome**: Removed payment timestamps from voucher table

---

### 6. Why Separate ERP Sync Queue?
**Alternative**: Use voucher table's `erp_sync_status` directly

**Rationale**:
- Workers need retry logic (retry_count, next_retry_at)
- Need to store request/response payloads for debugging
- Separation of concerns: Voucher = accounting data, Queue = sync orchestration
- Easier to monitor sync failures and performance

**Outcome**: Created `erp_sync_queue` table

---

## API Endpoints

### 1. Create Draft Voucher
**Endpoint**: `POST /v1/vouchers/draft`

**When**: User clicks "Next" on payment details page (Page 1 → Page 2)

**Request**:
```json
{
  "payee_id": "550e8400-e29b-41d4-a716-446655440000",
  "account_id": "660e8400-e29b-41d4-a716-446655440001",
  "invoice_number": "INV-2024-001",
  "invoice_date": "2024-09-15",
  "gross_amount": 118000,
  "gst_amount": 18000,
  "gst_rate": 18.00,
  "tds_amount": 1000,
  "tds_rate": 1.00,
  "tds_section": "194C",
  "net_amount": 117000,
  "expense_category": "Professional Fees",
  "description": "Website development services"
}
```

**Response**:
```json
{
  "voucher_id": "770e8400-e29b-41d4-a716-446655440002",
  "voucher_number": "PV-2024-00123",
  "status": "draft",
  "fiscal_year": "FY2024-25",
  "accounting_period": "2024-09",
  "message": "Draft voucher created. Please complete payment."
}
```

---

### 2. List Vouchers
**Endpoint**: `GET /v1/vouchers`

**Query Parameters**:
- `status`: `draft`, `finalized`, `posted`, `synced`
- `erp_sync_status`: `not_synced`, `pending`, `synced`, `failed`
- `date_from`: `2024-04-01`
- `date_to`: `2024-09-30`
- `fiscal_year`: `FY2024-25`
- `payee_id`: UUID
- `page`: 1
- `limit`: 50

**Example**:
```bash
GET /v1/vouchers?status=finalized&erp_sync_status=not_synced&fiscal_year=FY2024-25
```

**Response**:
```json
{
  "vouchers": [
    {
      "voucher_id": "...",
      "voucher_number": "PV-2024-00123",
      "voucher_date": "2024-09-28",
      "payee_name": "ABC Industries Pvt Ltd",
      "invoice_number": "INV-2024-001",
      "net_amount": 117000,
      "status": "finalized",
      "erp_sync_status": "not_synced"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 150
  }
}
```

---

### 3. Get Voucher Details
**Endpoint**: `GET /v1/vouchers/:voucher_id`

**Response**:
```json
{
  "voucher_id": "770e8400-e29b-41d4-a716-446655440002",
  "voucher_number": "PV-2024-00123",
  "voucher_date": "2024-09-28",
  "fiscal_year": "FY2024-25",
  "accounting_period": "2024-09",
  "payee": {
    "payee_id": "...",
    "display_name": "ABC Industries Pvt Ltd",
    "gstin": "29ABCDE1234F1Z5"
  },
  "bank_account": {
    "account_id": "...",
    "account_number_masked": "****3456",
    "ifsc_code": "HDFC0000123"
  },
  "invoice_number": "INV-2024-001",
  "gross_amount": 118000,
  "gst_amount": 18000,
  "tds_amount": 1000,
  "net_amount": 117000,
  "status": "finalized",
  "payment_id": "880e8400-e29b-41d4-a716-446655440003",
  "erp_sync_status": "synced",
  "erp_reference_id": "ZB-VCH-456",
  "erp_posting_date": "2024-09-28",
  "attachments": {
    "invoice_pdf": {
      "url": "https://s3.../invoice.pdf",
      "uploaded_at": "2024-09-28T10:30:00Z"
    }
  }
}
```

---

### 4. Finalize Voucher (Manual)
**Endpoint**: `PUT /v1/vouchers/:voucher_id/finalize`

**Note**: This is usually automatic (triggered by payment success), but provided for manual override.

**Request**:
```json
{
  "payment_id": "880e8400-e29b-41d4-a716-446655440003"
}
```

**Response**:
```json
{
  "voucher_id": "770e8400-e29b-41d4-a716-446655440002",
  "status": "finalized",
  "message": "Voucher finalized and added to ERP sync queue"
}
```

---

## External Integrations

### 1. ERP Systems (Phase 7)

#### Zoho Books Integration
**API**: https://www.zoho.com/books/api/v3/

**Endpoints**:
- `POST /bills`: Create bill/expense
- `GET /bills/:bill_id`: Get bill details

**Authentication**: OAuth 2.0

**Mapping**:
```json
{
  "vendor_id": "<from payee.erp_metadata.zoho_books.contact_id>",
  "bill_number": "<voucher.voucher_number>",
  "date": "<voucher.voucher_date>",
  "due_date": "<voucher.invoice_due_date>",
  "line_items": "<from voucher.financial_details>",
  "gst_treatment": "business_gst",
  "gst_no": "<from payee.gstin>"
}
```

#### Tally Integration
**API**: Tally XML API (HTTP + XML)

**Endpoints**:
- `POST /tally`: Send XML voucher

**Authentication**: API key

**Mapping**:
```xml
<VOUCHER>
  <DATE><voucher.voucher_date></DATE>
  <VOUCHERNUMBER><voucher.voucher_number></VOUCHERNUMBER>
  <VOUCHERTYPENAME>Payment</VOUCHERTYPENAME>
  <LEDGERNAME><payee.legal_name></LEDGERNAME>
  <AMOUNT><voucher.net_amount></AMOUNT>
</VOUCHER>
```

### 2. OCR Services (Phase 8)

#### Google Vision API
**Use Case**: Invoice text extraction

**Endpoint**: `POST https://vision.googleapis.com/v1/images:annotate`

**Features**:
- Text detection
- Table extraction
- Multi-language support

#### AWS Textract
**Use Case**: Advanced invoice parsing with table detection

**Endpoint**: `POST https://textract.amazonaws.com/`

**Features**:
- Form extraction (key-value pairs)
- Table extraction
- Handwriting recognition

---

## Testing Strategy

### Phase 2-3: Unit Tests
**Files**:
- `internal/core/voucher/voucher_service_test.go`
- `internal/adapters/database/postgres/voucher/voucher_repository_test.go`

**Test Cases**:
1. Voucher number generation (sequential, fiscal year reset)
2. Fiscal year calculation (Apr 1 - Mar 31)
3. Amount validation (net = gross - tds + gst)
4. Draft voucher creation
5. Voucher finalization with payment link

### Phase 4: Integration Tests
**Files**:
- `test/integration/voucher_flow_test.go`

**Test Scenarios**:
1. **Quick Payment Flow**:
   - Create draft voucher
   - Process payment
   - Verify voucher finalized
   - Check ERP queue entry

2. **Multi-Bill Payment Flow**:
   - Create 3 draft vouchers
   - Process single payment
   - Verify all vouchers finalized with same payment_id

3. **Abandonment Scenario**:
   - Create draft voucher
   - Simulate user closing browser
   - Verify voucher stays in draft

### Phase 5: API Tests
**Tool**: Postman / `go test` with HTTP client

**Test Cases**:
1. `POST /v1/vouchers/draft` → 201 Created
2. `GET /v1/vouchers?status=draft` → 200 OK
3. `GET /v1/vouchers/:id` → 200 OK
4. `PUT /v1/vouchers/:id/finalize` → 200 OK

### Phase 7: Worker Tests
**Files**:
- `internal/workers/erp_sync/erp_sync_worker_test.go`

**Test Cases**:
1. Process pending queue items
2. Retry logic with exponential backoff
3. Max retry handling
4. ERP API error handling
5. Concurrent worker processing

---

## Migration Commands

### Run Migration
```bash
go run ./cmd/migrate -action=up
```

### Check Migration Version
```bash
go run ./cmd/migrate -action=version
```

### Rollback Migration
```bash
go run ./cmd/migrate -action=down
```

### Force Version (Fix Dirty State)
```bash
go run ./cmd/migrate -action=force -version=3
```

---

## Next Steps

### Immediate (Phase 2)
1. Create domain entities in `internal/core/voucher/`
2. Define repository interfaces
3. Implement voucher number generation logic
4. Write unit tests

### After Phase 2 (Phase 3)
1. Implement PostgreSQL repository
2. Write SQL queries for CRUD operations
3. Add transaction support for multi-voucher creation
4. Integration tests with test database

### Questions to Clarify Before Phase 2
1. **Voucher Number Format**: Is `PV-2024-00001` acceptable? Or should it include business ID?
2. **Fiscal Year**: Confirm Apr 1 - Mar 31 (Indian FY) or calendar year (Jan 1 - Dec 31)?
3. **Auto-Sync**: Should vouchers auto-add to ERP queue after finalization, or manual trigger?
4. **Approval Workflow**: Is approval mandatory for all payments or configurable per business?
5. **Multi-Currency**: Should we support USD/EUR vouchers, or INR-only for MVP?

---

## Appendix: Example Scenarios

### Scenario 1: Quick Payment for Professional Services
**User**: ABC Corp (business_id: B123)
**Payee**: XYZ Consultants (payee_id: P456, account_id: A789)
**Invoice**: INV-2024-100 for ₹1,00,000 + 18% GST - 1% TDS

**Step 1**: User fills payment form
```
Amount: ₹1,18,000 (gross)
Invoice: INV-2024-100
GST: ₹18,000 (18%)
TDS: ₹1,000 (1%, Section 194J)
Net: ₹1,17,000
```

**Step 2**: User clicks "Next" → API call
```bash
POST /v1/vouchers/draft
{
  "payee_id": "P456",
  "account_id": "A789",
  "invoice_number": "INV-2024-100",
  "gross_amount": 118000,
  "gst_amount": 18000,
  "tds_amount": 1000,
  "net_amount": 117000
}
```

**Step 3**: Draft voucher created
```
Voucher ID: V-DRAFT-001
Voucher Number: PV-2024-00150
Status: draft
```

**Step 4**: User selects payment source and pays

**Step 5**: Payment success → Voucher finalized
```
Voucher ID: V-DRAFT-001
Status: finalized
Payment ID: PAY-2024-5000
ERP Sync Status: not_synced
```

**Step 6**: Worker picks up voucher and syncs to Zoho Books

### Scenario 2: Multi-Bill Payment
**User**: ABC Corp pays 3 vendor invoices together

**Step 1**: User selects 3 bills
```
Bill 1: INV-001 → ₹50,000
Bill 2: INV-002 → ₹30,000
Bill 3: INV-003 → ₹20,000
Total: ₹1,00,000
```

**Step 2**: Click "Next" → 3 draft vouchers created
```
Voucher V1: PV-2024-00151 (INV-001, ₹50,000, status: draft)
Voucher V2: PV-2024-00152 (INV-002, ₹30,000, status: draft)
Voucher V3: PV-2024-00153 (INV-003, ₹20,000, status: draft)
```

**Step 3**: User pays → 1 payment transaction
```
Payment ID: PAY-2024-5001
Amount: ₹1,00,000
Gateway: JioPay
```

**Step 4**: All vouchers finalized with same payment_id
```
Voucher V1: status = finalized, payment_id = PAY-2024-5001
Voucher V2: status = finalized, payment_id = PAY-2024-5001
Voucher V3: status = finalized, payment_id = PAY-2024-5001
```

**Step 5**: 3 separate entries synced to ERP (accounting requires individual bill tracking)

---

## Document Maintenance

**Last Updated**: 2025-10-02
**Updated By**: Claude Code
**Next Review**: After Phase 2 completion

**Change Log**:
- 2025-10-02: Initial document creation (Phase 1 complete)

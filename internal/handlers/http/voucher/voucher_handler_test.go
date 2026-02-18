package voucher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	corevoucher "xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/logger"
	customValidator "xcentro.in/internal/shared/validator"
	voucherSvc "xcentro.in/internal/services/voucher"
)

// ---------------------------------------------------------------------------
// noopVoucherService – satisfies voucherSvc.VoucherService with zero-value
// returns for every method. Embed this and override only what you need so
// that individual test stubs stay small.
//
// Why embed rather than define all 23 methods directly on stubVoucherService?
// Go interfaces require every method to be implemented. The real VoucherService
// takes a DB + Redis connection, so it cannot be used in unit tests. A stub is
// the right approach; embedding the noop base keeps per-test code minimal.
// ---------------------------------------------------------------------------

type noopVoucherService struct{}

func (n *noopVoucherService) CreateDraftVoucher(_ context.Context, _ *voucherSvc.CreateDraftVoucherRequest) (*voucherSvc.CreateDraftVoucherResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) UpdateVoucher(_ context.Context, _ *voucherSvc.UpdateVoucherRequest) (*voucherSvc.UpdateVoucherResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) CreateMultiVouchers(_ context.Context, _ *voucherSvc.CreateMultiVouchersRequest) (*voucherSvc.CreateMultiVouchersResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) FinalizeVoucher(_ context.Context, _ *voucherSvc.FinalizeVoucherRequest) (*voucherSvc.FinalizeVoucherResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetVoucherDetails(_ context.Context, _ uuid.UUID) (*voucherSvc.VoucherDetailsResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) DeleteVoucher(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}
func (n *noopVoucherService) ListVouchersForBusiness(_ context.Context, _ uuid.UUID, _ corevoucher.VoucherFilters) (*voucherSvc.VoucherListResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetDraftReminders(_ context.Context, _ *voucherSvc.GetDraftRemindersRequest) (*voucherSvc.GetDraftRemindersResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) EnqueueForERPSync(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (n *noopVoucherService) FinalizeVoucherByID(_ context.Context, _, _ string) error {
	return nil
}
func (n *noopVoucherService) RequestCancellation(_ context.Context, _ *voucherSvc.RequestCancellationRequest) (*voucherSvc.RequestCancellationResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) ApproveCancellation(_ context.Context, _ *voucherSvc.ApproveCancellationRequest) (*voucherSvc.ApproveCancellationResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetVouchersByPayee(_ context.Context, _ *voucherSvc.GetVouchersByPayeeRequest) (*voucherSvc.GetVouchersByPayeeResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetVoucherList(_ context.Context, _ string, _ *corevoucher.BillPaymentStatus, _, _ int) (*corevoucher.VoucherListResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) RequestApproval(_ context.Context, _ *corevoucher.VoucherApprovalRequest) (*corevoucher.VoucherApprovalResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetApprovalInbox(_ context.Context, _ *corevoucher.VoucherApprovalInboxRequest) (*corevoucher.VoucherApprovalInboxResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetApprovalInboxCounts(_ context.Context, _ string) (*corevoucher.VoucherApprovalInboxCounts, error) {
	return nil, nil
}
func (n *noopVoucherService) BatchApproveVouchers(_ context.Context, _ *corevoucher.BatchApprovalRequest) (*corevoucher.BatchApprovalResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetUnpaidBills(_ context.Context, _ *corevoucher.UnpaidBillsRequest, _ bool) (*corevoucher.UnpaidBillsResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetVoucherCounts(_ context.Context, _ string) (*corevoucher.VoucherCountsResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetVoucherActivity(_ context.Context, _ string) (*corevoucher.VoucherActivityResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetVoucherPaymentDetails(_ context.Context, _ string) (*voucherSvc.VoucherPaymentDetailsResponse, error) {
	return nil, nil
}
func (n *noopVoucherService) GetCalendarView(_ context.Context, _ *corevoucher.CalendarViewRequest) (*corevoucher.CalendarViewResponse, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// stubVoucherService – embeds noopVoucherService and overrides the three
// methods used by happy-path test cases. All other methods fall through to
// the noop base. Validation-failure cases return before the service is called.
// ---------------------------------------------------------------------------

type stubVoucherService struct {
	noopVoucherService
}

func (s *stubVoucherService) CreateDraftVoucher(_ context.Context, _ *voucherSvc.CreateDraftVoucherRequest) (*voucherSvc.CreateDraftVoucherResponse, error) {
	return &voucherSvc.CreateDraftVoucherResponse{
		VoucherID: uuid.New(),
		Status:    corevoucher.StatusDraft,
		Message:   "draft created",
	}, nil
}

func (s *stubVoucherService) UpdateVoucher(_ context.Context, _ *voucherSvc.UpdateVoucherRequest) (*voucherSvc.UpdateVoucherResponse, error) {
	return &voucherSvc.UpdateVoucherResponse{
		VoucherID:     uuid.New(),
		Status:        corevoucher.StatusDraft,
		Message:       "updated",
		UpdatedFields: []string{},
	}, nil
}

func (s *stubVoucherService) CreateMultiVouchers(_ context.Context, _ *voucherSvc.CreateMultiVouchersRequest) (*voucherSvc.CreateMultiVouchersResponse, error) {
	return &voucherSvc.CreateMultiVouchersResponse{
		Vouchers: []voucherSvc.CreateDraftVoucherResponse{},
		Message:  "multi vouchers created",
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestEcho() *echo.Echo {
	e := echo.New()
	e.Validator = customValidator.NewCustomValidator()
	return e
}

func newTestLogger() *logger.Logger {
	return logger.New(logger.Config{
		Level:  logger.ErrorLevel,
		Format: logger.TextFormat,
		Output: io.Discard,
	})
}

// responseBody decodes the JSON response body into a generic map.
func responseBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return body
}

// validRelationID is a stable test UUID used wherever a valid relation_id is needed.
var validRelationID = uuid.New().String()

// validBusinessID is a stable test UUID used in X-Business-ID header.
var validBusinessID = uuid.New().String()

// validUserID is a stable test UUID injected as JWT context userId.
var validUserID = uuid.New().String()

// validVoucherID is used as the /:id path parameter in update tests.
var validVoucherID = uuid.New().String()

// dateInRange returns a YYYY-MM-DD string for a date offset in months from today.
func dateInRange(monthOffset int) string {
	return time.Now().AddDate(0, monthOffset, 0).Format("2006-01-02")
}

// ---------------------------------------------------------------------------
// CreateDraftVoucher
// ---------------------------------------------------------------------------

func TestCreateDraftVoucher_Validation(t *testing.T) {
	validBody := fmt.Sprintf(`{
		"relation_id":    %q,
		"invoice_number": "INV-001",
		"gross_amount":   50000,
		"net_amount":     50000,
		"currency":       "INR"
	}`, validRelationID)

	tests := []struct {
		name          string
		businessIDHdr string
		userIDCtx     string
		body          string
		wantStatus    int
		wantError     string
	}{
		// --- Header / auth checks -----------------------------------------------
		{
			name:          "missing X-Business-ID header",
			businessIDHdr: "",
			userIDCtx:     validUserID,
			body:          validBody,
			wantStatus:    http.StatusBadRequest,
			wantError:     "MISSING_BUSINESS_ID",
		},
		{
			name:          "invalid X-Business-ID (not a UUID)",
			businessIDHdr: "not-a-uuid",
			userIDCtx:     validUserID,
			body:          validBody,
			wantStatus:    http.StatusBadRequest,
			wantError:     "INVALID_BUSINESS_ID",
		},
		{
			name:          "missing userId in context",
			businessIDHdr: validBusinessID,
			userIDCtx:     "", // intentionally empty – simulates missing JWT
			body:          validBody,
			wantStatus:    http.StatusUnauthorized,
			wantError:     "MISSING_USER_ID",
		},
		// --- Structural / type checks -------------------------------------------
		{
			name:          "malformed JSON body",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{invalid json`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid_request",
		},
		// --- Required field checks ----------------------------------------------
		{
			name:          "missing relation_id",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"currency":"INR"}`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "missing invoice_number",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"relation_id":%q,"gross_amount":50000,"net_amount":50000,"currency":"INR"}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "gross_amount is zero",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV-001","gross_amount":0,"net_amount":50000,"currency":"INR"}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "net_amount is zero",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":0,"currency":"INR"}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "invalid currency",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"currency":"GBP"}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		// --- Injection / safe_text checks ---------------------------------------
		// safe_text blocks: < > NUL (0x00) and control chars < 0x20
		// (except \n \r \t which are explicitly allowed)
		{
			name:          "description contains <script> XSS",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"currency":"INR","description":"<script>alert(1)<\/script>"}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "description contains SVG XSS vector",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"currency":"INR","description":"<svg onload=alert(1)>"}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "invoice_number contains HTML tag",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			// Tests safe_text on invoice_number, not just description
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"<div>INV<\/div>","gross_amount":50000,"net_amount":50000,"currency":"INR"}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "invoice_number contains null byte (\\u0000)",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			// JSON decoder converts \u0000 to actual NUL byte (0x00); safe_text blocks it
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV\u0000injection","gross_amount":50000,"net_amount":50000,"currency":"INR"}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "description contains control character (\\u0001 SOH)",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			// \u0001 (ASCII SOH, 0x01) is a control char blocked by safe_text
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"currency":"INR","description":"hello\u0001world"}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "description with only closing angle bracket",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			// > alone (without <) must also be blocked
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"currency":"INR","description":"amount > 0"}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		// --- Date range checks --------------------------------------------------
		{
			name:          "invoice_date more than 2 months in the past",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"currency":"INR","invoice_date":%q}`, validRelationID, dateInRange(-3)),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "invoice_date more than 2 months in the future",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"currency":"INR","invoice_date":%q}`, validRelationID, dateInRange(3)),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "invoice_due_date before invoice_date",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body: fmt.Sprintf(`{
				"relation_id":      %q,
				"invoice_number":   "INV-001",
				"gross_amount":     50000,
				"net_amount":       50000,
				"currency":         "INR",
				"invoice_date":     %q,
				"invoice_due_date": %q
			}`, validRelationID, dateInRange(0), dateInRange(-1)),
			wantStatus: http.StatusBadRequest,
			wantError:  "VALIDATION_FAILED",
		},
		// --- Whitelist: safe chars that must pass --------------------------------
		{
			name:          "description with newlines and tabs is allowed",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			// \n and \t are explicitly whitelisted in validateSafeText
			body:          fmt.Sprintf(`{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"currency":"INR","description":"Line1\nLine2\tTabbed"}`, validRelationID),
			wantStatus:    http.StatusCreated,
			wantError:     "",
		},
		// --- Happy path ---------------------------------------------------------
		{
			name:          "valid request reaches service",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          validBody,
			wantStatus:    http.StatusCreated,
			wantError:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEcho()
			req := httptest.NewRequest(http.MethodPost, "/vouchers/draft", strings.NewReader(tt.body))
			req.Header.Set(echo.MIMEApplicationJSON, "application/json")
			req.Header.Set("Content-Type", "application/json")
			if tt.businessIDHdr != "" {
				req.Header.Set("X-Business-ID", tt.businessIDHdr)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tt.userIDCtx != "" {
				c.Set("userId", tt.userIDCtx)
			}

			handler := NewVoucherHandler(&stubVoucherService{}, newTestLogger())
			if err := handler.CreateDraftVoucher(c); err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status: want %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			if tt.wantError != "" {
				body := responseBody(t, rec)
				if body["error"] != tt.wantError {
					t.Errorf("error code: want %q, got %q", tt.wantError, body["error"])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UpdateVoucher
// ---------------------------------------------------------------------------

func TestUpdateVoucher_Validation(t *testing.T) {
	tests := []struct {
		name          string
		voucherIDPath string
		businessIDHdr string
		userIDCtx     string
		body          string
		wantStatus    int
		wantError     string
	}{
		// --- Path / header / structural checks ----------------------------------
		{
			name:          "invalid voucher_id path param",
			voucherIDPath: "not-a-uuid",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{"gross_amount": 50000}`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid_voucher_id",
		},
		{
			name:          "malformed JSON body",
			voucherIDPath: validVoucherID,
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{bad json`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid_request",
		},
		// --- Field constraint checks --------------------------------------------
		{
			name:          "gross_amount is zero when explicitly provided",
			voucherIDPath: validVoucherID,
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{"gross_amount": 0}`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "invalid currency when provided",
			voucherIDPath: validVoucherID,
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{"currency": "EUR"}`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		// --- Injection / safe_text checks ---------------------------------------
		{
			name:          "description with <img> XSS",
			voucherIDPath: validVoucherID,
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{"description": "<img src=x onerror=alert(1)>"}`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "description with null byte injection",
			voucherIDPath: validVoucherID,
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			// \u0000 in JSON value decodes to NUL byte (0x00)
			body:          `{"description": "clean\u0000text"}`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "invoice_number with closing angle bracket only",
			voucherIDPath: validVoucherID,
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			// Verifies > is blocked even without an accompanying <
			body:          `{"invoice_number": "INV>001"}`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		// --- Date range checks --------------------------------------------------
		{
			name:          "invoice_date out of allowed range",
			voucherIDPath: validVoucherID,
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"invoice_date": %q}`, dateInRange(-3)),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "invoice_due_date before invoice_date",
			voucherIDPath: validVoucherID,
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"invoice_date":%q,"invoice_due_date":%q}`, dateInRange(0), dateInRange(-1)),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		// --- Happy path ---------------------------------------------------------
		{
			name:          "valid partial update reaches service",
			voucherIDPath: validVoucherID,
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{"currency": "INR"}`,
			wantStatus:    http.StatusOK,
			wantError:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEcho()
			req := httptest.NewRequest(http.MethodPut, "/vouchers/"+tt.voucherIDPath, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.businessIDHdr != "" {
				req.Header.Set("X-Business-ID", tt.businessIDHdr)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(tt.voucherIDPath)
			if tt.userIDCtx != "" {
				c.Set("userId", tt.userIDCtx)
			}

			handler := NewVoucherHandler(&stubVoucherService{}, newTestLogger())
			if err := handler.UpdateVoucher(c); err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status: want %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			if tt.wantError != "" {
				body := responseBody(t, rec)
				if body["error"] != tt.wantError {
					t.Errorf("error code: want %q, got %q", tt.wantError, body["error"])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CreateMultiVouchers
// ---------------------------------------------------------------------------

func TestCreateMultiVouchers_Validation(t *testing.T) {
	validSingleBill := fmt.Sprintf(`{
		"relation_id":    %q,
		"invoice_number": "INV-001",
		"gross_amount":   50000,
		"net_amount":     50000
	}`, validRelationID)

	tests := []struct {
		name          string
		businessIDHdr string
		userIDCtx     string
		body          string
		wantStatus    int
		wantError     string
	}{
		// --- Header / structural checks -----------------------------------------
		{
			name:          "missing X-Business-ID header",
			businessIDHdr: "",
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"vouchers":[%s]}`, validSingleBill),
			wantStatus:    http.StatusBadRequest,
			wantError:     "MISSING_BUSINESS_ID",
		},
		{
			name:          "empty vouchers array",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{"vouchers":[]}`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "vouchers field missing entirely",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{}`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		// --- Nested bill field checks (dive tag) --------------------------------
		{
			name:          "bill missing relation_id",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          `{"vouchers":[{"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000}]}`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "bill gross_amount is zero",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"vouchers":[{"relation_id":%q,"invoice_number":"INV-001","gross_amount":0,"net_amount":50000}]}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		// --- Injection / safe_text checks on nested bill fields -----------------
		{
			name:          "bill invoice_number contains HTML tag",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			// Tests that safe_text is enforced on invoice_number inside each bill
			body:          fmt.Sprintf(`{"vouchers":[{"relation_id":%q,"invoice_number":"<b>INV<\/b>","gross_amount":50000,"net_amount":50000}]}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		{
			name:          "bill description contains control character",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			// \u001f (ASCII 0x1F, Unit Separator) is a control char blocked by safe_text
			body:          fmt.Sprintf(`{"vouchers":[{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"description":"vendor\u001fpayment"}]}`, validRelationID),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		// --- Date range check on nested bill ------------------------------------
		{
			name:          "bill invoice_date out of range",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"vouchers":[{"relation_id":%q,"invoice_number":"INV-001","gross_amount":50000,"net_amount":50000,"invoice_date":%q}]}`, validRelationID, dateInRange(-3)),
			wantStatus:    http.StatusBadRequest,
			wantError:     "VALIDATION_FAILED",
		},
		// --- Happy path ---------------------------------------------------------
		{
			name:          "valid multi-voucher request reaches service",
			businessIDHdr: validBusinessID,
			userIDCtx:     validUserID,
			body:          fmt.Sprintf(`{"vouchers":[%s]}`, validSingleBill),
			wantStatus:    http.StatusCreated,
			wantError:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEcho()
			req := httptest.NewRequest(http.MethodPost, "/vouchers/multi", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.businessIDHdr != "" {
				req.Header.Set("X-Business-ID", tt.businessIDHdr)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tt.userIDCtx != "" {
				c.Set("userId", tt.userIDCtx)
			}

			handler := NewVoucherHandler(&stubVoucherService{}, newTestLogger())
			if err := handler.CreateMultiVouchers(c); err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status: want %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			if tt.wantError != "" {
				body := responseBody(t, rec)
				if body["error"] != tt.wantError {
					t.Errorf("error code: want %q, got %q", tt.wantError, body["error"])
				}
			}
		})
	}
}

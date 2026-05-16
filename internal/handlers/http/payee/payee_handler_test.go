package payee

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/shared/logger"
	customValidator "xcentro.in/internal/shared/validator"
	payeeSvc "xcentro.in/internal/services/payee"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestEcho returns an Echo instance with the shared custom validator registered,
// matching exactly how the production server registers it in cmd/api/http_server.go.
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

// responseBody decodes the JSON response into a generic map.
func responseBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return body
}

// newPayeeHandler builds a PayeeHandler with a nil service.
// All test cases in this file exercise validation gates that return before
// the service is ever called, so nil is safe here.
func newPayeeHandler() *PayeeHandler {
	return NewPayeeHandler((*payeeSvc.PayeeService)(nil), newTestLogger())
}

const validBusinessID = "b1a2c3d4-e5f6-7890-abcd-ef1234567890"

// ---------------------------------------------------------------------------
// OnboardPayee – tests that improper requests are rejected before the service
// is called. The payee handler uses a manual switch-based validation (not
// c.Validate), so the gate tests cover: missing bid in JWT context, missing JWT,
// bad type, type mismatch, and malformed JSON.
// ---------------------------------------------------------------------------

func TestOnboardPayee_Validation(t *testing.T) {
	// A structurally valid vendor payload used for the positive gate tests.
	validVendorBody := `{
		"payee_type": "vendor",
		"display_name": "ACME Supplies",
		"vendor": {
			"gstin":              "27AAPFU0939F1ZV",
			"account_number":     "123456789",
			"ifsc_code":          "HDFC0001234",
			"account_holder_name":"ACME Supplies Ltd",
			"bank_name":          "HDFC Bank",
			"branch_name":        "MG Road"
		}
	}`

	tests := []struct {
		name       string
		businessID string
		userIDCtx  string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing bid in JWT context",
			businessID: "",
			userIDCtx:  "user-uuid-001",
			body:       validVendorBody,
			wantStatus: http.StatusUnauthorized,
			wantError:  "NO_BUSINESS_CONTEXT",
		},
		{
			name:       "missing userId in JWT context",
			businessID: validBusinessID,
			userIDCtx:  "", // intentionally empty
			body:       validVendorBody,
			wantStatus: http.StatusUnauthorized,
			wantError:  "MISSING_USER_ID",
		},
		{
			name:       "malformed JSON body",
			businessID: validBusinessID,
			userIDCtx:  "user-uuid-001",
			body:       `{not valid json`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_request",
		},
		{
			name:       "unsupported payee_type",
			businessID: validBusinessID,
			userIDCtx:  "user-uuid-001",
			body:       `{"payee_type":"unknown_type","display_name":"Test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_payee_type",
		},
		{
			name:       "payee_type vendor but vendor object missing",
			businessID: validBusinessID,
			userIDCtx:  "user-uuid-001",
			body:       `{"payee_type":"vendor","display_name":"ACME"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_request",
		},
		{
			name:       "payee_type employee but employee object missing",
			businessID: validBusinessID,
			userIDCtx:  "user-uuid-001",
			body:       `{"payee_type":"employee","display_name":"John Doe"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_request",
		},
		{
			name:       "payee_type biller but biller object missing",
			businessID: validBusinessID,
			userIDCtx:  "user-uuid-001",
			body:       `{"payee_type":"biller","display_name":"Electricity Board"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_request",
		},
		{
			name:       "payee_type gst_payment but gst object missing",
			businessID: validBusinessID,
			userIDCtx:  "user-uuid-001",
			body:       `{"payee_type":"gst_payment","display_name":"GST Payment"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEcho()
			req := httptest.NewRequest(http.MethodPost, "/payees", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tt.businessID != "" {
				c.Set("bid", tt.businessID)
			}
			if tt.userIDCtx != "" {
				c.Set("userId", tt.userIDCtx)
			}

			if err := newPayeeHandler().OnboardPayee(c); err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status: want %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			body := responseBody(t, rec)
			if body["error"] != tt.wantError {
				t.Errorf("error code: want %q, got %q", tt.wantError, body["error"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetPayees – validates the payee type filter query parameter.
// ---------------------------------------------------------------------------

func TestGetPayees_Validation(t *testing.T) {
	tests := []struct {
		name           string
		businessID     string
		typeQueryParam string
		wantStatus     int
		wantError      string
	}{
		{
			name:           "missing bid in JWT context",
			businessID:     "",
			typeQueryParam: "",
			wantStatus:     http.StatusUnauthorized,
			wantError:      "NO_BUSINESS_CONTEXT",
		},
		{
			name:           "invalid payee type filter",
			businessID:     validBusinessID,
			typeQueryParam: "unknown",
			wantStatus:     http.StatusBadRequest,
			wantError:      "invalid_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEcho()
			target := "/payees"
			if tt.typeQueryParam != "" {
				target += "?type=" + tt.typeQueryParam
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tt.businessID != "" {
				c.Set("bid", tt.businessID)
			}
			c.Set("userId", "user-uuid-001")

			if err := newPayeeHandler().GetPayees(c); err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status: want %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			body := responseBody(t, rec)
			if body["error"] != tt.wantError {
				t.Errorf("error code: want %q, got %q", tt.wantError, body["error"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UpdatePayee – validates JWT context and body gates.
// Note: UpdatePayee currently uses an anonymous inline struct without
// validate tags, so only structural gates (missing bid in JWT, malformed JSON)
// are enforced at the handler level.
// ---------------------------------------------------------------------------

func TestUpdatePayee_Validation(t *testing.T) {
	tests := []struct {
		name          string
		payeeIDPath   string
		accountIDPath string
		businessID    string
		body          string
		wantStatus    int
		wantError     string
	}{
		{
			name:          "missing bid in JWT context",
			payeeIDPath:   "payee-uuid-001",
			accountIDPath: "account-uuid-001",
			businessID:    "",
			body:          `{"display_name":"ACME"}`,
			wantStatus:    http.StatusUnauthorized,
			wantError:     "NO_BUSINESS_CONTEXT",
		},
		{
			name:          "malformed JSON body",
			payeeIDPath:   "payee-uuid-001",
			accountIDPath: "account-uuid-001",
			businessID:    validBusinessID,
			body:          `{bad json`,
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEcho()
			path := "/payees/" + tt.payeeIDPath + "/accounts/" + tt.accountIDPath
			req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("payee_id", "account_id")
			c.SetParamValues(tt.payeeIDPath, tt.accountIDPath)
			if tt.businessID != "" {
				c.Set("bid", tt.businessID)
			}
			c.Set("userId", "user-uuid-001")

			if err := newPayeeHandler().UpdatePayee(c); err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status: want %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			body := responseBody(t, rec)
			if body["error"] != tt.wantError {
				t.Errorf("error code: want %q, got %q", tt.wantError, body["error"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// OnboardVendorData – validate tags on the nested vendor struct.
// Exercises the validate tags (gstin, ifsc) that are defined on
// OnboardVendorData. These fire via c.Validate on the top-level request
// if the handler called c.Validate — but since it currently doesn't,
// these tests document the gap and will pass once c.Validate is wired in.
// For now, they verify the tag declarations exist and are structurally valid.
// ---------------------------------------------------------------------------

func TestOnboardVendorData_ValidateTags(t *testing.T) {
	tests := []struct {
		name      string
		data      payee.OnboardVendorData
		wantValid bool
	}{
		{
			name: "valid vendor data",
			data: payee.OnboardVendorData{
				GSTIN:    "27AAPFU0939F1ZV",
				IFSCCode: "HDFC0001234",
			},
			wantValid: true,
		},
		{
			name: "invalid GSTIN format",
			data: payee.OnboardVendorData{
				GSTIN:    "INVALID",
				IFSCCode: "HDFC0001234",
			},
			wantValid: false,
		},
		{
			name: "invalid IFSC format",
			data: payee.OnboardVendorData{
				GSTIN:    "27AAPFU0939F1ZV",
				IFSCCode: "INVALID",
			},
			wantValid: false,
		},
	}

	v := customValidator.NewCustomValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(&tt.data)
			if tt.wantValid && err != nil {
				t.Errorf("expected valid but got error: %v", err)
			}
			if !tt.wantValid && err == nil {
				t.Error("expected validation error but got nil")
			}
		})
	}
}

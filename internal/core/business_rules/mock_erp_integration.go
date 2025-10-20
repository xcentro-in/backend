package business_rules

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MockERPIntegrationService is a mock implementation for development and testing
type MockERPIntegrationService struct {
	// Simulate delays
	DelayMilliseconds int

	// Simulate errors
	ShouldFail       bool
	FailureErrorCode string
}

// NewMockERPIntegrationService creates a new mock ERP integration service
func NewMockERPIntegrationService() ERPIntegrationService {
	return &MockERPIntegrationService{
		DelayMilliseconds: 500, // Simulate 500ms API delay
		ShouldFail:        false,
	}
}

// PushVoucher simulates pushing a voucher to ERP
func (m *MockERPIntegrationService) PushVoucher(ctx context.Context, request *ERPSyncRequest) (*ERPSyncResponse, error) {
	// Simulate API delay
	if m.DelayMilliseconds > 0 {
		time.Sleep(time.Duration(m.DelayMilliseconds) * time.Millisecond)
	}

	// Simulate failure if configured
	if m.ShouldFail {
		errorCode := m.FailureErrorCode
		if errorCode == "" {
			errorCode = ERPErrorServerError
		}
		return &ERPSyncResponse{
				Success:      false,
				ErrorCode:    errorCode,
				ErrorMessage: "Mock ERP integration failure (simulated)",
			}, &ERPIntegrationError{
				Provider:     request.Provider,
				Operation:    "push",
				ErrorCode:    errorCode,
				ErrorMessage: "Mock ERP integration failure (simulated)",
				Retryable:    errorCode == ERPErrorNetworkTimeout || errorCode == ERPErrorServerError,
			}
	}

	// Simulate successful sync
	erpReferenceID := generateMockERPReferenceID(request.Provider, request.Voucher.VoucherNumber)

	return &ERPSyncResponse{
		Success:        true,
		ERPReferenceID: erpReferenceID,
		PostingDate:    time.Now(),
		Message:        fmt.Sprintf("Voucher %s synced successfully to %s", request.Voucher.VoucherNumber, request.Provider),
		RawResponse: map[string]interface{}{
			"mock":       true,
			"voucher_id": erpReferenceID,
			"status":     "posted",
			"timestamp":  time.Now().Format(time.RFC3339),
		},
	}, nil
}

// PullVoucher simulates pulling a voucher from ERP
func (m *MockERPIntegrationService) PullVoucher(ctx context.Context, provider ERPProvider, erpReferenceID string, config map[string]interface{}) (*ERPVoucher, error) {
	// Simulate API delay
	if m.DelayMilliseconds > 0 {
		time.Sleep(time.Duration(m.DelayMilliseconds) * time.Millisecond)
	}

	// Simulate failure if configured
	if m.ShouldFail {
		return nil, &ERPIntegrationError{
			Provider:     provider,
			Operation:    "pull",
			ErrorCode:    ERPErrorServerError,
			ErrorMessage: "Mock ERP integration failure (simulated)",
			Retryable:    true,
		}
	}

	// Return a mock voucher
	now := time.Now()
	return &ERPVoucher{
		BusinessID:    uuid.Must(uuid.NewV7()),
		VoucherID:     uuid.Must(uuid.NewV7()),
		VoucherNumber: "PV-2024-MOCK",
		VoucherDate:   now,
		VoucherType:   "payment",
		PayeeName:     "Mock Vendor Pvt Ltd",
		PayeeGSTIN:    "29ABCDE1234F1Z5",
		InvoiceNumber: "INV-MOCK-001",
		InvoiceDate:   &now,
		GrossAmount:   118000,
		NetAmount:     117000,
		GSTAmount:     18000,
		GSTRate:       18.0,
		CGSTAmount:    9000,
		SGSTAmount:    9000,
		TDSAmount:     1000,
		TDSRate:       1.0,
		TDSSection:    "194C",
		Metadata: map[string]interface{}{
			"mock":             true,
			"erp_reference_id": erpReferenceID,
		},
	}, nil
}

// ValidateConnection simulates validating ERP connection
func (m *MockERPIntegrationService) ValidateConnection(ctx context.Context, provider ERPProvider, config map[string]interface{}) error {
	// Simulate API delay
	if m.DelayMilliseconds > 0 {
		time.Sleep(time.Duration(m.DelayMilliseconds) * time.Millisecond)
	}

	// Simulate failure if configured
	if m.ShouldFail {
		return &ERPIntegrationError{
			Provider:     provider,
			Operation:    "validate",
			ErrorCode:    ERPErrorAuthFailed,
			ErrorMessage: "Invalid credentials (simulated)",
			Retryable:    false,
		}
	}

	// Check if required config fields are present
	if config == nil {
		return &ERPIntegrationError{
			Provider:     provider,
			Operation:    "validate",
			ErrorCode:    ERPErrorNotConfigured,
			ErrorMessage: "ERP configuration is missing",
			Retryable:    false,
		}
	}

	return nil
}

// GetSupportedProviders returns list of supported providers
func (m *MockERPIntegrationService) GetSupportedProviders() []ERPProvider {
	return []ERPProvider{
		ERPProviderZohoBooks,
		ERPProviderTally,
		ERPProviderSAP,
		ERPProviderQuickBooks,
		ERPProviderCustom,
	}
}

// TestConnection simulates testing ERP connection
func (m *MockERPIntegrationService) TestConnection(ctx context.Context, provider ERPProvider, config map[string]interface{}) (bool, error) {
	// Simulate API delay
	if m.DelayMilliseconds > 0 {
		time.Sleep(time.Duration(m.DelayMilliseconds) * time.Millisecond)
	}

	// Simulate failure if configured
	if m.ShouldFail {
		return false, &ERPIntegrationError{
			Provider:     provider,
			Operation:    "test",
			ErrorCode:    ERPErrorNetworkTimeout,
			ErrorMessage: "Connection timeout (simulated)",
			Retryable:    true,
		}
	}

	return true, nil
}

// Helper function to generate mock ERP reference IDs
func generateMockERPReferenceID(provider ERPProvider, voucherNumber string) string {
	switch provider {
	case ERPProviderZohoBooks:
		return fmt.Sprintf("ZB-VCH-%d", time.Now().Unix()%10000)
	case ERPProviderTally:
		return fmt.Sprintf("TALLY-%s", voucherNumber)
	case ERPProviderSAP:
		return fmt.Sprintf("SAP-DOC-%d", time.Now().Unix()%100000)
	case ERPProviderQuickBooks:
		return fmt.Sprintf("QB-BILL-%d", time.Now().Unix()%10000)
	default:
		return fmt.Sprintf("ERP-%s", uuid.Must(uuid.NewV7()).String()[:8])
	}
}

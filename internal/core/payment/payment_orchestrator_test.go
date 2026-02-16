package payment

import (
	"io"
	"testing"

	"xcentro.in/internal/shared/logger"
)

// TestSelectGateway_CorporateCard tests that all corporate cards route to JioPay
func TestSelectGateway_CorporateCard(t *testing.T) {
	tests := []struct {
		name         string
		cardType     CardType
		amount       int64
		expectedGW   GatewayType
	}{
		{
			name:       "Corporate Visa → JioPay",
			cardType:   CardTypeVisa,
			amount:     50000, // ₹500
			expectedGW: GatewayJioPay,
		},
		{
			name:       "Corporate Mastercard → JioPay",
			cardType:   CardTypeMastercard,
			amount:     100000, // ₹1000
			expectedGW: GatewayJioPay,
		},
		{
			name:       "Corporate RuPay → JioPay",
			cardType:   CardTypeRupay,
			amount:     200000, // ₹2000
			expectedGW: GatewayJioPay,
		},
		{
			name:       "Corporate Amex → JioPay",
			cardType:   CardTypeAmex,
			amount:     500000, // ₹5000
			expectedGW: GatewayJioPay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator := createTestOrchestrator(false)

			request := PaymentRequest{
				PaymentMethod: PaymentMethodCard,
				CardDetails: &Card{
					CardType:     tt.cardType,
					CardCategory: CardCategoryCorporate,
				},
				Amount: tt.amount,
			}

			gateway, err := orchestrator.SelectGateway(request)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gateway != tt.expectedGW {
				t.Errorf("expected gateway %s, got %s", tt.expectedGW, gateway)
			}
		})
	}
}

// TestSelectGateway_CreditCard tests credit card routing logic
func TestSelectGateway_CreditCard(t *testing.T) {
	tests := []struct {
		name       string
		cardType   CardType
		amount     int64
		expectedGW GatewayType
	}{
		{
			name:       "Visa Credit → JioPay",
			cardType:   CardTypeVisa,
			amount:     100000, // ₹1000
			expectedGW: GatewayJioPay,
		},
		{
			name:       "Mastercard Credit → JioPay",
			cardType:   CardTypeMastercard,
			amount:     150000, // ₹1500
			expectedGW: GatewayJioPay,
		},
		{
			name:       "RuPay Credit → JioPay",
			cardType:   CardTypeRupay,
			amount:     200000, // ₹2000
			expectedGW: GatewayJioPay,
		},
		{
			name:       "Amex Credit → Easebuzz",
			cardType:   CardTypeAmex,
			amount:     300000, // ₹3000
			expectedGW: GatewayEasebuzz,
		},
		{
			name:       "Diners Credit → Easebuzz",
			cardType:   CardTypeDiners,
			amount:     250000, // ₹2500
			expectedGW: GatewayEasebuzz,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator := createTestOrchestrator(false)

			request := PaymentRequest{
				PaymentMethod: PaymentMethodCard,
				CardDetails: &Card{
					CardType:     tt.cardType,
					CardCategory: CardCategoryCredit,
				},
				Amount: tt.amount,
			}

			gateway, err := orchestrator.SelectGateway(request)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gateway != tt.expectedGW {
				t.Errorf("expected gateway %s, got %s", tt.expectedGW, gateway)
			}
		})
	}
}

// TestSelectGateway_DebitCard tests debit card routing logic with amount thresholds
func TestSelectGateway_DebitCard(t *testing.T) {
	tests := []struct {
		name       string
		cardType   CardType
		amount     int64
		expectedGW GatewayType
	}{
		{
			name:       "Visa Debit < ₹2000 → Easebuzz",
			cardType:   CardTypeVisa,
			amount:     100000, // ₹1000
			expectedGW: GatewayEasebuzz,
		},
		{
			name:       "Visa Debit = ₹2000 → JioPay",
			cardType:   CardTypeVisa,
			amount:     200000, // ₹2000 (exactly at threshold)
			expectedGW: GatewayJioPay,
		},
		{
			name:       "Visa Debit > ₹2000 → JioPay",
			cardType:   CardTypeVisa,
			amount:     250000, // ₹2500
			expectedGW: GatewayJioPay,
		},
		{
			name:       "Mastercard Debit < ₹2000 → Easebuzz",
			cardType:   CardTypeMastercard,
			amount:     150000, // ₹1500
			expectedGW: GatewayEasebuzz,
		},
		{
			name:       "Mastercard Debit >= ₹2000 → JioPay",
			cardType:   CardTypeMastercard,
			amount:     500000, // ₹5000
			expectedGW: GatewayJioPay,
		},
		{
			name:       "RuPay Debit (any amount) → JioPay",
			cardType:   CardTypeRupay,
			amount:     50000, // ₹500 (below threshold but RuPay always JioPay)
			expectedGW: GatewayJioPay,
		},
		{
			name:       "RuPay Debit high amount → JioPay",
			cardType:   CardTypeRupay,
			amount:     500000, // ₹5000
			expectedGW: GatewayJioPay,
		},
		{
			name:       "Amex Debit → Easebuzz",
			cardType:   CardTypeAmex,
			amount:     300000, // ₹3000
			expectedGW: GatewayEasebuzz,
		},
		{
			name:       "Diners Debit → Easebuzz",
			cardType:   CardTypeDiners,
			amount:     200000, // ₹2000
			expectedGW: GatewayEasebuzz,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator := createTestOrchestrator(false)

			request := PaymentRequest{
				PaymentMethod: PaymentMethodCard,
				CardDetails: &Card{
					CardType:     tt.cardType,
					CardCategory: CardCategoryDebit,
				},
				Amount: tt.amount,
			}

			gateway, err := orchestrator.SelectGateway(request)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gateway != tt.expectedGW {
				t.Errorf("expected gateway %s, got %s", tt.expectedGW, gateway)
			}
		})
	}
}

// TestSelectGateway_NonCardPayments tests routing for non-card payment methods
func TestSelectGateway_NonCardPayments(t *testing.T) {
	tests := []struct {
		name          string
		paymentMethod PaymentMethod
		expectedGW    GatewayType
	}{
		{
			name:          "Net Banking → JioPay",
			paymentMethod: PaymentMethodNetBanking,
			expectedGW:    GatewayJioPay,
		},
		{
			name:          "UPI → JioPay",
			paymentMethod: PaymentMethodUPI,
			expectedGW:    GatewayJioPay,
		},
		{
			name:          "Wallet → JioPay",
			paymentMethod: PaymentMethodWallet,
			expectedGW:    GatewayJioPay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator := createTestOrchestrator(false)

			request := PaymentRequest{
				PaymentMethod: tt.paymentMethod,
				Amount:        100000, // ₹1000
			}

			gateway, err := orchestrator.SelectGateway(request)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gateway != tt.expectedGW {
				t.Errorf("expected gateway %s, got %s", tt.expectedGW, gateway)
			}
		})
	}
}

// TestSelectGateway_MockPGEnabled tests that mock PG bypasses all routing logic
func TestSelectGateway_MockPGEnabled(t *testing.T) {
	tests := []struct {
		name          string
		paymentMethod PaymentMethod
		cardType      *CardType
		cardCategory  *CardCategory
	}{
		{
			name:          "Corporate Card with MockPG",
			paymentMethod: PaymentMethodCard,
			cardType:      ptr(CardTypeVisa),
			cardCategory:  ptr(CardCategoryCorporate),
		},
		{
			name:          "Amex Credit with MockPG",
			paymentMethod: PaymentMethodCard,
			cardType:      ptr(CardTypeAmex),
			cardCategory:  ptr(CardCategoryCredit),
		},
		{
			name:          "Net Banking with MockPG",
			paymentMethod: PaymentMethodNetBanking,
		},
		{
			name:          "UPI with MockPG",
			paymentMethod: PaymentMethodUPI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator := createTestOrchestrator(true) // Enable mock PG

			request := PaymentRequest{
				PaymentMethod: tt.paymentMethod,
				Amount:        100000, // ₹1000
			}

			if tt.cardType != nil && tt.cardCategory != nil {
				request.CardDetails = &Card{
					CardType:     *tt.cardType,
					CardCategory: *tt.cardCategory,
				}
			}

			gateway, err := orchestrator.SelectGateway(request)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gateway != GatewayMockPG {
				t.Errorf("expected gateway %s, got %s (MockPG should bypass all routing)", GatewayMockPG, gateway)
			}
		})
	}
}

// TestValidatePaymentRequest tests payment request validation
func TestValidatePaymentRequest(t *testing.T) {
	orchestrator := createTestOrchestrator(false)

	tests := []struct {
		name      string
		request   PaymentRequest
		shouldErr bool
	}{
		{
			name: "Valid card payment",
			request: PaymentRequest{
				BusinessID:    "business-123",
				Amount:        100000,
				Currency:      CurrencyINR,
				PaymentMethod: PaymentMethodCard,
				CardDetails: &Card{
					CardType:     CardTypeVisa,
					CardCategory: CardCategoryCredit,
				},
			},
			shouldErr: false,
		},
		{
			name: "Missing business_id",
			request: PaymentRequest{
				Amount:        100000,
				Currency:      CurrencyINR,
				PaymentMethod: PaymentMethodCard,
			},
			shouldErr: true,
		},
		{
			name: "Invalid amount (zero)",
			request: PaymentRequest{
				BusinessID:    "business-123",
				Amount:        0,
				Currency:      CurrencyINR,
				PaymentMethod: PaymentMethodCard,
			},
			shouldErr: true,
		},
		{
			name: "Invalid amount (negative)",
			request: PaymentRequest{
				BusinessID:    "business-123",
				Amount:        -100,
				Currency:      CurrencyINR,
				PaymentMethod: PaymentMethodCard,
			},
			shouldErr: true,
		},
		{
			name: "Missing currency",
			request: PaymentRequest{
				BusinessID:    "business-123",
				Amount:        100000,
				PaymentMethod: PaymentMethodCard,
			},
			shouldErr: true,
		},
		{
			name: "Missing payment method",
			request: PaymentRequest{
				BusinessID: "business-123",
				Amount:     100000,
				Currency:   CurrencyINR,
			},
			shouldErr: true,
		},
		{
			name: "Card payment without card details",
			request: PaymentRequest{
				BusinessID:    "business-123",
				Amount:        100000,
				Currency:      CurrencyINR,
				PaymentMethod: PaymentMethodCard,
			},
			shouldErr: true,
		},
		{
			name: "Valid net banking payment",
			request: PaymentRequest{
				BusinessID:    "business-123",
				Amount:        100000,
				Currency:      CurrencyINR,
				PaymentMethod: PaymentMethodNetBanking,
				NetBankingDetails: &NetBanking{
					BankCode: "HDFC",
					BankName: "HDFC Bank",
				},
			},
			shouldErr: false,
		},
		{
			name: "Net banking without details",
			request: PaymentRequest{
				BusinessID:    "business-123",
				Amount:        100000,
				Currency:      CurrencyINR,
				PaymentMethod: PaymentMethodNetBanking,
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := orchestrator.ValidatePaymentRequest(tt.request)
			if tt.shouldErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// createTestOrchestrator creates a minimal orchestrator for testing gateway selection logic
func createTestOrchestrator(mockPGEnabled bool) PaymentOrchestrator {
	// Create a logger that discards all output for tests
	testLogger := logger.New(logger.Config{
		Level:  logger.ErrorLevel, // Only log errors and above
		Format: logger.TextFormat,
		Output: io.Discard, // Discard all log output
	})

	return &paymentOrchestratorImpl{
		mockPGEnabled: mockPGEnabled,
		logger:        testLogger,
		// Note: We don't need actual gateway instances for SelectGateway tests
		// as it only returns the gateway type, doesn't call the gateway
	}
}

// ptr is a helper function to get pointer to a value
func ptr[T any](v T) *T {
	return &v
}

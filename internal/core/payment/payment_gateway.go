package payment

import (
	"context"
	"time"
)

// PaymentGateway defines the interface that all payment gateways must implement
type PaymentGateway interface {
	// TokenizeCard tokenizes a card for future payments
	// Different gateways have different approaches:
	// - JioPay: Transaction first → auth reference → tokenize
	// - Easebuzz: Direct tokenize with amount
	TokenizeCard(ctx context.Context, request GatewayTokenizeRequest) (*GatewayTokenizeResponse, error)

	// InitiatePayment initiates a payment using tokenized card or direct payment details
	// Uses token + cryptogram for tokenized payments
	InitiatePayment(ctx context.Context, request GatewayPaymentRequest) (*GatewayPaymentResponse, error)

	// GenerateCryptogram generates PG-specific cryptogram for secure payments
	// Each gateway has its own cryptogram format and validity period
	GenerateCryptogram(ctx context.Context, request CryptogramRequest) (*CryptogramResponse, error)

	// GetGatewayType returns the gateway type identifier
	GetGatewayType() GatewayType

	// ValidateWebhook validates incoming webhook signatures
	ValidateWebhook(payload []byte, signature string) error

	// IsAggregatorModel returns the payment model for this gateway
	// false = Normal model: merchant passes total amount (base + convenience fee) to PG
	// true = Aggregator model: merchant passes base amount, PG charges convenience fee from customer
	IsAggregatorModel() bool
}

// === GATEWAY REQUEST/RESPONSE STRUCTURES ===

// GatewayTokenizeRequest represents a generic tokenization request
type GatewayTokenizeRequest struct {
	BusinessID    string         `json:"business_id"`
	TransactionID string         `json:"transaction_id"`
	CardDetails   Card           `json:"card_details"`
	Amount        int64          `json:"amount,omitempty"` // Required for Easebuzz
	Currency      Currency       `json:"currency,omitempty"`
	AuthReference string         `json:"auth_reference,omitempty"` // Required for JioPay
	CallbackURL   string         `json:"callback_url"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// GatewayTokenizeResponse represents a generic tokenization response
type GatewayTokenizeResponse struct {
	Token         string         `json:"token"`
	CardID        string         `json:"card_id"`
	TransactionID string         `json:"transaction_id"`
	Status        string         `json:"status"`
	Message       string         `json:"message"`
	RedirectURL   string         `json:"redirect_url,omitempty"` // For additional verification
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// GatewayPaymentRequest represents a generic payment initiation request
type GatewayPaymentRequest struct {
	BusinessID    string        `json:"business_id"`
	TransactionID string        `json:"transaction_id"`
	Amount        int64         `json:"amount"`
	Currency      Currency      `json:"currency"`
	PaymentMethod PaymentMethod `json:"payment_method"`
	PaymentType   PaymentType   `json:"payment_type"` // Type: payment, tokenization, refund
	Description   string        `json:"description"`
	CallbackURL   string        `json:"callback_url"`

	// Card payment specific
	Token       string `json:"token,omitempty"`        // For tokenized payments
	Cryptogram  string `json:"cryptogram,omitempty"`   // Generated cryptogram
	CardDetails *Card  `json:"card_details,omitempty"` // For direct card payments

	// Other payment methods
	NetBankingDetails *NetBanking `json:"net_banking,omitempty"`
	UPIDetails        *UPI        `json:"upi_details,omitempty"`
	WalletDetails     *Wallet     `json:"wallet_details,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

// GatewayPaymentResponse represents a generic payment initiation response
type GatewayPaymentResponse struct {
	PaymentID        string         `json:"payment_id"`         // Our internal payment ID
	GatewayPaymentID string         `json:"gateway_payment_id"` // Gateway's payment ID
	TransactionID    string         `json:"transaction_id"`     // Transaction tracking ID
	Status           PaymentStatus  `json:"status"`
	Message          string         `json:"message"`
	RedirectURL      string         `json:"redirect_url"` // Where user goes for auth/OTP
	Amount           int64          `json:"amount"`
	Currency         Currency       `json:"currency"`
	CreatedAt        time.Time      `json:"created_at"`
	ExpiresAt        *time.Time     `json:"expires_at,omitempty"` // Redirect URL expiry
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// CryptogramRequest represents a request to generate cryptogram
type CryptogramRequest struct {
	Token         string         `json:"token"`
	Amount        int64          `json:"amount"`
	Currency      Currency       `json:"currency"`
	TransactionID string         `json:"transaction_id"`
	BusinessID    string         `json:"business_id"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// CryptogramResponse represents the generated cryptogram response
type CryptogramResponse struct {
	Cryptogram   string         `json:"cryptogram"`
	ExpiresAt    time.Time      `json:"expires_at"`
	ValidityMins int            `json:"validity_minutes"`
	Algorithm    string         `json:"algorithm,omitempty"` // Algorithm used
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// === WEBHOOK STRUCTURES ===

// WebhookPayload represents a generic webhook payload from payment gateways
type WebhookPayload struct {
	GatewayType      GatewayType    `json:"gateway_type"`
	PaymentID        string         `json:"payment_id"`
	GatewayPaymentID string         `json:"gateway_payment_id"`
	TransactionID    string         `json:"transaction_id"`
	Status           PaymentStatus  `json:"status"`
	Amount           int64          `json:"amount"`
	Currency         Currency       `json:"currency"`
	PaymentMethod    PaymentMethod  `json:"payment_method"`
	ProcessedAt      time.Time      `json:"processed_at"`
	FailureReason    string         `json:"failure_reason,omitempty"`
	BankReference    string         `json:"bank_reference,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

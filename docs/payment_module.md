# Payment Module Implementation

This document describes the implementation of the payment module following the clean architecture pattern established in the project. The payment system is designed with a sophisticated orchestrator pattern that intelligently routes payments to appropriate gateways based on payment method and card details.

## Overview

The payment module implements a complete payment processing system with multi-gateway support, card tokenization, bank account verification, and wallet management. It follows clean architecture principles with clear separation between business logic, orchestration, and external integrations.

### Architecture Components

#### 1. Domain Layer (`internal/core/payment/`)
- **payment_entity.go**: Payment domain entities (Payment, Card, BankAccount, UPI, StoredCard)
- **payment_orchestrator.go**: Payment orchestrator interface for multi-gateway routing
- **payment_gateway.go**: Payment gateway interface abstraction

#### 2. Business Rules Layer (`internal/core/business_rules/`)
- **fee_calculator.go**: Fee calculation service interface and implementation
- **wallet_service.go**: Wallet management service with payment source storage
- **bank_verification_service.go**: Bank account penny drop verification interface
- **mock_bank_verification.go**: Mock implementation with realistic API simulation

#### 3. Infrastructure Layer (`internal/adapters/external/payment/`)
- **jiopay/**: JioPay gateway implementation (Net Banking, Master/Visa/RuPay/Corporate Credit Cards)
- **easebuzz/**: Easebuzz gateway implementation (Amex, Diners, Master/Visa Debit Cards)
- **factory/**: Gateway factory for dynamic gateway instantiation

#### 4. Orchestration Layer (`internal/services/payments/`)
- **payment_orchestrator.go**: Multi-gateway routing orchestrator with intelligent card-based routing
- Automatically selects optimal gateway based on payment method and card type
- Handles gateway failover and retry logic

#### 5. Presentation Layer (`internal/handlers/http/payment/`)
- **payment_handler.go**: HTTP handlers for payment operations with REST API endpoints
- **payment_models.go**: Request/response models for API layer
- **webhook_handler.go**: Gateway webhook handling for payment status updates

## Payment Gateway Routing

### Intelligent Gateway Selection

The payment orchestrator automatically routes payments to the appropriate gateway based on payment method and card details:

#### JioPay Gateway Routes
- Net Banking (all banks)
- Master/Visa Credit Cards
- RuPay Credit Cards
- Corporate Credit Cards (all types)

#### Easebuzz Gateway Routes
- Amex Cards (all categories)
- Diners Cards (all categories)
- Master/Visa Debit Cards

### Routing Logic
```go
// Automatic gateway selection based on card type
if paymentMethod == "netbanking" {
    gateway = JioPayGateway
} else if cardType == "Amex" || cardType == "Diners" {
    gateway = EasebuzzGateway
} else if cardCategory == "credit" {
    gateway = JioPayGateway  // Master/Visa/RuPay credit
} else {
    gateway = EasebuzzGateway  // Master/Visa debit
}
```

## API Endpoints

### Core Payment Operations
- `POST /api/v1/process_payment/fetch_wallet` - Get business wallet with all payment sources
- `POST /api/v1/process_payment/get_fee_rules` - Get fee calculation rules for payment method
- `POST /api/v1/process_payment/make_payment` - Process payment with automatic gateway routing
- `POST /api/v1/process_payment/tokenize` - Tokenize card for PCI DSS compliant storage
- `POST /api/v1/process_payment/add_payment_source` - Add payment sources (bank accounts, UPI)

### Webhook Endpoints
- `POST /api/v1/process_payment/webhooks/jiopay` - JioPay webhook for payment status updates
- `POST /api/v1/process_payment/webhooks/easebuzz` - Easebuzz webhook for payment status updates

## Key Features

### 1. Multi-Gateway Orchestration
Intelligent payment routing across multiple gateways with automatic failover:
- Route selection based on payment method and card type
- Dynamic gateway instantiation using factory pattern
- Consistent interface across different gateways
- Retry logic with exponential backoff

### 2. PCI DSS Compliant Card Tokenization
Secure card storage following PCI DSS standards:
- Two-step tokenization process with gateway validation
- Cards never stored in plain text
- Token-based payment processing
- Gateway-specific token management

### 3. Bank Account Penny Drop Verification
Comprehensive bank account verification with penny drop simulation:
- Interface-based verification service design
- Mock implementation with realistic 2-second API simulation
- Pattern-based testing (account numbers ending in 0000, 1111, 2222, 9999)
- Verification metadata storage (status, date, reference, amount)
- Duplicate account detection per business
- Account number masking for security

### 4. Wallet Management
Business wallet system for storing payment sources:
- Store tokenized cards with metadata
- Store verified bank accounts
- Store UPI details (future implementation)
- Credit line management (future implementation)
- Unified wallet retrieval API

### 5. Fee Calculation
Flexible fee calculation system:
- Method-specific fee rules (percentage + fixed fee)
- Min/max fee enforcement
- Card type and category-based differentiation
- Multiple currency support

### 6. Webhook Processing
Asynchronous payment status updates:
- Signature verification for security
- Status update handling
- Retry mechanism for failed webhooks
- Gateway-specific webhook handling

### 7. Transaction Management
Proper transaction handling throughout the payment flow:
- Context propagation for request tracing
- Timeout management (60s for tokenization, 30s for payments)
- Proper error handling with categorization
- Audit trail for all operations

## Payment Methods Supported

### 1. Card Payments
- **Direct Card**: One-time payment with card details
- **Tokenized Card**: Payment using stored card token
- **Card Types**: Visa, MasterCard, RuPay, Amex, Diners
- **Card Categories**: Credit, Debit, Corporate

### 2. Net Banking
- Multiple bank support
- Bank code-based routing
- Redirect-based authentication

### 3. UPI
- VPA-based payments
- Real-time payment processing
- Multiple UPI apps support

### 4. Wallet Payments
- Digital wallet integration
- Multiple wallet provider support

## Request/Response Examples

### 1. Fetch Wallet
Get all payment sources for a business:
```json
POST /api/v1/process_payment/fetch_wallet
{
    "business_id": "business-uuid-123"
}

Response:
{
    "message": "Wallet fetched successfully",
    "status": "success",
    "data": {
        "business_id": "business-uuid-123",
        "stored_cards": [
            {
                "card_id": "card-uuid",
                "last_four_digits": "4242",
                "card_type": "credit",
                "card_category": "visa",
                "expiry_month": "12",
                "expiry_year": "2025",
                "issuer_bank": "HDFC Bank",
                "is_active": true
            }
        ],
        "bank_accounts": [
            {
                "account_id": "account-uuid",
                "last_five_digits": "23456",
                "bank_name": "HDFC Bank",
                "ifsc_code": "HDFC0000123",
                "account_type": "savings",
                "is_active": true,
                "is_verified": true
            }
        ],
        "upi_details": [],
        "credit_lines": [],
        "last_updated": "2025-09-30T10:30:00Z"
    }
}
```

### 2. Tokenize Card (PCI DSS Compliant)
Store card securely for future payments:
```json
POST /api/v1/process_payment/tokenize
{
    "user_id": "business-uuid-123",
    "card_details": {
        "card_number": "4111111111111111",
        "expiry_month": "12",
        "expiry_year": "2025",
        "cvv": "123",
        "holder_name": "John Doe",
        "card_type": "credit",
        "card_category": "visa"
    }
}

Response:
{
    "message": "Card tokenized successfully",
    "status": "success",
    "data": {
        "token": "gateway-token-xyz",
        "card_id": "card-uuid",
        "last_four_digits": "1111",
        "card_type": "credit",
        "card_category": "visa",
        "expiry_month": "12",
        "expiry_year": "2025",
        "status": "active",
        "message": "Card tokenized and stored successfully"
    }
}
```

### 3. Make Payment (Automatic Gateway Routing)
Process payment with intelligent gateway selection:
```json
POST /api/v1/process_payment/make_payment
{
    "amount": 50000,
    "currency": "INR",
    "payment_method": "card",
    "description": "Product purchase",
    "card_details": {
        "card_number": "5555555555554444",
        "expiry_month": "12",
        "expiry_year": "2025",
        "cvv": "123",
        "holder_name": "Jane Smith",
        "card_type": "credit",
        "card_category": "mastercard"
    },
    "callback_url": "https://example.com/payment/callback",
    "metadata": {
        "order_id": "order-123",
        "product_id": "product-456"
    }
}

Response:
{
    "message": "Payment initiated successfully",
    "status": "success",
    "data": {
        "payment_id": "payment-uuid",
        "status": "pending",
        "amount": 50000,
        "currency": "INR",
        "payment_method": "card",
        "gateway_used": "jiopay",
        "gateway_payment_id": "jiopay-txn-123",
        "redirect_url": "https://gateway.jiopay.com/pay/123",
        "message": "Redirect user to complete payment",
        "created_at": "2025-09-30T10:30:00Z",
        "metadata": {
            "order_id": "order-123",
            "product_id": "product-456"
        }
    }
}
```

### 4. Add Bank Account (Penny Drop Verification)
Add and verify bank account with penny drop:
```json
POST /api/v1/process_payment/add_payment_source
{
    "business_id": "business-uuid-123",
    "source_type": "bank_account",
    "bank_account_details": {
        "account_number": "1234567890123456",
        "ifsc_code": "HDFC0000123",
        "account_holder_name": "John Doe",
        "bank_name": "HDFC Bank",
        "branch_name": "Mumbai Main Branch",
        "account_type": "savings"
    }
}

Response:
{
    "message": "Payment source added successfully",
    "status": "success",
    "data": {
        "success": true,
        "message": "Bank account verified and added successfully",
        "source_id": "account-uuid",
        "source_type": "bank_account",
        "verification_status": "verified",
        "account_details": {
            "masked_account_number": "****3456",
            "verified_name": "JOHN DOE",
            "bank_name": "HDFC Bank",
            "branch_name": "Mumbai Main Branch",
            "ifsc_code": "HDFC0000123",
            "account_type": "savings",
            "is_active": true
        }
    }
}
```

### 5. Payment with Tokenized Card
Use stored card for payment:
```json
POST /api/v1/process_payment/make_payment
{
    "amount": 25000,
    "currency": "INR",
    "payment_method": "token-card",
    "description": "Subscription renewal",
    "token_details": {
        "card_id": "card-uuid-from-tokenize"
    },
    "callback_url": "https://example.com/payment/callback"
}
```

### 6. Net Banking Payment
Process net banking payment:
```json
POST /api/v1/process_payment/make_payment
{
    "amount": 100000,
    "currency": "INR",
    "payment_method": "netbanking",
    "description": "Invoice payment",
    "net_banking": {
        "bank_code": "HDFC",
        "bank_name": "HDFC Bank"
    },
    "callback_url": "https://example.com/payment/callback"
}
```

### 7. Get Fee Rules
Calculate fees for a payment method:
```json
POST /api/v1/process_payment/get_fee_rules
{
    "payment_method": "card",
    "card_type": "credit",
    "card_category": "visa",
    "amount": 50000
}

Response:
{
    "message": "Fee rules fetched successfully",
    "status": "success",
    "data": {
        "payment_method": "card",
        "fee_percentage": 2.0,
        "fixed_fee": 0,
        "min_fee": 0,
        "max_fee": 5000,
        "currency": "INR"
    }
}
```

## Bank Verification Testing

### Pattern-Based Testing
The mock bank verification service uses account number patterns for testing:

#### Success Scenario
```json
{
    "account_number": "1234567890123456",  // Any number not matching failure patterns
    "ifsc_code": "HDFC0000123",
    "account_holder_name": "John Doe"
}
// Result: Verified successfully
```

#### Account Closed (ends in 0000)
```json
{
    "account_number": "1234567890120000",
    "ifsc_code": "ICIC0000456",
    "account_holder_name": "Jane Smith"
}
// Result: ACCOUNT_CLOSED error
```

#### Invalid Account (ends in 1111)
```json
{
    "account_number": "9876543210981111",
    "ifsc_code": "SBIN0001234",
    "account_holder_name": "Bob Wilson"
}
// Result: INVALID_ACCOUNT error
```

#### Name Mismatch (ends in 2222)
```json
{
    "account_number": "5555444433332222",
    "ifsc_code": "AXIS0000789",
    "account_holder_name": "Wrong Name"
}
// Result: NAME_MISMATCH error
```

#### API Timeout (ends in 9999)
```json
{
    "account_number": "1111222233339999",
    "ifsc_code": "PUNB0001122",
    "account_holder_name": "Timeout Test"
}
// Result: API_TIMEOUT error
```

## Error Handling

### Payment Errors
- **INVALID_CARD**: Card number validation failed
- **EXPIRED_CARD**: Card has expired
- **INSUFFICIENT_FUNDS**: Transaction declined by bank
- **GATEWAY_ERROR**: Payment gateway returned error
- **DUPLICATE_TRANSACTION**: Transaction already processed

### Bank Verification Errors
- **ACCOUNT_CLOSED**: Bank account is closed
- **INVALID_ACCOUNT**: Account number not found
- **NAME_MISMATCH**: Account holder name doesn't match
- **INVALID_IFSC**: IFSC code validation failed
- **API_TIMEOUT**: Verification service timeout
- **VERIFICATION_FAILED**: Generic verification failure

### Validation Errors
- **validation_failed**: Request validation failed
- **invalid_request**: Malformed request body
- **unsupported_source_type**: Payment source type not supported
- **unsupported_payment_method**: Payment method not supported

## Environment Configuration

### Required Environment Variables

#### JioPay Configuration
```bash
JIOPAY_BASE_URL=https://api.jiopay.com
JIOPAY_MERCHANT_ID=your_merchant_id
JIOPAY_API_KEY=your_api_key
JIOPAY_SECRET_KEY=your_secret_key
JIOPAY_ENVIRONMENT=sandbox  # or production
```

#### Easebuzz Configuration
```bash
EASEBUZZ_BASE_URL=https://testpay.easebuzz.in
EASEBUZZ_MERCHANT_KEY=your_merchant_key
EASEBUZZ_SALT=your_salt_key
EASEBUZZ_ENVIRONMENT=test  # or prod
```

## Database Schema

### Payment Tables
- `payments` - Main payment records with gateway routing info
- `payment_attempts` - Retry attempt tracking for debugging
- `tokenized_cards` - Secure card token storage for future payments

### Wallet Tables
- `business_wallets` - Business wallet metadata
- `wallet_payment_sources` - Stored payment sources
- `bank_verification_logs` - Penny drop verification audit trail

## Integration Guide

### 1. Initialize Payment System
```go
// In your main application setup
import (
    "xcentro.in/internal/core/payment"
    "xcentro.in/internal/core/business_rules"
    "xcentro.in/internal/services/payments"
    paymentHandler "xcentro.in/internal/handlers/http/payment"
)

// Create gateway factory
gatewayFactory := factory.NewGatewayFactory(cfg)

// Create payment orchestrator
orchestrator := payments.NewPaymentOrchestrator(gatewayFactory)

// Create fee calculator
feeCalculator := business_rules.NewMockFeeCalculator()

// Create wallet service
walletService := business_rules.NewMockWalletService()

// Register payment routes
paymentHandler.RegisterPaymentRoutes(apiGroup, orchestrator, feeCalculator, walletService)
```

### 2. Adding New Payment Gateway
To add a new payment gateway:

1. **Create Gateway Client**
```go
// internal/adapters/external/payment/newgateway/client.go
type NewGatewayClient struct {
    baseURL string
    apiKey  string
}

func (c *NewGatewayClient) ProcessPayment(ctx context.Context, req payment.PaymentRequest) (*payment.PaymentResponse, error) {
    // Implement gateway-specific logic
}
```

2. **Implement PaymentGateway Interface**
```go
func (c *NewGatewayClient) GetGatewayType() payment.GatewayType {
    return payment.GatewayNewGateway
}

func (c *NewGatewayClient) TokenizeCard(ctx context.Context, req payment.TokenizeRequest) (*payment.TokenizeResponse, error) {
    // Implement tokenization
}
```

3. **Update Gateway Factory**
```go
// internal/adapters/external/payment/factory/gateway_factory.go
func (f *GatewayFactory) CreateGateway(gatewayType payment.GatewayType) (payment.PaymentGateway, error) {
    switch gatewayType {
    case payment.GatewayNewGateway:
        return newgateway.NewClient(f.config.NewGateway), nil
    // ... other cases
    }
}
```

4. **Update Routing Rules**
```go
// internal/services/payments/payment_orchestrator.go
func (o *PaymentOrchestrator) selectGateway(req payment.PaymentRequest) payment.GatewayType {
    // Add routing logic for new gateway
}
```

### 3. Webhook Integration
Configure webhooks in gateway dashboard pointing to:
- JioPay: `https://your-domain.com/api/v1/process_payment/webhooks/jiopay`
- Easebuzz: `https://your-domain.com/api/v1/process_payment/webhooks/easebuzz`

## Security Considerations

### 1. PCI DSS Compliance
- Cards never stored in plain text
- Card details only in tokenization flow
- Tokenization uses gateway-provided tokens
- CVV never stored (transient only)

### 2. Bank Account Security
- Account numbers masked (show last 4 digits only)
- Verification metadata encrypted at rest
- Penny drop amounts randomized
- Audit trail for all verification attempts

### 3. Webhook Security
- Signature verification for all webhooks
- IP whitelist for gateway IPs
- Replay attack prevention
- Rate limiting on webhook endpoints

### 4. API Security
- JWT authentication required
- Business ID validation
- Request rate limiting
- Input sanitization and validation

## Performance Considerations

### 1. Gateway Failover
- Primary gateway selection based on routing rules
- Automatic failover to secondary gateway on failure
- Circuit breaker pattern for gateway health

### 2. Caching Strategy
- Fee rules cached for 1 hour
- Gateway configuration cached at startup
- Wallet data cached for 5 minutes

### 3. Async Processing
- Webhook processing via background workers
- Payment status polling for long-running transactions
- Email notifications via queue

## Monitoring and Logging

### Key Metrics to Track
- Payment success rate by gateway
- Average payment processing time
- Gateway failure rate
- Tokenization success rate
- Bank verification success rate
- Webhook delivery success rate

### Logging Best Practices
- Log all payment attempts with unique request IDs
- Never log card numbers or CVV
- Log masked account numbers only
- Track gateway response times
- Monitor error patterns

## Troubleshooting

### Common Issues

#### Payment Stuck in Pending
1. Check webhook delivery from gateway
2. Verify webhook signature validation
3. Check payment status via gateway API
4. Review gateway logs

#### Tokenization Failures
1. Verify card number validity
2. Check expiry date format
3. Confirm gateway credentials
4. Review gateway response errors

#### Bank Verification Failures
1. Verify IFSC code format (11 characters)
2. Check account number length (9-18 digits)
3. Validate account holder name
4. Review verification service logs

## Testing

### Integration Tests
```bash
# Run all payment integration tests
go test ./internal/services/payments/... -v

# Run specific test suite
go test ./internal/adapters/external/payment/jiopay/... -v
```

### Mock Testing
All payment services have mock implementations for testing:
- `MockPaymentOrchestrator`
- `MockFeeCalculator`
- `MockWalletService`
- `MockBankVerificationService`

## Future Enhancements

### Planned Features
- [ ] UPI account verification implementation
- [ ] Credit line integration
- [ ] Recurring payment support
- [ ] Multi-currency conversion
- [ ] Payment analytics dashboard
- [ ] Fraud detection system
- [ ] Smart routing based on success rates
- [ ] Gateway load balancing
- [ ] Payment link generation
- [ ] QR code payment support

## Notes

- All payment amounts are in smallest currency unit (paise for INR)
- Gateway selection is deterministic based on card details
- Webhook retry logic: 3 attempts with exponential backoff
- Payment timeout: 30 seconds for standard, 60 seconds for tokenization
- Bank verification: 2-second simulated delay (production will vary)
- All timestamps in ISO 8601 format (UTC)
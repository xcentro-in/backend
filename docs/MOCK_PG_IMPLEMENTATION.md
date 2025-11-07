# Mock Payment Gateway Implementation

## Overview

A complete mock payment gateway implementation for end-to-end testing without making real API calls or incurring charges. The mock PG implements the full `PaymentGateway` interface and can bypass normal gateway routing when enabled via environment variable.

## Implementation Details

### 1. Mock PG Client (`internal/adapters/external/payment/mockpg/client.go`)

**Features:**
- Full `PaymentGateway` interface implementation
- Supports all payment methods: card (tokenized & direct), net banking, UPI, wallet
- Simulates realistic payment flows including:
  - Card tokenization
  - Cryptogram generation
  - Payment initiation with redirect URLs
  - Webhook validation
- Configurable success/failure simulation for error handling testing
- Artificial network latency simulation (100ms default)
- Detailed console logging for debugging

**Key Methods:**
- `TokenizeCard()` - Simulates card tokenization without external API calls
- `GenerateCryptogram()` - Creates mock cryptograms for secure payments
- `InitiatePayment()` - Initiates mock payments and returns redirect URLs
- `ValidateWebhook()` - Validates webhook signatures in mock mode

### 2. Environment Configuration

**New Environment Variables (`.env.example`):**
```bash
# Enable mock PG to bypass real payment gateways for testing
# When enabled, all payment requests use mock gateway regardless of routing rules
MOCKPG_ENABLED=false

# Mock merchant credentials (for testing purposes)
MOCKPG_MERCHANT_ID=MOCK_MERCHANT_123
MOCKPG_API_KEY=mock_api_key_for_testing_only

# Simulate success/failure scenarios
# true = All operations succeed (happy path testing)
# false = Randomly simulate failures (error handling testing)
MOCKPG_SIMULATE_SUCCESS=true
```

**Configuration Structure (`internal/shared/config/config.go`):**
```go
type MockPGConfig struct {
    Enabled         bool   // Enable mock PG (bypass real gateways)
    MerchantID      string // Mock merchant identifier
    APIKey          string // Mock API key (sensitive)
    SimulateSuccess bool   // If true, always succeed; if false, simulate random failures
}
```

### 3. Gateway Type Extension

**Updated `payment_types.go`:**
```go
const (
    GatewayJioPay   GatewayType = "jiopay"
    GatewayEasebuzz GatewayType = "easebuzz"
    GatewayMockPG   GatewayType = "mockpg" // Mock gateway for testing
)
```

### 4. Payment Orchestrator Integration

**Bypass Logic (`internal/core/payment/payment_orchestrator.go`):**

The orchestrator now includes:
- `mockPGGateway` field for mock PG client instance
- `mockPGEnabled` flag to control bypass behavior

**Gateway Selection Bypass:**
```go
func (o *MockPaymentOrchestrator) SelectGateway(request PaymentRequest) (GatewayType, error) {
    // Check if mock PG is enabled - bypass normal gateway selection
    if o.mockPGEnabled {
        o.logger.InfoWith("Mock PG enabled: bypassing normal gateway selection")
        return GatewayMockPG, nil
    }

    // Normal gateway selection logic...
}
```

**Tokenized Payment Bypass:**
```go
// If mock PG is enabled, bypass the normal gateway and use mock PG instead
var selectedGatewayType GatewayType
if o.mockPGEnabled {
    selectedGatewayType = GatewayMockPG
    o.logger.InfoWith("Mock PG enabled: bypassing tokenized card gateway routing",
        "original_gateway", storedCard.GatewayUsed,
        "override_gateway", GatewayMockPG)
} else {
    selectedGatewayType = storedCard.GatewayUsed
}
```

### 5. Application Wiring (`cmd/api/routes.go`)

**Gateway Initialization:**
```go
easebuzzClient := easebuzz.NewEasebuzzClient(...)
jioPayClient := jiopay.NewJioPayClient(...)
mockPGClient := mockpg.NewMockPGClient(
    cfg.MockPG.MerchantID,
    cfg.MockPG.APIKey,
    cfg.MockPG.SimulateSuccess,
)

orchestrator := corePayment.NewMockPaymentOrchestrator(
    paymentRepository,
    storedCardRepository,
    jioPayClient,
    easebuzzClient,
    mockPGClient,        // Mock PG client
    cfg.MockPG.Enabled,  // Enable flag
    logger.GetDefault(),
)
```

## Usage

### Enable Mock PG for Testing

**In `.env` file:**
```bash
MOCKPG_ENABLED=true
MOCKPG_SIMULATE_SUCCESS=true
```

### Test Scenarios

**1. Happy Path Testing (`MOCKPG_SIMULATE_SUCCESS=true`):**
- All tokenization operations succeed
- All payment initiations succeed
- All cryptogram generations succeed
- Webhook validations pass
- Perfect for end-to-end flow testing

**2. Error Handling Testing (`MOCKPG_SIMULATE_SUCCESS=false`):**
- Random failures simulated (10-15% failure rate)
- Tests error handling and retry logic
- Validates transaction rollback behavior

### Behavior When Enabled

**Direct Payments:**
1. Payment request comes in with card/UPI/netbanking details
2. Normal gateway selection is **bypassed**
3. Mock PG processes the payment
4. Returns mock redirect URL and gateway payment ID
5. All data flows through normal service/repository layers

**Tokenized Card Payments:**
1. Card is already tokenized with a real gateway (e.g., JioPay)
2. Payment request uses stored card by card_id
3. Normal gateway lookup (from stored card) is **bypassed**
4. Mock PG generates cryptogram and processes payment
5. Original gateway info is preserved in metadata for reference

### Key Advantages

1. **No External API Calls**: Complete offline testing capability
2. **No Costs**: No payment gateway charges during development
3. **Consistent Results**: Predictable behavior for CI/CD pipelines
4. **Fast Testing**: No network latency from real gateways
5. **Full E2E Coverage**: Tests entire payment flow including:
   - Payment creation
   - Fee calculation
   - Voucher finalization
   - Database transactions
   - State management

### Testing Different Payment Methods

**Card Payments (Direct):**
```bash
POST /v1/process_payment
{
  "business_id": "...",
  "amount": 50000,
  "payment_method": "card",
  "card_details": {...}
}
```

**Card Payments (Tokenized):**
```bash
POST /v1/process_payment
{
  "business_id": "...",
  "amount": 50000,
  "payment_method": "token_card",
  "token_card_details": {
    "card_id": "existing_card_id"
  }
}
```

**Net Banking:**
```bash
POST /v1/process_payment
{
  "business_id": "...",
  "amount": 50000,
  "payment_method": "net_banking",
  "net_banking_details": {...}
}
```

All above requests will route to Mock PG when `MOCKPG_ENABLED=true`.

## Implementation Notes

### Bypass Mechanism

The bypass happens at two levels:

1. **Gateway Selection**: When `mockPGEnabled=true`, `SelectGateway()` always returns `GatewayMockPG`
2. **Gateway Instance**: `getGatewayInstance()` returns mock PG client when type is `GatewayMockPG`

This ensures:
- No changes needed to service layer logic
- No changes needed to handler logic
- No changes needed to repository logic
- Complete transparency to higher layers

### Metadata Tracking

When mock PG is used, payment metadata includes:
```go
{
  "gateway_used": "mockpg",
  "mockpg_override": true,
  "original_gateway": "jiopay",  // For tokenized payments
  "mock_simulation": true
}
```

This allows:
- Tracking which payments used mock PG
- Debugging and auditing
- Distinguishing test vs real payments

### Security Considerations

1. **Production Safety**:
   - Default: `MOCKPG_ENABLED=false`
   - Requires explicit opt-in
   - Logged at INFO level when active

2. **Mock Data**:
   - Clearly marked in all responses
   - Mock IDs use `MOCK_` prefix
   - Easy to identify in logs and database

3. **Configuration Redaction**:
   - Mock API key is redacted in logs
   - Follows same pattern as real gateway credentials

## Future Enhancements

Potential additions:
1. Configurable failure scenarios (timeout, invalid card, insufficient funds)
2. Webhook simulation endpoints for callback testing
3. Settlement simulation for payout flow testing
4. Configurable delay simulation for performance testing
5. Mock PG dashboard for monitoring test payments

## Related Files

- `/internal/adapters/external/payment/mockpg/client.go` - Mock PG implementation
- `/internal/core/payment/payment_types.go` - Gateway type enum
- `/internal/core/payment/payment_orchestrator.go` - Bypass logic
- `/internal/shared/config/config.go` - Configuration structure
- `/cmd/api/routes.go` - Application wiring
- `/.env.example` - Environment variable documentation
- `/docs/payment_module.md` - Payment system architecture
- `/GATEWAY_INTEGRATION.md` - Gateway integration guide

## Testing Checklist

- [ ] Enable mock PG in `.env`
- [ ] Test direct card payment flow
- [ ] Test tokenized card payment flow
- [ ] Test net banking payment flow
- [ ] Test UPI payment flow
- [ ] Test wallet payment flow
- [ ] Test card tokenization flow
- [ ] Test cryptogram generation
- [ ] Test with `SIMULATE_SUCCESS=true` (happy path)
- [ ] Test with `SIMULATE_SUCCESS=false` (error scenarios)
- [ ] Verify no external API calls made
- [ ] Verify database state is correct
- [ ] Verify fees calculated correctly
- [ ] Verify vouchers finalized correctly
- [ ] Check logs for bypass indicators
- [ ] Disable mock PG and verify normal routing works

## Support

For questions or issues with the mock PG implementation, refer to:
- Payment module documentation: `/docs/payment_module.md`
- Gateway integration guide: `/GATEWAY_INTEGRATION.md`
- Example code: `/EXAMPLES.md`

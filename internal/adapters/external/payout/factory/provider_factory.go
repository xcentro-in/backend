package factory

import (
	"fmt"

	"xcentro.in/internal/adapters/external/payout/castler"
	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// PayoutProviderFactory creates payout provider instances based on provider type
type PayoutProviderFactory struct {
	config *config.Config
	// Cache provider instances (singleton pattern)
	castlerClient     *castler.CastlerClient
	mockCastlerClient *castler.MockCastlerClient
	// Add other providers here in future (e.g., razorpayClient)
}

// NewPayoutProviderFactory creates a new factory with configuration
func NewPayoutProviderFactory(cfg *config.Config) *PayoutProviderFactory {
	return &PayoutProviderFactory{
		config: cfg,
	}
}

// GetProvider returns a payout provider instance based on provider type
func (f *PayoutProviderFactory) GetProvider(providerType payout.ProviderType) (payout.PayoutProvider, error) {
	switch providerType {
	case payout.ProviderCastler:
		return f.getCastlerClient(), nil

	// Future providers:
	// case payout.ProviderRazorpay:
	// 	return f.getRazorpayClient(), nil

	default:
		return nil, fmt.Errorf("unsupported payout provider: %s", providerType)
	}
}

// getCastlerClient returns cached Castler client or creates new one
// If CASTLER_MOCK_ENABLED=true, returns mock client instead
func (f *PayoutProviderFactory) getCastlerClient() payout.PayoutProvider {
	// Check if mock mode is enabled
	logger.GetDefault().InfoWith("🔍 Castler Provider Factory Check",
		"mock_enabled", f.config.Castler.MockEnabled,
		"base_url", f.config.Castler.BaseURL)

	if f.config.Castler.MockEnabled {
		logger.GetDefault().Info("🎭 Castler Mock Mode ENABLED - Using mock client")
		if f.mockCastlerClient == nil {
			f.mockCastlerClient = castler.NewMockCastlerClient()
		}
		return f.mockCastlerClient
	}

	logger.GetDefault().Warn("⚠️  Castler Mock Mode DISABLED - Using REAL Castler API client")

	// Use real Castler client
	if f.castlerClient == nil {
		// Get Castler configuration from app config
		baseURL := f.config.Castler.BaseURL
		apiKey := f.config.Castler.APIKey
		secretKey := f.config.Castler.SecretKey
		escrowID := f.config.Castler.EscrowID
		encryptionKey := f.config.Castler.EncryptionKey
		encryptionIV := f.config.Castler.EncryptionIV

		client, err := castler.NewCastlerClient(baseURL, apiKey, secretKey, escrowID, encryptionKey, encryptionIV)
		if err != nil {
			panic("Failed to create Castler client: " + err.Error())
		}
		f.castlerClient = client
	}
	return f.castlerClient
}

// GetAllProviders returns all available providers (useful for multi-provider fallback)
func (f *PayoutProviderFactory) GetAllProviders() []payout.PayoutProvider {
	providers := []payout.PayoutProvider{
		f.getCastlerClient(),
		// Add other providers here
	}
	return providers
}

// GetPreferredProvider returns the preferred provider based on business rules
// Priority: Castler > Razorpay (add more logic as needed)
func (f *PayoutProviderFactory) GetPreferredProvider() (payout.PayoutProvider, error) {
	// For MVP, always use Castler as preferred provider
	return f.getCastlerClient(), nil
}

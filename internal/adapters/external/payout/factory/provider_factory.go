package factory

import (
	"fmt"

	"xcentro.in/internal/adapters/external/payout/castler"
	"xcentro.in/internal/core/payout"
)

// PayoutProviderConfig holds configuration for payout providers
type PayoutProviderConfig struct {
	// Castler configuration
	CastlerBaseURL   string
	CastlerAPIKey    string
	CastlerSecretKey string

	// Razorpay configuration (future)
	RazorpayBaseURL string
	RazorpayKeyID   string
	RazorpaySecret  string

	// Default provider
	DefaultProvider payout.ProviderType
}

// PayoutProviderFactory creates payout provider instances
type PayoutProviderFactory struct {
	config *PayoutProviderConfig
}

// NewPayoutProviderFactory creates a new provider factory
func NewPayoutProviderFactory(config *PayoutProviderConfig) *PayoutProviderFactory {
	return &PayoutProviderFactory{
		config: config,
	}
}

// GetProvider returns a payout provider instance for the specified type
func (f *PayoutProviderFactory) GetProvider(providerType payout.ProviderType) (payout.PayoutProvider, error) {
	switch providerType {
	case payout.ProviderCastler:
		return castler.NewCastlerClient(
			f.config.CastlerBaseURL,
			f.config.CastlerAPIKey,
			f.config.CastlerSecretKey,
		), nil

	case payout.ProviderRazorpay:
		// TODO: Implement Razorpay client in future
		return nil, fmt.Errorf("Razorpay provider not yet implemented")

	case payout.ProviderBankAPI:
		// TODO: Implement direct bank API client in future
		return nil, fmt.Errorf("Bank API provider not yet implemented")

	default:
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}
}

// GetDefaultProvider returns the configured default provider
func (f *PayoutProviderFactory) GetDefaultProvider() (payout.PayoutProvider, error) {
	if f.config.DefaultProvider == "" {
		f.config.DefaultProvider = payout.ProviderCastler // Default to Castler
	}
	return f.GetProvider(f.config.DefaultProvider)
}

// GetProviderWithFallback returns the primary provider, falling back to secondary if primary fails
func (f *PayoutProviderFactory) GetProviderWithFallback(primaryType payout.ProviderType) (payout.PayoutProvider, error) {
	// Try primary provider first
	provider, err := f.GetProvider(primaryType)
	if err == nil {
		return provider, nil
	}

	// Fallback logic: Castler → Razorpay → Bank API
	switch primaryType {
	case payout.ProviderCastler:
		// Fallback to Razorpay
		return f.GetProvider(payout.ProviderRazorpay)
	case payout.ProviderRazorpay:
		// Fallback to Bank API
		return f.GetProvider(payout.ProviderBankAPI)
	default:
		return nil, fmt.Errorf("no fallback available for provider: %s", primaryType)
	}
}

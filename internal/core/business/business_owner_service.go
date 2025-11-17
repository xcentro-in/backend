package business

import "context"

// BusinessOwnerService provides business owner information for payment gateway operations
// This interface abstracts the complexity of fetching business owner details,
// which may involve multiple repository lookups (business_user_relation → user)
type BusinessOwnerService interface {
	// GetOwnerMobile retrieves the mobile number of the business owner
	// Returns the owner's mobile number in string format
	// Used by payment gateways (e.g., JioPay) that require owner mobile for externalId construction
	GetOwnerMobile(ctx context.Context, businessID string) (string, error)
}

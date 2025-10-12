package keys

import (
	"fmt"
	"time"
)

// TTL constants for different cache types
const (
	// User-related TTLs
	TTLUserProfile      = 5 * time.Minute     // User profile data
	TTLVerification     = 24 * time.Hour      // Verification status
	TTLOnboardingStatus = 5 * time.Minute     // Onboarding status (KYC/KYB flags)
	TTLRevokedToken     = 30 * 24 * time.Hour // Revoked refresh tokens (30 days)

	// Business-related TTLs
	TTLBusinessDetails = 10 * time.Minute // Business details
	TTLUserRoles       = 5 * time.Minute  // User-business relations

	// No TTL (manual invalidation or permanent)
	TTLNone = 0
)

// Key prefix for all XCentro cache keys
const keyPrefix = "xcentro"

// User provides user-related cache key builders
var User = userKeys{}

type userKeys struct{}

// Profile returns cache key for user profile data
// Pattern: xcentro:user:{userId}:profile
func (userKeys) Profile(userID string) string {
	return fmt.Sprintf("%s:user:%s:profile", keyPrefix, userID)
}

// EmailLookup returns cache key for email-to-userID lookup
// Pattern: xcentro:user:email:{email}
func (userKeys) EmailLookup(email string) string {
	return fmt.Sprintf("%s:user:email:%s", keyPrefix, email)
}

// Verification returns cache key for user verification status (KYB initiated, etc.)
// Pattern: xcentro:user:verification:{userId}
func (userKeys) Verification(userID string) string {
	return fmt.Sprintf("%s:user:verification:%s", keyPrefix, userID)
}

// OnboardingStatus returns cache key for user onboarding status (KYC/KYB needed flags)
// Pattern: xcentro:user:{userId}:onboarding
func (userKeys) OnboardingStatus(userID string) string {
	return fmt.Sprintf("%s:user:%s:onboarding", keyPrefix, userID)
}

// Business provides business-related cache key builders
var Business = businessKeys{}

type businessKeys struct{}

// Details returns cache key for business details
// Pattern: xcentro:business:{businessId}
func (businessKeys) Details(businessID string) string {
	return fmt.Sprintf("%s:business:%s", keyPrefix, businessID)
}

// KYBData returns cache key for business KYB data
// Pattern: xcentro:business:{businessId}:kyb
func (businessKeys) KYBData(businessID string) string {
	return fmt.Sprintf("%s:business:%s:kyb", keyPrefix, businessID)
}

// UserRoles returns cache key for user roles in business
// Pattern: xcentro:business:user:{userId}:roles
func (businessKeys) UserRoles(userID string) string {
	return fmt.Sprintf("%s:business:user:%s:roles", keyPrefix, userID)
}

// GSTIN returns cache key for GSTIN-to-businessID lookup
// Pattern: xcentro:business:gstin:{gstin}
func (businessKeys) GSTIN(gstin string) string {
	return fmt.Sprintf("%s:business:gstin:%s", keyPrefix, gstin)
}

// RevokedTokens returns cache key for user's revoked refresh tokens (Redis hash)
// Pattern: xcentro:user:{userId}:token
// Usage: HSET key tokenHash 1, HEXISTS key tokenHash
func (userKeys) RevokedTokens(userID string) string {
	return fmt.Sprintf("%s:user:%s:token", keyPrefix, userID)
}

// InvalidateUser invalidates all user-related cache keys (helper for bulk invalidation)
func InvalidateUser(userID string) []string {
	return []string{
		User.Profile(userID),
		User.Verification(userID),
		User.OnboardingStatus(userID),
		Business.UserRoles(userID),
	}
}

// InvalidateBusiness invalidates all business-related cache keys
func InvalidateBusiness(businessID string) []string {
	return []string{
		Business.Details(businessID),
		Business.KYBData(businessID),
	}
}

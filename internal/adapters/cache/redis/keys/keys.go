package keys

import (
	"fmt"
	"time"
)

// TTL constants for different cache types
const (
	// User-related TTLs
	TTLUserProfile  = 5 * time.Minute  // User profile data
	TTLRevokedToken = 15 * time.Minute // Revoked refresh tokens (15 minutes)

	// Business-related TTLs
	TTLBusinessDetails = 10 * time.Minute // Business details
	TTLUserRoles       = 5 * time.Minute  // User-business relations
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

// Business provides business-related cache key builders
var Business = businessKeys{}

type businessKeys struct{}

// Details returns cache key for business details
// Pattern: xcentro:business:{businessId}
func (businessKeys) Details(businessID string) string {
	return fmt.Sprintf("%s:business:%s", keyPrefix, businessID)
}

// UserRoles returns cache key for user roles in business
// Pattern: xcentro:business:user:{userId}:roles
func (businessKeys) UserRoles(userID string) string {
	return fmt.Sprintf("%s:business:user:%s:roles", keyPrefix, userID)
}

// OnboardingStatus returns cache key for business onboarding status
// Pattern: xcentro:business:{businessId}:onboarding
func (businessKeys) OnboardingStatus(businessID string) string {
	return fmt.Sprintf("%s:business:%s:onboarding", keyPrefix, businessID)
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
		Business.UserRoles(userID),
	}
}

// InvalidateUserRoles invalidates only user roles cache (for role changes without profile updates)
func InvalidateUserRoles(userID string) []string {
	return []string{
		Business.UserRoles(userID),
	}
}

// Notification provides notification-related cache key builders
var Notification = notificationKeys{}

type notificationKeys struct{}

// Preferences returns cache key for user notification preferences
// Pattern: xcentro:notification:prefs:{userId}
func (notificationKeys) Preferences(userID string) string {
	return fmt.Sprintf("%s:notification:prefs:%s", keyPrefix, userID)
}

// TTL for notification preferences (1 hour, with explicit invalidation on updates)
const TTLNotificationPreferences = 1 * time.Hour

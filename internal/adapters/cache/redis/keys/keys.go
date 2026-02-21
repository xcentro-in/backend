package keys

import (
	"fmt"
	"math/rand/v2"
	"time"
)

// WithJitter returns a TTL with ±10% random jitter to prevent synchronized cache expiry
// (thundering herd) where many keys set at the same time would all expire simultaneously.
func WithJitter(ttl time.Duration) time.Duration {
	jitter := time.Duration(rand.N(int64(ttl) / 5)) // 0–20% of TTL
	if rand.N(2) == 0 {
		return ttl - jitter/2 // subtract up to 10%
	}
	return ttl + jitter/2 // add up to 10%
}

// TTL constants for different cache types
const (
	// User-related TTLs
	TTLUserProfile   = 5 * time.Minute // User profile data
	TTLDeviceVersion = 1 * time.Hour   // Device JWT version (1 hour, explicitly invalidated on change)

	// Business-related TTLs
	TTLBusinessDetails = 10 * time.Minute // Business details
	TTLUserRoles       = 5 * time.Minute  // User-business relations
	TTLFeatureFlags    = 15 * time.Minute // Feature flags (longer TTL as they change infrequently)
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

// FeatureFlags provides feature flags cache key builders
var FeatureFlags = featureFlagsKeys{}

type featureFlagsKeys struct{}

// Global returns cache key for global feature flags
// Pattern: xcentro:feature_flags
func (featureFlagsKeys) Global() string {
	return fmt.Sprintf("%s:feature_flags", keyPrefix)
}

// DeviceVersion returns cache key for user's device JWT version number
// Pattern: xcentro:user:{userId}:device:{deviceType}:version
// Stores the current version number for session invalidation
func (userKeys) DeviceVersion(userID string, deviceType string) string {
	return fmt.Sprintf("%s:user:%s:device:%s:version", keyPrefix, userID, deviceType)
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

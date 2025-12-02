package common

// RedactActorName redacts internal system and third-party actor names to prevent
// exposing internal architecture details in customer-facing API responses.
//
// This function is used across multiple endpoints (payment activity, voucher activity)
// to ensure consistent redaction of sensitive actor names.
//
// Redaction rules:
// - Payment gateway actors (mockpg_callback, jiopay_*, easebuzz_*) → "system"
// - Payout provider actors (castler_webhook, castler_api) → "system"
// - Internal handlers/workers → "system"
// - UUID-formatted names (36 chars with dashes at positions 8, 13) → "user"
// - Actual user/admin names → preserved as-is
func RedactActorName(actorName string) string {
	// List of internal/system actors that should be redacted
	internalActors := map[string]bool{
		// Payment gateway webhooks/callbacks
		"mockpg_callback":   true,
		"mockpg_webhook":    true,
		"jiopay_callback":   true,
		"jiopay_webhook":    true,
		"easebuzz_callback": true,
		"easebuzz_webhook":  true,

		// Payout provider webhooks
		"castler_webhook": true,
		"castler_api":     true,

		// Internal handlers/workers
		"payout_handler":   true,
		"payout_worker":    true,
		"payout_service":   true,
		"payment_handler":  true,
		"webhook_handler":  true,
		"settlement_webhook": true,

		// Generic system actors
		"system":  true,
		"webhook": true,
		"user":    false, // Keep user as-is
		"admin":   false, // Keep admin as-is
	}

	// Check if this is an internal actor that should be redacted
	if shouldRedact, exists := internalActors[actorName]; exists && shouldRedact {
		return "system"
	}

	// If actor name looks like a UUID (user ID), keep it as "user"
	// UUID format: 8-4-4-4-12 characters (total 36 with dashes)
	if len(actorName) == 36 && actorName[8] == '-' && actorName[13] == '-' {
		return "user"
	}

	// Default: assume it's a user name/ID, return as-is
	return actorName
}

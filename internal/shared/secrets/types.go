package secrets

import (
	"context"
	"time"
)

// SecretsManager defines the interface for loading secrets
type SecretsManager interface {
	// LoadAllSecrets fetches all secrets as a key-value map
	LoadAllSecrets(ctx context.Context) (map[string]string, error)

	// Close closes the secrets manager and releases resources
	Close() error
}

// Config holds configuration for the secrets manager
type Config struct {
	// AWS Secrets Manager configuration
	AWSRegion   string
	SecretName  string // Secret name (e.g., "xcentro/backend/production")

	// Cache configuration
	UseCache bool
	CacheTTL time.Duration

	// Fallback configuration (for local development)
	UseFallback bool // If true, fall back to environment variables when AWS fails

	// AWS credentials (optional - uses IAM role if not provided)
	AWSAccessKey string
	AWSSecretKey string
}

// Secret key constants for consistency
const (
	KeyJWTSecret                = "jwt_secret_key"
	KeyBcryptPepper             = "bcrypt_pepper"
	KeyBankAccountEncryption    = "bank_account_encryption_key"
	KeyOpsTokenEncryption       = "ops_token_encryption_key"
	KeyRateLimiterHMAC          = "rate_limiter_hmac_secret"

	KeyDBPassword               = "db_password"
	KeyDBMigrationPassword      = "db_migration_password"
	KeyRedisPassword            = "redis_password"

	KeyS3AccessKey              = "s3_access_key"
	KeyS3SecretKey              = "s3_secret_key"
	KeySESAccessKey             = "ses_access_key"
	KeySESSecretKey             = "ses_secret_key"

	KeyJioPayAPIKey             = "jiopay_api_key"
	KeyJioPayAppID              = "jiopay_app_id"
	KeyJioPayEncryptionKey      = "jiopay_encryption_key"
	KeyJioPayWebhookSecret      = "jiopay_webhook_secret"

	KeyEasebuzzMerchantKey      = "easebuzz_merchant_key"
	KeyEasebuzzSalt             = "easebuzz_salt"
	KeyEasebuzzAPIKey           = "easebuzz_api_key"
	KeyEasebuzzMerchantTokenID  = "easebuzz_merchant_token_id"
	KeyEasebuzzEncryptionKey    = "easebuzz_encryption_key"
	KeyEasebuzzEncryptionIV     = "easebuzz_encryption_iv"
	KeyEasebuzzWebhookSecret    = "easebuzz_webhook_secret"

	KeyCastlerAPIKey            = "castler_api_key"
	KeyCastlerSecretKey         = "castler_secret_key"
	KeyCastlerWebhookSecret     = "castler_webhook_secret"
	KeyCastlerEncryptionKey     = "castler_encryption_key"
	KeyCastlerEncryptionIV      = "castler_encryption_iv"
	KeyCastlerEscrowID          = "castler_escrow_id"

	KeyOpsOAuthClientSecret     = "ops_oauth_client_secret"
	KeyTurnstileSecretKey       = "turnstile_secret_key"

	KeyFast2SMSAPIKey           = "fast2sms_api_key"
	KeyTruthscreenPassword      = "truthscreen_password"
)

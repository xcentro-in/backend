package secrets

import (
	"context"
	"os"

	"xcentro.in/internal/shared/logger"
)

// envSecretsManager implements SecretsManager using environment variables (fallback for local dev)
type envSecretsManager struct {
	log *logger.Logger
}

// NewEnvSecretsManager creates a secrets manager that reads from environment variables
func NewEnvSecretsManager() SecretsManager {
	return &envSecretsManager{
		log: logger.GetDefault(),
	}
}

// LoadAllSecrets reads all secrets from environment variables
func (m *envSecretsManager) LoadAllSecrets(ctx context.Context) (map[string]string, error) {
	m.log.Warn("Loading secrets from environment variables (FALLBACK MODE - not recommended for production)")

	secrets := map[string]string{
		KeyJWTSecret:               os.Getenv("JWT_SECRET_KEY"),
		KeyBcryptPepper:            os.Getenv("BCRYPT_PEPPER"),
		KeyBankAccountEncryption:   os.Getenv("BANK_ACCOUNT_ENCRYPTION_KEY"),
		KeyOpsTokenEncryption:      os.Getenv("OPS_TOKEN_ENCRYPTION_KEY"),
		KeyRateLimiterHMAC:         os.Getenv("RATE_LIMITER_HMAC_SECRET"),

		KeyDBPassword:              os.Getenv("DB_PASSWORD"),
		KeyDBMigrationPassword:     os.Getenv("DB_MIGRATION_PASSWORD"),
		KeyRedisPassword:           os.Getenv("REDIS_PASSWORD"),

		KeyS3AccessKey:             os.Getenv("S3_ACCESS_KEY"),
		KeyS3SecretKey:             os.Getenv("S3_SECRET_KEY"),
		KeySESAccessKey:            os.Getenv("SES_ACCESS_KEY"),
		KeySESSecretKey:            os.Getenv("SES_SECRET_KEY"),

		KeyJioPayAPIKey:            os.Getenv("JIOPAY_API_KEY"),
		KeyJioPayAppID:             os.Getenv("JIOPAY_APP_ID"),
		KeyJioPayEncryptionKey:     os.Getenv("JIOPAY_ENCRYPTION_KEY"),
		KeyJioPayWebhookSecret:     os.Getenv("JIOPAY_WEBHOOK_SECRET"),

		KeyEasebuzzMerchantKey:     os.Getenv("EASEBUZZ_MERCHANT_KEY"),
		KeyEasebuzzSalt:            os.Getenv("EASEBUZZ_SALT"),
		KeyEasebuzzAPIKey:          os.Getenv("EASEBUZZ_API_KEY"),
		KeyEasebuzzMerchantTokenID: os.Getenv("EASEBUZZ_MERCHANT_TOKEN_ID"),
		KeyEasebuzzEncryptionKey:   os.Getenv("EASEBUZZ_ENCRYPTION_KEY"),
		KeyEasebuzzEncryptionIV:    os.Getenv("EASEBUZZ_ENCRYPTION_IV"),
		KeyEasebuzzWebhookSecret:   os.Getenv("EASEBUZZ_WEBHOOK_SECRET"),

		KeyCastlerAPIKey:           os.Getenv("CASTLER_API_KEY"),
		KeyCastlerSecretKey:        os.Getenv("CASTLER_SECRET_KEY"),
		KeyCastlerWebhookSecret:    os.Getenv("CASTLER_WEBHOOK_SECRET"),
		KeyCastlerEncryptionKey:    os.Getenv("CASTLER_ENCRYPTION_KEY"),
		KeyCastlerEncryptionIV:     os.Getenv("CASTLER_ENCRYPTION_IV"),
		KeyCastlerEscrowID:         os.Getenv("CASTLER_ESCROW_ID"),

		KeyOpsOAuthClientSecret:    os.Getenv("OPS_OAUTH_CLIENT_SECRET"),
		KeyTurnstileSecretKey:      os.Getenv("TURNSTILE_SECRET_KEY"),
	}

	// Count non-empty secrets
	count := 0
	for _, v := range secrets {
		if v != "" {
			count++
		}
	}

	m.log.InfoWith("Loaded secrets from environment variables",
		"num_secrets", count,
		"mode", "fallback")

	return secrets, nil
}

// Close is a no-op for environment variables
func (m *envSecretsManager) Close() error {
	return nil
}

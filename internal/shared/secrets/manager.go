package secrets

import (
	"context"
	"fmt"
	"time"

	"xcentro.in/internal/shared/logger"
)

// NewSecretsManager creates a new secrets manager based on configuration
// If AWS Secrets Manager fails and UseFallback is true, it falls back to environment variables
func NewSecretsManager(cfg Config) (SecretsManager, error) {
	log := logger.GetDefault()

	// If secret name is empty, use environment variables
	if cfg.SecretName == "" {
		log.Warn("AWS_SECRET_NAME not configured, using environment variables for secrets")
		return NewEnvSecretsManager(), nil
	}

	// Try AWS Secrets Manager
	awsMgr, err := NewAWSSecretsManager(cfg)
	if err != nil {
		if cfg.UseFallback {
			log.WarnWith("Failed to initialize AWS Secrets Manager, falling back to environment variables",
				"error", err.Error())
			return NewEnvSecretsManager(), nil
		}
		return nil, fmt.Errorf("failed to initialize AWS Secrets Manager: %w", err)
	}

	// Test AWS connection by attempting to load secrets
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = awsMgr.LoadAllSecrets(ctx)
	if err != nil {
		if cfg.UseFallback {
			log.WarnWith("Failed to load secrets from AWS Secrets Manager, falling back to environment variables",
				"error", err.Error())
			awsMgr.Close() // Clean up AWS client
			return NewEnvSecretsManager(), nil
		}
		return nil, fmt.Errorf("failed to load secrets from AWS Secrets Manager: %w", err)
	}

	return awsMgr, nil
}

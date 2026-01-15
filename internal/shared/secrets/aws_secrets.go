package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-secretsmanager-caching-go/v2/secretcache"
	"xcentro.in/internal/shared/logger"
)

// awsSecretsManager implements SecretsManager using AWS Secrets Manager
type awsSecretsManager struct {
	client     *secretsmanager.Client
	cache      *secretcache.Cache
	secretName string
	useCache   bool
	log        *logger.Logger
}

// NewAWSSecretsManager creates a new AWS Secrets Manager client
func NewAWSSecretsManager(cfg Config) (SecretsManager, error) {
	if cfg.AWSRegion == "" {
		return nil, fmt.Errorf("aws region is required")
	}
	if cfg.SecretName == "" {
		return nil, fmt.Errorf("secret name is required")
	}

	log := logger.GetDefault()
	ctx := context.Background()

	// Build AWS config options
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.AWSRegion),
	}

	// Use static credentials if provided (for local dev), otherwise use IAM role
	if cfg.AWSAccessKey != "" && cfg.AWSSecretKey != "" {
		log.Warn("Using static AWS credentials (not recommended for production)")
		staticCreds := credentials.NewStaticCredentialsProvider(
			cfg.AWSAccessKey,
			cfg.AWSSecretKey,
			"",
		)
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(staticCreds))
	} else {
		log.Info("Using IAM role for AWS Secrets Manager authentication (recommended)")
	}

	// Load AWS config
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create Secrets Manager client
	client := secretsmanager.NewFromConfig(awsCfg)

	mgr := &awsSecretsManager{
		client:     client,
		secretName: cfg.SecretName,
		useCache:   cfg.UseCache,
		log:        log,
	}

	// Initialize cache if enabled
	if cfg.UseCache {
		cacheConfig := secretcache.CacheConfig{
			MaxCacheSize: 10, // Small cache, we only have one secret
			CacheItemTTL: cfg.CacheTTL,
		}

		cache, err := secretcache.New(
			func(c *secretcache.Cache) { c.CacheConfig = cacheConfig },
			func(c *secretcache.Cache) { c.Client = client },
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize secret cache: %w", err)
		}

		mgr.cache = cache
		log.InfoWith("AWS Secrets Manager initialized with caching",
			"secret_name", cfg.SecretName,
			"cache_ttl", cfg.CacheTTL)
	} else {
		log.InfoWith("AWS Secrets Manager initialized without caching",
			"secret_name", cfg.SecretName)
	}

	return mgr, nil
}

// LoadAllSecrets fetches the secret from AWS Secrets Manager and parses it as JSON
func (m *awsSecretsManager) LoadAllSecrets(ctx context.Context) (map[string]string, error) {
	var secretString string
	var err error

	// Use cache if enabled
	if m.useCache && m.cache != nil {
		m.log.DebugWith("Fetching secret from cache", "secret_name", m.secretName)
		secretString, err = m.cache.GetSecretString(m.secretName)
	} else {
		m.log.DebugWith("Fetching secret from AWS Secrets Manager", "secret_name", m.secretName)
		result, err := m.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
			SecretId: aws.String(m.secretName),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get secret value: %w", err)
		}
		if result.SecretString != nil {
			secretString = *result.SecretString
		} else {
			return nil, fmt.Errorf("secret binary format not supported, use JSON string format")
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to fetch secret: %w", err)
	}

	// Parse JSON
	var secrets map[string]string
	if err := json.Unmarshal([]byte(secretString), &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secret JSON: %w", err)
	}

	m.log.InfoWith("Successfully loaded secrets from AWS Secrets Manager",
		"secret_name", m.secretName,
		"num_secrets", len(secrets))

	return secrets, nil
}

// Close closes the secrets manager (no-op for AWS client)
func (m *awsSecretsManager) Close() error {
	// AWS SDK clients don't need explicit cleanup
	return nil
}

package config

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// AWSConfig represents common AWS configuration fields
type AWSConfig struct {
	Region       string // AWS region (e.g., "us-east-1")
	Endpoint     string // Optional custom endpoint (localstack, MinIO)
	AccessKey    string // Optional static credentials
	SecretKey    string
	SessionToken string
	Profile      string // Optional shared config profile
}

// LoadAWSConfig loads AWS SDK configuration from the provided config struct
// This is a centralized helper to avoid duplicating AWS config loading logic
// across S3, SES, CloudWatch, and other AWS service clients.
func LoadAWSConfig(ctx context.Context, cfg AWSConfig) (aws.Config, error) {
	if cfg.Region == "" {
		return aws.Config{}, fmt.Errorf("aws region is required")
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	// Shared config profile (e.g., "default", "production")
	if cfg.Profile != "" {
		loadOpts = append(loadOpts, awsconfig.WithSharedConfigProfile(cfg.Profile))
	}

	// Custom endpoint (e.g., localstack, MinIO)
	if cfg.Endpoint != "" {
		loadOpts = append(loadOpts, awsconfig.WithBaseEndpoint(cfg.Endpoint))
	}

	// Static credentials override (if provided)
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		staticCreds := credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			cfg.SessionToken,
		)
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(staticCreds))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("load AWS config: %w", err)
	}

	// Defensive region fallback (check environment variable)
	if awsCfg.Region == "" {
		if envRegion := os.Getenv("AWS_REGION"); envRegion != "" {
			awsCfg.Region = envRegion
		}
	}

	if awsCfg.Region == "" {
		return aws.Config{}, fmt.Errorf("aws region could not be resolved")
	}

	return awsCfg, nil
}

package config

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"xcentro.in/internal/adapters/database/postgres/client"
)

// Config holds the application configuration.
// Sensitive fields (like Password) are excluded from JSON marshalling.
type Config struct {
	Environment           string                      `json:"environment" env:"ENVIRONMENT" env-default:"dev"` // dev or production
	Database              DatabaseConfig              `json:"database"`
	MigrationDatabase     MigrationDatabaseConfig     `json:"migration_database"`
	Server                ServerConfig                `json:"server"`
	Rcache                RedisConfig                 `json:"rcache"`
	JWT                   JWTSecretConfig             `json:"jwt"`
	Bcrypt                BcryptConfig                `json:"bcrypt"`
	Cookie                CookieConfig                `json:"cookie"`
	CORS                  CORSConfig                  `json:"cors"`
	S3                    S3Config                    `json:"s3"`
	SES                   SESConfig                   `json:"ses"`
	SMS                   SMSConfig                   `json:"sms"`
	Castler               CastlerConfig               `json:"castler"`
	Payout                PayoutConfig                `json:"payout"`
	JioPay                JioPayConfig                `json:"jiopay"`
	Easebuzz              EasebuzzConfig              `json:"easebuzz"`
	MockPG                MockPGConfig                `json:"mockpg"`
	CloudWatch            CloudWatchConfig            `json:"cloudwatch"`
	Workers               WorkersConfig               `json:"workers"`
	Frontend              FrontendConfig              `json:"frontend"`
	BankAccountEncryption BankAccountEncryptionConfig `json:"bank_account_encryption"`
	AuthBridge            AuthBridgeConfig            `json:"authbridge"`
	RateLimiter           RateLimiterConfig           `json:"rate_limiter"`
	OpsAccess             OpsAccessConfig             `json:"ops_access"`
	Logger                LoggerConfig                `json:"logger"`
	OpsOAuth              OpsOAuthConfig              `json:"ops_oauth"`
	OpsTokenEncryption    OpsTokenEncryptionConfig    `json:"ops_token_encryption"`
}

// JWTSecretConfig holds JWT secret configuration.
type JWTSecretConfig struct {
	SecretKey          string        `json:"secret_key" env:"JWT_SECRET_KEY" env-required:"true"`
	AccessTokenExpiry  time.Duration `json:"access_token_expiry" env:"JWT_ACCESS_TOKEN_EXPIRY" env-default:"1h"`
	RefreshTokenExpiry time.Duration `json:"refresh_token_expiry" env:"JWT_REFRESH_TOKEN_EXPIRY" env-default:"720h"` // 30 days
}

// BcryptConfig holds bcrypt password hashing configuration.
type BcryptConfig struct {
	Cost   int    `json:"cost" env:"BCRYPT_COST" env-default:"12"`
	Pepper string `json:"-" env:"BCRYPT_PEPPER" env-required:"true"`
}

// CookieConfig holds cookie configuration for authentication.
type CookieConfig struct {
	Secure   bool   `json:"secure" env:"COOKIE_SECURE" env-default:"false"`    // Set to true in production (HTTPS only)
	SameSite string `json:"same_site" env:"COOKIE_SAMESITE" env-default:"Lax"` // Strict, Lax, or None
	Domain   string `json:"domain" env:"COOKIE_DOMAIN" env-default:""`         // Empty for current domain
}

// CORSConfig holds CORS (Cross-Origin Resource Sharing) configuration.
type CORSConfig struct {
	AllowOrigins     []string `json:"allow_origins" env:"CORS_ALLOW_ORIGINS" env-default:"http://localhost:8080"`
	AllowCredentials bool     `json:"allow_credentials" env:"CORS_ALLOW_CREDENTIALS" env-default:"true"`
	MaxAge           int      `json:"max_age" env:"CORS_MAX_AGE" env-default:"86400"` // 24 hours
}

// S3Config holds AWS S3 related configuration.
// Access/Secret keys are optional if the environment uses IAM roles or other providers.
type S3Config struct {
	Bucket       string `json:"bucket" env:"S3_BUCKET" env-required:"true"`
	Region       string `json:"region" env:"S3_REGION" env-required:"true"`
	Endpoint     string `json:"endpoint" env:"S3_ENDPOINT"`             // optional custom endpoint (MinIO/localstack)
	UsePathStyle bool   `json:"use_path_style" env:"S3_USE_PATH_STYLE"` // set true for MinIO/localstack
	AccessKey    string `json:"-" env:"S3_ACCESS_KEY"`                  // optional static credentials
	SecretKey    string `json:"-" env:"S3_SECRET_KEY"`
	SessionToken string `json:"-" env:"S3_SESSION_TOKEN"`
	Profile      string `json:"profile" env:"AWS_PROFILE"` // optional shared profile
}

// SESConfig holds AWS SES related configuration.
// Access/Secret keys are optional if the environment uses IAM roles or other providers.
type SESConfig struct {
	DefaultFromEmail      string `json:"default_from_email" env:"SES_DEFAULT_FROM_EMAIL" env-required:"true"`
	DefaultFromName       string `json:"default_from_name" env:"SES_DEFAULT_FROM_NAME"`
	NotificationFromEmail string `json:"notification_from_email" env:"SES_NOTIFICATION_FROM_EMAIL" env-default:"noreply@transactions.xcentro.in"`
	NotificationFromName  string `json:"notification_from_name" env:"SES_NOTIFICATION_FROM_NAME" env-default:"Xcentro Notifications"`
	DefaultConfigSet      string `json:"default_config_set" env:"SES_DEFAULT_CONFIG_SET"`
	Region                string `json:"region" env:"SES_REGION" env-required:"true"`
	Endpoint              string `json:"endpoint" env:"SES_ENDPOINT"` // optional custom endpoint (localstack)
	AccessKey             string `json:"-" env:"SES_ACCESS_KEY"`      // optional static credentials
	SecretKey             string `json:"-" env:"SES_SECRET_KEY"`
	SessionToken          string `json:"-" env:"SES_SESSION_TOKEN"`
	Profile               string `json:"profile" env:"AWS_PROFILE"` // optional shared profile
}

// SMSConfig holds AWS End User Messaging SMS configuration.
// Access/Secret keys are optional if the environment uses IAM roles or other providers.
type SMSConfig struct {
	Region           string        `json:"region" env:"SMS_REGION" env-required:"true"`
	OriginationID    string        `json:"origination_id" env:"SMS_ORIGINATION_ID" env-default:"Xcentro"` // Sender ID (alphanumeric, max 11 chars)
	Endpoint         string        `json:"endpoint" env:"SMS_ENDPOINT"`                                   // optional custom endpoint (localstack)
	AccessKey        string        `json:"-" env:"SMS_ACCESS_KEY"`                                        // optional static credentials
	SecretKey        string        `json:"-" env:"SMS_SECRET_KEY"`
	SessionToken     string        `json:"-" env:"SMS_SESSION_TOKEN"`
	Profile          string        `json:"profile" env:"AWS_PROFILE"`                                                // optional shared profile
	OTPExpiryMinutes int           `json:"otp_expiry_minutes" env:"SMS_OTP_EXPIRY_MINUTES" env-default:"10"`         // OTP expiry in minutes
	OTPLength        int           `json:"otp_length" env:"SMS_OTP_LENGTH" env-default:"6"`                          // OTP length (default 6 digits)
	OTPExpiry        time.Duration `json:"otp_expiry" env:"SMS_OTP_EXPIRY" env-default:"10m"`                        // Calculated from OTPExpiryMinutes
}

// CastlerConfig holds Castler payout provider configuration.
type CastlerConfig struct {
	BaseURL       string `json:"base_url" env:"CASTLER_BASE_URL" env-default:"https://api.castler.in"`
	APIKey        string `json:"-" env:"CASTLER_API_KEY"`                                     // API authentication key (sensitive)
	SecretKey     string `json:"-" env:"CASTLER_SECRET_KEY"`                                  // API signing secret (sensitive)
	WebhookSecret string `json:"-" env:"CASTLER_WEBHOOK_SECRET"`                              // Webhook signature verification (sensitive)
	EncryptionKey string `json:"-" env:"CASTLER_ENCRYPTION_KEY"`                              // AES encryption key (optional, sensitive)
	EncryptionIV  string `json:"-" env:"CASTLER_ENCRYPTION_IV"`                               // Fixed AES IV in base64 (optional, sensitive)
	EscrowID      string `json:"-" env:"CASTLER_ESCROW_ID"`                                   // Escrow account ID (sensitive)
	MockEnabled   bool   `json:"mock_enabled" env:"CASTLER_MOCK_ENABLED" env-default:"false"` // Enable mock mode for testing (no real API calls)
}

// PayoutConfig holds payout system configuration
// All values hardcoded - no need for env variables until automated retry is implemented
type PayoutConfig struct {
	MaxRetryAttempts       int     // Hardcoded: 3 (not used until auto-retry implemented)
	RetryDelayMinutes      int     // Hardcoded: 15 (not used until auto-retry implemented)
	RetryBackoffMultiplier float64 // Hardcoded: 2.0 (not used until auto-retry implemented)
	WebhookTimeoutMinutes  int     // Hardcoded: 60 minutes (flags transfer for manual review if no webhook)
	RTGSThresholdAmount    int64   // Hardcoded: 20000000 paise (₹2,00,000) - NEFT below, RTGS above
}

// NewPayoutConfig returns hardcoded payout configuration
func NewPayoutConfig() PayoutConfig {
	return PayoutConfig{
		MaxRetryAttempts:       3,
		RetryDelayMinutes:      15,
		RetryBackoffMultiplier: 2.0,
		WebhookTimeoutMinutes:  60,       // 1 hour
		RTGSThresholdAmount:    20000000, // ₹2 lakh
	}
}

// JioPayConfig holds JioPay payment gateway configuration.
type JioPayConfig struct {
	BaseURL             string `json:"base_url" env:"JIOPAY_BASE_URL" env-default:"https://api.jiopay.com"`
	TokenizationBaseURL string `json:"tokenization_base_url" env:"JIOPAY_TOKENIZATION_BASE_URL" env-default:"https://uat.jiopay.co.in/qatsp/api/vault"` // Vault API base URL
	MerchantID          string `json:"merchant_id" env:"JIOPAY_MERCHANT_ID"`                                                                            // Merchant identifier
	APIKey              string `json:"-" env:"JIOPAY_API_KEY"`                                                                                          // API authentication key for HMAC (sensitive)
	AppID               string `json:"-" env:"JIOPAY_APP_ID"`                                                                                           // x-app-id header for vault APIs (sensitive)
	EncryptionKey       string `json:"-" env:"JIOPAY_ENCRYPTION_KEY"`                                                                                   // AES-128 encryption key for x-app-token (sensitive)
	WebhookSecret       string `json:"-" env:"JIOPAY_WEBHOOK_SECRET"`                                                                                   // Webhook signature verification (sensitive)
	IsAggregatorModel   bool   `json:"is_aggregator_model" env:"JIOPAY_IS_AGGREGATOR_MODEL" env-default:"false"`                                        // Payment model: true = aggregator (PG charges customer), false = normal (merchant charges customer)
}

// EasebuzzConfig holds Easebuzz payment gateway configuration.
type EasebuzzConfig struct {
	BaseURL           string `json:"base_url" env:"EASEBUZZ_BASE_URL" env-default:"https://pay.easebuzz.in"`
	MerchantKey       string `json:"-" env:"EASEBUZZ_MERCHANT_KEY"`                                              // Merchant key (sensitive)
	Salt              string `json:"-" env:"EASEBUZZ_SALT"`                                                      // Hash salt (sensitive)
	APIKey            string `json:"-" env:"EASEBUZZ_API_KEY"`                                                   // API key for tokenization (EBZ-KEY header, sensitive)
	EncryptionKey     string `json:"-" env:"EASEBUZZ_ENCRYPTION_KEY"`                                            // AES encryption key (sensitive, optional)
	EncryptionIV      string `json:"-" env:"EASEBUZZ_ENCRYPTION_IV"`                                             // Fixed AES IV in base64 (sensitive, optional)
	WebhookSecret     string `json:"-" env:"EASEBUZZ_WEBHOOK_SECRET"`                                            // Webhook signature verification (sensitive)
	IsAggregatorModel bool   `json:"is_aggregator_model" env:"EASEBUZZ_IS_AGGREGATOR_MODEL" env-default:"false"` // Payment model: true = aggregator (PG charges customer), false = normal (merchant charges customer)
}

// MockPGConfig holds Mock Payment Gateway configuration for testing.
type MockPGConfig struct {
	Enabled           bool   `json:"enabled" env:"MOCKPG_ENABLED" env-default:"false"`                         // Enable mock PG (bypass real gateways)
	MerchantID        string `json:"merchant_id" env:"MOCKPG_MERCHANT_ID" env-default:"MOCK_MERCH"`            // Mock merchant identifier
	APIKey            string `json:"-" env:"MOCKPG_API_KEY" env-default:"mock_api_key_123"`                    // Mock API key (sensitive)
	SimulateSuccess   bool   `json:"simulate_success" env:"MOCKPG_SIMULATE_SUCCESS" env-default:"true"`        // If true, always succeed; if false, simulate random failures
	IsAggregatorModel bool   `json:"is_aggregator_model" env:"MOCKPG_IS_AGGREGATOR_MODEL" env-default:"false"` // Payment model: true = aggregator (PG charges customer), false = normal (merchant charges customer)
}

// CloudWatchConfig holds AWS CloudWatch metrics configuration.
type CloudWatchConfig struct {
	Enabled         bool          `json:"enabled" env:"CLOUDWATCH_ENABLED" env-default:"false"`                    // Enable/disable CloudWatch metrics
	Namespace       string        `json:"namespace" env:"CLOUDWATCH_NAMESPACE" env-default:"XCentro/Backend"`      // CloudWatch namespace
	Region          string        `json:"region" env:"CLOUDWATCH_REGION" env-default:"us-east-1"`                  // AWS region
	Endpoint        string        `json:"endpoint" env:"CLOUDWATCH_ENDPOINT"`                                      // Optional custom endpoint (localstack)
	AccessKey       string        `json:"-" env:"CLOUDWATCH_ACCESS_KEY"`                                           // Optional static credentials
	SecretKey       string        `json:"-" env:"CLOUDWATCH_SECRET_KEY"`                                           // Optional static credentials
	SessionToken    string        `json:"-" env:"CLOUDWATCH_SESSION_TOKEN"`                                        // Optional session token
	Profile         string        `json:"profile" env:"AWS_PROFILE"`                                               // Optional shared profile
	BatchInterval   time.Duration `json:"batch_interval" env:"CLOUDWATCH_BATCH_INTERVAL" env-default:"30s"`        // Batch metrics every N seconds
	BatchSize       int           `json:"batch_size" env:"CLOUDWATCH_BATCH_SIZE" env-default:"20"`                 // Max metrics per batch (CloudWatch limit: 1000)
	MaxRetries      int           `json:"max_retries" env:"CLOUDWATCH_MAX_RETRIES" env-default:"5"`                // Max retry attempts with exponential backoff
	FlushOnShutdown bool          `json:"flush_on_shutdown" env:"CLOUDWATCH_FLUSH_ON_SHUTDOWN" env-default:"true"` // Flush pending metrics on shutdown
}

// RedisConfig holds Redis cache configuration.
type RedisConfig struct {
	Host           string `json:"host" env:"REDIS_HOST" env-default:"localhost"`
	Port           int    `json:"port" env:"REDIS_PORT" env-default:"6379"`
	Username       string `json:"username" env:"REDIS_USERNAME" env-default:""`
	Password       string `json:"-" env:"REDIS_PASSWORD"` // no weak default
	DB             int    `json:"db" env:"REDIS_DB" env-default:"0"`
	MaxConnections int    `json:"max_connections" env:"REDIS_MAX_CONNECTIONS" env-default:"10"`
}

// DatabaseConfig holds database configuration.
type DatabaseConfig struct {
	Host            string        `json:"host" env:"DB_HOST" env-default:"localhost"`
	Port            int           `json:"port" env:"DB_PORT" env-default:"5432"`
	Database        string        `json:"database" env:"DB_NAME" env-default:"xcentro"`
	Username        string        `json:"username" env:"DB_USER" env-default:"postgres"`
	Password        string        `json:"-" env:"DB_PASSWORD" env-required:"true"` // no weak default
	Schema          string        `json:"schema" env:"DB_SCHEMA" env-default:"xcentro"`
	MaxConnections  int32         `json:"max_connections" env:"DB_MAX_CONNECTIONS" env-default:"25"`
	MinConnections  int32         `json:"min_connections" env:"DB_MIN_CONNECTIONS" env-default:"5"`
	MaxConnLifetime time.Duration `json:"max_conn_lifetime" env:"DB_MAX_CONN_LIFETIME" env-default:"1h"`
	MaxConnIdleTime time.Duration `json:"max_conn_idle_time" env:"DB_MAX_CONN_IDLE_TIME" env-default:"30m"`
	SSLMode         string        `json:"ssl_mode" env:"DB_SSL_MODE" env-default:"disable"`
}

// MigrationDatabaseConfig holds database configuration specifically for migrations.
type MigrationDatabaseConfig struct {
	Host     string `json:"host" env:"DB_HOST" env-default:"localhost"`
	Port     int    `json:"port" env:"DB_PORT" env-default:"5432"`
	Database string `json:"database" env:"DB_NAME" env-default:"xcentro"`
	Username string `json:"username" env:"DB_MIGRATION_USER" env-default:"postgres"`
	Password string `json:"-" env:"DB_MIGRATION_PASSWORD" env-required:"true"` // no weak default
	Schema   string `json:"schema" env:"DB_SCHEMA" env-default:"xcentro"`
	SSLMode  string `json:"ssl_mode" env:"DB_SSL_MODE" env-default:"disable"`
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	Host string `json:"host" env:"SERVER_HOST" env-default:"0.0.0.0"`
	Port int    `json:"port" env:"SERVER_PORT" env-default:"8080"`
	URL  string `json:"url" env:"SERVER_URL" env-default:"http://localhost:8888"` // Public-facing backend URL for callbacks
}

// FrontendConfig holds frontend application configuration.
type FrontendConfig struct {
	URL string `json:"url" env:"FRONTEND_URL" env-default:"http://localhost:3000"`
}

// WorkersConfig holds worker configuration.
type WorkersConfig struct {
	Notification NotificationWorkerConfig `json:"notification"`
}

// NotificationWorkerConfig holds notification worker configuration.
type NotificationWorkerConfig struct {
	// Fanout configuration
	FanoutBatchSize    int `json:"fanout_batch_size" env:"NOTIFICATION_FANOUT_BATCH_SIZE" env-default:"50"`
	FanoutPollInterval int `json:"fanout_poll_interval_seconds" env:"NOTIFICATION_FANOUT_POLL_INTERVAL_SECONDS" env-default:"2"`
	FanoutWorkers      int `json:"fanout_workers" env:"NOTIFICATION_FANOUT_WORKERS" env-default:"1"`

	// Dispatch configuration
	DispatchBatchSize    int `json:"dispatch_batch_size" env:"NOTIFICATION_DISPATCH_BATCH_SIZE" env-default:"50"`
	DispatchPollInterval int `json:"dispatch_poll_interval_seconds" env:"NOTIFICATION_DISPATCH_POLL_INTERVAL_SECONDS" env-default:"2"`
	EmailDispatchWorkers int `json:"email_dispatch_workers" env:"NOTIFICATION_EMAIL_DISPATCH_WORKERS" env-default:"2"`
	PushDispatchWorkers  int `json:"push_dispatch_workers" env:"NOTIFICATION_PUSH_DISPATCH_WORKERS" env-default:"0"` // 0 = disabled (MVP)

	// Retry configuration
	MaxRetryAttempts int `json:"max_retry_attempts" env:"NOTIFICATION_MAX_RETRY_ATTEMPTS" env-default:"3"`

	// Stuck job recovery configuration
	ProcessingTimeoutMinutes int `json:"processing_timeout_minutes" env:"NOTIFICATION_PROCESSING_TIMEOUT_MINUTES" env-default:"10"`
	RecoveryWorkers          int `json:"recovery_workers" env:"NOTIFICATION_RECOVERY_WORKERS" env-default:"1"`
	RecoveryIntervalSeconds  int `json:"recovery_interval_seconds" env:"NOTIFICATION_RECOVERY_INTERVAL_SECONDS" env-default:"60"`
}

// BankAccountEncryptionConfig holds bank account encryption configuration for PCI DSS compliance.
type BankAccountEncryptionConfig struct {
	EncryptionKey string `json:"-" env:"BANK_ACCOUNT_ENCRYPTION_KEY" env-required:"true"` // AES-256-GCM key (sensitive, 32 bytes recommended)
}

// AuthBridgeConfig holds AuthBridge/TruthScreen document verification service configuration.
type AuthBridgeConfig struct {
	Username string `json:"username" env:"TRUTHSCREEN_USERNAME" env-required:"true"` // AuthBridge/TruthScreen username
	Password string `json:"-" env:"TRUTHSCREEN_PASSWORD" env-required:"true"`        // AuthBridge/TruthScreen password (sensitive)
	UseMock  bool   `json:"use_mock" env:"AUTHBRIDGE_USE_MOCK" env-default:"false"`  // Use mock responses instead of real API calls
}

// OpsAccessConfig holds configuration for ops dashboard access control.
type OpsAccessConfig struct {
	RejectAccess bool `json:"reject_access" env:"OPS_REJECT_ACCESS" env-default:"true"` // Reject all ops dashboard requests by default
}

// LoggerConfig holds logger configuration.
type LoggerConfig struct {
	Level  string `json:"level" env:"LOG_LEVEL" env-default:"info"`   // Log level: debug, info, warn, error
	Format string `json:"format" env:"LOG_FORMAT" env-default:"text"` // Log format: text, json
}

// OpsOAuthConfig holds OAuth2/OIDC configuration for ops dashboard authentication.
type OpsOAuthConfig struct {
	ClientID       string `json:"client_id" env:"OPS_OAUTH_CLIENT_ID" env-required:"true"`         // Office365 Application (client) ID
	ClientSecret   string `json:"-" env:"OPS_OAUTH_CLIENT_SECRET" env-required:"true"`             // Office365 Client secret (sensitive)
	RedirectURL    string `json:"redirect_url" env:"OPS_OAUTH_REDIRECT_URL" env-required:"true"`   // OAuth callback URL (e.g., https://ops.xcentro.in/ops/v1/auth/callback)
	TenantID       string `json:"tenant_id" env:"OPS_OAUTH_TENANT_ID" env-default:"organizations"` // Office365 Tenant ID (default: organizations for multi-tenant)
	SkipValidation bool   `json:"skip_validation" env:"OPS_OAUTH_SKIP_VALIDATION" env-default:"false"` // Skip OAuth token validation in middleware (dev/testing only)
}

// OpsTokenEncryptionConfig holds encryption configuration for storing OIDC tokens.
type OpsTokenEncryptionConfig struct {
	EncryptionKey string `json:"-" env:"OPS_TOKEN_ENCRYPTION_KEY" env-required:"true"` // AES-256-GCM key for encrypting OIDC refresh tokens (32 bytes)
}

// RateLimiterConfig holds rate limiter configuration for token-bucket algorithm.
type RateLimiterConfig struct {
	Enabled                      bool          `json:"enabled" env:"RATE_LIMITER_ENABLED" env-default:"true"`
	HMACSecret                   string        `json:"-" env:"RATE_LIMITER_HMAC_SECRET" env-required:"true"` // Secret for signing device IDs
	CookieName                   string        `json:"cookie_name" env:"RATE_LIMITER_COOKIE_NAME" env-default:"device_id"`
	CookieTTL                    time.Duration `json:"cookie_ttl" env:"RATE_LIMITER_COOKIE_TTL" env-default:"720h"` // 30 days
	DeviceCreationThrottlePerIP  int           `json:"device_creation_throttle_per_ip" env:"RATE_LIMITER_DEVICE_CREATION_THROTTLE_PER_IP" env-default:"50"`
	DeviceCreationThrottleWindow time.Duration `json:"device_creation_throttle_window" env:"RATE_LIMITER_DEVICE_CREATION_THROTTLE_WINDOW" env-default:"1h"`
	InvalidSignaturePenalty      time.Duration `json:"invalid_signature_penalty" env:"RATE_LIMITER_INVALID_SIGNATURE_PENALTY" env-default:"1h"` // Penalty for tampered device signatures
	FailOpenByDefault            bool          `json:"fail_open_by_default" env:"RATE_LIMITER_FAIL_OPEN_BY_DEFAULT" env-default:"true"`
	// Note: FailOpenByDefault only affects the "default" policy. Other built-in policies (login, signup, etc.) have their own hardcoded FailClosed settings
	// Note: Token bucket TTL is hardcoded to 30 minutes in Lua script (lua_script.go line 48)
	// Note: Fixed window TTL is automatically set to window_seconds in the strategy

	// Default policy (used as fallback for any group not in JSON)
	DefaultCapacity        int64   `json:"default_capacity" env:"RATE_LIMITER_DEFAULT_CAPACITY" env-default:"120"`
	DefaultRefillPerSec    float64 `json:"default_refill_per_sec" env:"RATE_LIMITER_DEFAULT_REFILL_PER_SEC" env-default:"2.0"` // 120 req/min
	DefaultRequestedTokens int64   `json:"default_requested_tokens" env:"RATE_LIMITER_DEFAULT_REQUESTED_TOKENS" env-default:"1"`

	// GroupPoliciesJSON is a JSON string defining rate limit policies for custom groups
	// Format: {"group_name": {"capacity": 100, "refill_per_second": 2.0, "requested_tokens": 1, "fail_closed": false}}
	// If JSON contains "default" key, it overrides the individual default fields above
	// If empty, only the default policy from individual fields is used
	GroupPoliciesJSON string `json:"group_policies_json" env:"RATE_LIMITER_GROUP_POLICIES_JSON" env-default:""`
}

var (
	loadedCfg Config
	once      sync.Once
	loadErr   error
)

// Load reads configuration from optional file path (YAML/JSON/TOML supported by cleanenv) plus env overrides.
func Load(path string) (Config, error) {
	var cfg Config
	var err error

	if err = cleanenv.ReadConfig(path, &cfg); err != nil {
		fmt.Printf("error reading config file: %v\n", err)
		if err = cleanenv.ReadEnv(&cfg); err != nil {
			return cfg, fmt.Errorf("read env: %w", err)
		}
	}

	// Set hardcoded payout configuration (not configurable via env)
	cfg.Payout = NewPayoutConfig()

	return cfg, err
}

// MustLoad loads configuration once and exits on failure.
func MustLoad(path string) Config {
	once.Do(func() {
		loadedCfg, loadErr = Load(path)
	})
	if loadErr != nil {
		log.Fatalf("configuration error: %v", loadErr)
	}
	return loadedCfg
}

// ToDBConfig converts DatabaseConfig to client.DBConfig.
func (dc DatabaseConfig) ToDBConfig() client.DBConfig {
	return client.DBConfig{
		Host:            dc.Host,
		Port:            dc.Port,
		Database:        dc.Database,
		Username:        dc.Username,
		Password:        dc.Password,
		Schema:          dc.Schema,
		MaxConnections:  dc.MaxConnections,
		MinConnections:  dc.MinConnections,
		MaxConnLifetime: dc.MaxConnLifetime,
		MaxConnIdleTime: dc.MaxConnIdleTime,
		SSLMode:         dc.SSLMode,
	}
}

// ToDBConfig converts MigrationDatabaseConfig to client.DBConfig.
func (mdc MigrationDatabaseConfig) ToDBConfig() client.DBConfig {
	return client.DBConfig{
		Host:            mdc.Host,
		Port:            mdc.Port,
		Database:        mdc.Database,
		Username:        mdc.Username,
		Password:        mdc.Password,
		Schema:          mdc.Schema,
		MaxConnections:  1, // migrations typically need only one connection
		MinConnections:  1,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
		SSLMode:         mdc.SSLMode,
	}
}

// Safe returns a redacted copy for logging/metrics.
func (c Config) Safe() Config {
	c.Database.Password = "***REDACTED***"
	c.MigrationDatabase.Password = "***REDACTED***"
	c.JWT.SecretKey = "***REDACTED***"
	c.Bcrypt.Pepper = "***REDACTED***"
	c.Rcache.Password = "***REDACTED***"
	c.S3.AccessKey = "***REDACTED***"
	c.S3.SecretKey = "***REDACTED***"
	c.S3.SessionToken = "***REDACTED***"
	c.SES.AccessKey = "***REDACTED***"
	c.SES.SecretKey = "***REDACTED***"
	c.SES.SessionToken = "***REDACTED***"
	c.SMS.AccessKey = "***REDACTED***"
	c.SMS.SecretKey = "***REDACTED***"
	c.SMS.SessionToken = "***REDACTED***"
	c.Castler.APIKey = "***REDACTED***"
	c.Castler.SecretKey = "***REDACTED***"
	c.Castler.WebhookSecret = "***REDACTED***"
	c.Castler.EncryptionKey = "***REDACTED***"
	c.Castler.EncryptionIV = "***REDACTED***"
	c.JioPay.APIKey = "***REDACTED***"
	c.JioPay.AppID = "***REDACTED***"
	c.JioPay.EncryptionKey = "***REDACTED***"
	c.JioPay.WebhookSecret = "***REDACTED***"
	c.Easebuzz.MerchantKey = "***REDACTED***"
	c.Easebuzz.Salt = "***REDACTED***"
	c.Easebuzz.APIKey = "***REDACTED***"
	c.Easebuzz.EncryptionKey = "***REDACTED***"
	c.Easebuzz.EncryptionIV = "***REDACTED***"
	c.Easebuzz.WebhookSecret = "***REDACTED***"
	c.MockPG.APIKey = "***REDACTED***"
	c.BankAccountEncryption.EncryptionKey = "***REDACTED***"
	c.OpsOAuth.ClientSecret = "***REDACTED***"
	c.OpsTokenEncryption.EncryptionKey = "***REDACTED***"
	return c
}

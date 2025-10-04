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
	Database          DatabaseConfig          `json:"database"`
	MigrationDatabase MigrationDatabaseConfig `json:"migration_database"`
	Server            ServerConfig            `json:"server"`
	Rcache            RedisConfig             `json:"rcache"`
	JWT               JWTSecretConfig         `json:"jwt"`
	S3                S3Config                `json:"s3"`
	SES               SESConfig               `json:"ses"`
}

// JWTSecretConfig holds JWT secret configuration.
type JWTSecretConfig struct {
	SecretKey string `json:"secret_key" env:"JWT_SECRET_KEY" env-required:"true"`
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
	DefaultFromEmail string `json:"default_from_email" env:"SES_DEFAULT_FROM_EMAIL" env-required:"true"`
	DefaultFromName  string `json:"default_from_name" env:"SES_DEFAULT_FROM_NAME"`
	DefaultConfigSet string `json:"default_config_set" env:"SES_DEFAULT_CONFIG_SET"`
	Region           string `json:"region" env:"SES_REGION" env-required:"true"`
	Endpoint         string `json:"endpoint" env:"SES_ENDPOINT"` // optional custom endpoint (localstack)
	AccessKey        string `json:"-" env:"SES_ACCESS_KEY"`      // optional static credentials
	SecretKey        string `json:"-" env:"SES_SECRET_KEY"`
	SessionToken     string `json:"-" env:"SES_SESSION_TOKEN"`
	Profile          string `json:"profile" env:"AWS_PROFILE"` // optional shared profile
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
}

var (
	loadedCfg Config
	once      sync.Once
	loadErr   error
)

// Load reads configuration from optional file path (YAML/JSON/TOML supported by cleanenv) plus env overrides.
func Load(path string) (Config, error) {
	var cfg Config
	if path != "" {
		if err := cleanenv.ReadConfig(path, &cfg); err != nil {
			return cfg, fmt.Errorf("read config file: %w", err)
		}
		// env overrides
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			return cfg, fmt.Errorf("read env: %w", err)
		}
	} else {
		// Try to load from .env file first if it exists
		if err := cleanenv.ReadConfig(".env", &cfg); err != nil {
			// If .env doesn't exist or fails to load, continue with env vars only
			log.Printf("No .env file found or failed to load, using environment variables only: %v", err)
		}
		// Load/override from environment variables
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			return cfg, fmt.Errorf("read env: %w", err)
		}
	}
	return cfg, nil
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
	return c
}

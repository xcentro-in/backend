package config

import (
	"errors"
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
	Database DatabaseConfig `json:"database"`
	Server   ServerConfig   `json:"server"`
}

// DatabaseConfig holds database configuration.
type DatabaseConfig struct {
	Host            string        `json:"host" env:"DB_HOST" env-default:"localhost"`
	Port            int           `json:"port" env:"DB_PORT" env-default:"5432"`
	Database        string        `json:"database" env:"DB_NAME" env-default:"xcentro"`
	Username        string        `json:"username" env:"DB_USER" env-default:"postgres"`
	Password        string        `json:"-" env:"DB_PASSWORD" env-required:"true"` // no weak default
	MaxConnections  int32         `json:"max_connections" env:"DB_MAX_CONNECTIONS" env-default:"25"`
	MinConnections  int32         `json:"min_connections" env:"DB_MIN_CONNECTIONS" env-default:"5"`
	MaxConnLifetime time.Duration `json:"max_conn_lifetime" env:"DB_MAX_CONN_LIFETIME" env-default:"1h"`
	MaxConnIdleTime time.Duration `json:"max_conn_idle_time" env:"DB_MAX_CONN_IDLE_TIME" env-default:"30m"`
	SSLMode         string        `json:"ssl_mode" env:"DB_SSL_MODE" env-default:"disable"`
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
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			return cfg, fmt.Errorf("read env: %w", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("validation failed: %w", err)
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

// Validate performs sanity and security checks.
func (c *Config) Validate() error {
	db := c.Database

	if db.Password == "" {
		return errors.New("database password is required (DB_PASSWORD)")
	}
	if db.MinConnections < 0 {
		return errors.New("min connections cannot be negative")
	}
	if db.MaxConnections <= 0 {
		return errors.New("max connections must be > 0")
	}
	if db.MinConnections > db.MaxConnections {
		return errors.New("min connections cannot exceed max connections")
	}
	if db.MaxConnLifetime < time.Minute {
		return errors.New("max_conn_lifetime should be at least 1m")
	}
	if db.MaxConnIdleTime < 30*time.Second {
		return errors.New("max_conn_idle_time should be at least 30s")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return errors.New("server port out of range")
	}
	switch db.SSLMode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("unsupported ssl_mode: %s", db.SSLMode)
	}
	return nil
}

// ToDBConfig converts DatabaseConfig to client.DBConfig.
func (dc DatabaseConfig) ToDBConfig() client.DBConfig {
	return client.DBConfig{
		Host:            dc.Host,
		Port:            dc.Port,
		Database:        dc.Database,
		Username:        dc.Username,
		Password:        dc.Password,
		MaxConnections:  dc.MaxConnections,
		MinConnections:  dc.MinConnections,
		MaxConnLifetime: dc.MaxConnLifetime,
		MaxConnIdleTime: dc.MaxConnIdleTime,
		SSLMode:         dc.SSLMode,
	}
}

// Safe returns a redacted copy for logging/metrics.
func (c Config) Safe() Config {
	c.Database.Password = "***REDACTED***"
	return c
}

// DefaultConfig kept for tests or local fallback (still enforces required password if used directly).
func DefaultConfig() Config {
	return Config{
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Database:        "xcentro",
			Username:        "postgres",
			Password:        "", // force explicit set in tests if used
			MaxConnections:  25,
			MinConnections:  5,
			MaxConnLifetime: time.Hour,
			MaxConnIdleTime: 30 * time.Minute,
			SSLMode:         "disable",
		},
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
	}
}

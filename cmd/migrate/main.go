package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"xcentro.in/internal/adapters/database/postgres/migrations"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/secrets"
)

func main() {
	// Parse command line flags
	var (
		action  = flag.String("action", "up", "Migration action: up, down, version, goto, force")
		version = flag.String("version", "", "Target version (for goto/force actions)")
		path    = flag.String("path", "internal/adapters/database/postgres/migrations/sql", "Path to migration files")
		envFile = flag.String("env", ".env", "Path to environment file (default: .env)")
	)
	flag.Parse()

	log := logger.GetDefault()
	ctx := context.Background()

	// Load .env file into environment variables
	if err := godotenv.Load(*envFile); err != nil {
		log.Warnf("Failed to load %s file: %v (will use existing environment variables)", *envFile, err)
	}

	// Initialize secrets manager
	useFallback, _ := strconv.ParseBool(os.Getenv("SECRETS_USE_FALLBACK"))
	secretsMgr, err := secrets.NewSecretsManager(secrets.Config{
		AWSRegion:    os.Getenv("AWS_REGION"),
		SecretName:   os.Getenv("AWS_SECRET_NAME"),
		UseCache:     true,
		CacheTTL:     15 * time.Minute,
		UseFallback:  useFallback,
		AWSAccessKey: os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	})
	if err != nil {
		fmt.Printf("Failed to initialize secrets manager: %v\n", err)
		os.Exit(1)
	}
	defer secretsMgr.Close()

	// Load all secrets
	secretsMap, err := secretsMgr.LoadAllSecrets(ctx)
	if err != nil {
		fmt.Printf("Failed to load secrets: %v\n", err)
		os.Exit(1)
	}

	// Load configuration with secrets
	cfg, err := config.LoadWithSecrets(*envFile, secretsMap)
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}
	log.Info("Config loaded successfully with secrets")

	// Initialize migration service
	migrationService := migrations.NewMigrationService(*path, cfg.MigrationDatabase)

	// Execute migration action
	switch *action {
	case "up":
		log.Info("Running up migrations...")
		if err := migrationService.Up(ctx); err != nil {
			log.Errorf("Failed to run up migrations: %v", err)
			os.Exit(1)
		}
		log.Info("Up migrations completed successfully")

	case "down":
		log.Info("Running down migration (1 step)...")
		if err := migrationService.Down(ctx); err != nil {
			log.Errorf("Failed to run down migration: %v", err)
			os.Exit(1)
		}
		log.Info("Down migration completed successfully")

	case "version":
		version, dirty, err := migrationService.Version(ctx)
		if err != nil {
			log.Errorf("Failed to get migration version: %v", err)
			os.Exit(1)
		}
		log.Infof("Current migration version: %d (dirty: %t)", version, dirty)

	case "goto":
		if *version == "" {
			log.Error("Version flag is required for goto action")
			os.Exit(1)
		}
		targetVersion, err := strconv.ParseUint(*version, 10, 32)
		if err != nil {
			log.Errorf("Invalid version number: %v", err)
			os.Exit(1)
		}
		log.Infof("Migrating to version %d...", targetVersion)
		if err := migrationService.MigrateToVersion(ctx, uint(targetVersion)); err != nil {
			log.Errorf("Failed to migrate to version %d: %v", targetVersion, err)
			os.Exit(1)
		}
		log.Infof("Successfully migrated to version %d", targetVersion)

	case "force":
		if *version == "" {
			log.Error("Version flag is required for force action")
			os.Exit(1)
		}
		targetVersion, err := strconv.Atoi(*version)
		if err != nil {
			log.Errorf("Invalid version number: %v", err)
			os.Exit(1)
		}
		log.Warnf("Forcing migration version to %d (without running migrations)...", targetVersion)
		if err := migrationService.Force(ctx, targetVersion); err != nil {
			log.Errorf("Failed to force version %d: %v", targetVersion, err)
			os.Exit(1)
		}
		log.Infof("Successfully forced version to %d", targetVersion)

	case "drop":
		log.Warn("DANGER: This will completely wipe all tables and data!")
		log.Info("Running drop (wipe database)...")
		if err := migrationService.Drop(ctx); err != nil {
			log.Errorf("Failed to drop database: %v", err)
			os.Exit(1)
		}
		log.Info("Database wiped successfully")

	default:
		fmt.Printf("Usage: %s -action=<up|down|version|goto|force|drop> [-version=<number>]\n", os.Args[0])
		fmt.Println("Actions:")
		fmt.Println("  up      - Run all pending up migrations")
		fmt.Println("  down    - Run one down migration")
		fmt.Println("  version - Show current migration version")
		fmt.Println("  goto    - Migrate to specific version (requires -version flag)")
		fmt.Println("  force   - Force migration version (for fixing dirty state, requires -version flag)")
		fmt.Println("  drop    - DANGER: Completely wipe the database (removes all tables and data)")
		os.Exit(1)
	}
}

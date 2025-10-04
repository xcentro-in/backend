package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"xcentro.in/internal/adapters/database/postgres/migrations"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

func main() {
	// Parse command line flags
	var (
		action  = flag.String("action", "up", "Migration action: up, down, version, goto, force")
		version = flag.String("version", "", "Target version (for goto/force actions)")
		path    = flag.String("path", "internal/adapters/database/postgres/migrations/sql", "Path to migration files")
	)
	flag.Parse()

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	log := logger.GetDefault()

	// Initialize migration service
	migrationService := migrations.NewMigrationService(*path, cfg.MigrationDatabase)

	ctx := context.Background()

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

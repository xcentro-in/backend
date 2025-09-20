package container

import (
	"context"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/database/postgres/user_repository"
	"xcentro.in/internal/services/user"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// Container holds all the application dependencies
type Container struct {
	config    config.Config
	db        client.DB
	txManager client.TxManager
	logger    *logger.Logger

	// Repositories
	UserRepo *user_repository.PgUserRepository

	// Services
	UserService *user.UserService
}

// NewContainer creates a new dependency injection container
func NewContainer(cfg config.Config) (*Container, error) {
	container := &Container{
		config: cfg,
		logger: logger.GetDefault(),
	}

	if err := container.initDatabase(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	container.initRepositories()
	container.initServices()

	return container, nil
}

// initDatabase initializes the database connection and transaction manager
func (c *Container) initDatabase() error {
	ctx := context.Background()

	db, err := client.NewPgxDB(ctx, c.config.Database.ToDBConfig())
	if err != nil {
		return fmt.Errorf("failed to create database connection: %w", err)
	}

	c.db = db
	c.txManager = client.NewTxManager(db)

	c.logger.Info("Database connection established successfully")
	return nil
}

// initRepositories initializes all repository instances
func (c *Container) initRepositories() {
	c.UserRepo = user_repository.NewPgUserRepository(c.db)
	c.logger.Info("Repositories initialized successfully")
}

// initServices initializes all service instances
func (c *Container) initServices() {
	c.UserService = user.NewUserService(c.UserRepo, c.txManager)
	c.logger.Info("Services initialized successfully")
}

// Close gracefully shuts down all connections
func (c *Container) Close() {
	if c.db != nil {
		c.db.Close()
		c.logger.Info("Database connections closed")
	}
}

// GetDB returns the database connection (useful for health checks)
func (c *Container) GetDB() client.DB {
	return c.db
}

// GetTxManager returns the transaction manager
func (c *Container) GetTxManager() client.TxManager {
	return c.txManager
}

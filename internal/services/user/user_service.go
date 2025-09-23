package user

import (
	"context"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
)

// UserService provides business logic for user operations
type UserService struct {
	userRepo  *userRepo.PgUserRepository
	txManager client.TxManager
	logger    *logger.Logger
}

// NewUserService creates a new user service with dependency injection
func NewUserService(userRepo *userRepo.PgUserRepository, txManager client.TxManager) *UserService {
	return &UserService{
		userRepo:  userRepo,
		txManager: txManager,
		logger:    logger.GetDefault(),
	}
}

// CreateUser creates a new user with proper validation and transaction handling
func (s *UserService) CreateUser(ctx context.Context, authPayload auth.AuthPayload) (*user.User, error) {
	s.logger.Info("Creating user with email: %s", authPayload.Email)

	// Validate input
	if authPayload.Email == "" {
		return nil, fmt.Errorf("email is required")
	}

	var createdUser *user.User

	// Use transaction to ensure data consistency
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Create repository instance that uses the transaction
		txUserRepo := s.userRepo.WithTx(tx)

		// Check if user already exists
		existingUser, err := txUserRepo.GetUserByEmail(ctx, authPayload.Email)
		if err == nil && existingUser != nil {
			s.logger.Errorf("user with email %s already exists", authPayload.Email)
			return nil
		}

		if err != nil {
			s.logger.Errorf("failed to check existing user: %v", err)
			return err
		}

		// Create new user
		createdUser, err = txUserRepo.CreateUser(ctx, authPayload)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}

		s.logger.Info("User created successfully with ID: %s", createdUser.Id)
		return nil
	})

	if err != nil {
		s.logger.Error("Failed to create user: %v", err)
		return nil, err
	}

	return createdUser, nil
}

// GetUserByEmail retrieves a user by email
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*user.User, error) {
	s.logger.Info("Getting user by email: %s", email)

	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		s.logger.Error("Failed to get user by email: %v", err)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetUserByID retrieves a user by ID
func (s *UserService) GetUserByID(ctx context.Context, userID string) (*user.User, error) {
	s.logger.Info("Getting user by ID: %s", userID)

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user by ID: %v", err)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// UpdateUserProfile updates user profile information in a transaction
func (s *UserService) UpdateUserProfile(ctx context.Context, userID string, mobile, pan string) error {
	s.logger.Info("Updating user profile for ID: %s", userID)

	return s.txManager.WithTx(context.Background(), func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		if mobile != "" {
			if err := txUserRepo.UpdateUserMobile(ctx, userID, mobile); err != nil {
				return fmt.Errorf("failed to update mobile: %w", err)
			}
		}

		if pan != "" {
			if err := txUserRepo.UpdateUserPAN(ctx, userID, pan); err != nil {
				return fmt.Errorf("failed to update PAN: %w", err)
			}
		}

		return nil
	})
}

// UpdateKYCStatus updates user's KYC status
func (s *UserService) UpdateKYCStatus(ctx context.Context, userID string, status shared.KYCStatus, kycData string) error {
	s.logger.Info("Updating KYC status for user ID: %s to status: %v", userID, status)

	return s.txManager.WithTx(context.Background(), func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		if err := txUserRepo.UpdateUserKYCStatus(ctx, userID, status); err != nil {
			return fmt.Errorf("failed to update KYC status: %w", err)
		}

		if kycData != "" {
			if err := txUserRepo.UpdateUserKYCData(ctx, userID, kycData); err != nil {
				return fmt.Errorf("failed to update KYC data: %w", err)
			}
		}

		return nil
	})
}

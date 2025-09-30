package business

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	businessRepo "xcentro.in/internal/adapters/database/postgres/business"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/database/postgres/user"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/shared/logger"
)

// BusinessServiceImpl provides business logic for business operations
type BusinessServiceImpl struct {
	businessRepo        *businessRepo.PgBusinessRepository
	businessUserRelRepo *businessRepo.PgBusinessUserRelationRepository
	userRepo            *user.PgUserRepository
	txManager           client.TxManager
	rclient             *redis.Client
	logger              *logger.Logger
}

// NewBusinessService creates a new business service with dependency injection
func NewBusinessService(
	businessRepo *businessRepo.PgBusinessRepository,
	businessUserRelRepo *businessRepo.PgBusinessUserRelationRepository,
	userRepo *user.PgUserRepository,
	txManager client.TxManager,
	rclient *redis.Client,
) *BusinessServiceImpl {
	return &BusinessServiceImpl{
		businessRepo:        businessRepo,
		businessUserRelRepo: businessUserRelRepo,
		userRepo:            userRepo,
		txManager:           txManager,
		rclient:             rclient,
		logger:              logger.GetDefault(),
	}
}

func (s *BusinessServiceImpl) GetUserKYBInitiatedStatus(ctx context.Context, uid string) (string, error) {
	relations, err := s.businessUserRelRepo.FindByUser(ctx, uid)
	if err != nil {
		return "", fmt.Errorf("error finding user-business relations: %w", err)
	}

	// If no relations found, KYB is not initiated
	if len(relations) == 0 {
		return "", nil
	}

	// Check if any relation has KYB initiated
	if len(relations) == 1 && relations[0].KybInitiated {
		return relations[0].BID, nil
	}

	return "", nil
}

func (s *BusinessServiceImpl) CreateBusinessByNameAndGSTIN(ctx context.Context, name string, gstin string, uid string) (*business.Business, error) {
	s.logger.Info("Creating business with name: %s and GSTIN: %s", name, gstin)

	if name == "" || gstin == "" {
		return nil, fmt.Errorf("business name and GSTIN are required")
	}

	existingBusiness, err := s.businessRepo.FindByGSTIN(ctx, gstin)
	if err != nil {
		return nil, fmt.Errorf("error finding business by GSTIN: %w", err)
	}
	if existingBusiness != nil {
		return nil, fmt.Errorf("business with GSTIN %s already exists", gstin)
	}

	relations, err := s.businessUserRelRepo.FindByUser(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("error finding user-business relations: %w", err)
	}

	// Check if user is already associated with a business
	if len(relations) > 0 {
		return nil, fmt.Errorf("user is already associated with a business")
	}

	business, err := s.businessRepo.CreateBusinessByNameAndGSTIN(ctx, name, gstin)
	if err != nil {
		return nil, fmt.Errorf("error creating business: %w", err)
	}

	if business == nil {
		return nil, fmt.Errorf("failed to create business")
	}

	err = s.businessUserRelRepo.UpdateKYBInitiated(ctx, uid, business.BID, true)
	if err != nil {
		return nil, fmt.Errorf("error updating KYB initiated status: %w", err)
	}

	cmd := s.rclient.HSet(ctx, "xcentro:user:verification:"+uid, "is_kyb_initiated", true)
	if err := cmd.Err(); err != nil {
		return nil, fmt.Errorf("error setting Redis cache: %w", err)
	}

	return business, nil
}

func (s *BusinessServiceImpl) UpdateBusinessKYBData(ctx context.Context, bdto *business.BusinessKYBDTO) error {
	if bdto == nil {
		return fmt.Errorf("business KYB DTO is required")
	}

	kybdata, err := s.businessRepo.UpdateKYBData(ctx, bdto.BID, &business.BusinessKYBData{
		ITID:     bdto.ITID,
		ITIDData: "{}",
		CID:      bdto.CID,
		CIDData:  "{}",
		DocUrl:   "",
	})

	if err != nil {
		return fmt.Errorf("error updating business KYB data: %w", err)
	}

	if kybdata == nil {
		return fmt.Errorf("failed to update business KYB data")
	}

	err = s.businessRepo.AddKYBSuccessData(ctx, &business.Business{
		BID:          bdto.BID,
		KybStatus:    shared.KYBCompleted,
		BusinessType: bdto.BusinessType,
		ITID:         bdto.ITID,
	})

	if err != nil {
		return fmt.Errorf("error updating business KYB success data: %w", err)
	}

	return nil
}

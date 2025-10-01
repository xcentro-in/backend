package business

import (
	"context"
	"fmt"

	"github.com/labstack/echo/v4"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
)

// BusinessServiceImpl provides business logic for business operations
type BusinessServiceImpl struct {
	businessRepo        business.BusinessRepository
	businessUserRelRepo business.BusinessUserRelationRepository
	userRepo            user.UserRepository
	txManager           client.TxManager
	rclient             *redis.RedisClient
	logger              *logger.Logger
}

// NewBusinessService creates a new business service with dependency injection
func NewBusinessService(
	businessRepo business.BusinessRepository,
	businessUserRelRepo business.BusinessUserRelationRepository,
	userRepo user.UserRepository,
	txManager client.TxManager,
	rclient *redis.RedisClient,
) business.BusinessService {
	return &BusinessServiceImpl{
		businessRepo:        businessRepo,
		businessUserRelRepo: businessUserRelRepo,
		userRepo:            userRepo,
		txManager:           txManager,
		rclient:             rclient,
		logger:              logger.GetDefault(),
	}
}

func (s *BusinessServiceImpl) GetUserRolesMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID := c.Get("userId").(string)
			rel, err := s.businessUserRelRepo.FindByUser(c.Request().Context(), userID)
			if err != nil {
				s.logger.Errorf("%v", err)
			}
			if len(rel) > 0 {
				c.Set("userRoles", rel)
			}
			return next(c)
		}
	}
}

func (s *BusinessServiceImpl) GetUserKYBInitiatedStatus(ctx context.Context, uid string) (*business.CreateBusinessDTO, error) {
	kybData, bus, err := s.businessUserRelRepo.GetKYBInitiatedStatus(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("error finding user-business relations: %w", err)
	}
	if bus == nil {
		return nil, nil
	}

	dto := &business.CreateBusinessDTO{
		Name:         bus.Name,
		GSTIN:        bus.GSTIN,
		BusinessType: bus.BusinessType,
		ITID:         kybData.ITID,
		CID:          kybData.CID,
		DocUrl:       kybData.DocUrl,
	}

	return dto, nil
}

func (s *BusinessServiceImpl) CreateBusinessByNameAndGSTIN(ctx context.Context, uid string, dto *business.CreateBusinessDTO) (*business.CreateBusinessDTO, error) {
	s.logger.Info("Creating business with name: %s and GSTIN: %s", dto.Name, dto.GSTIN)

	if dto.Name == "" || dto.GSTIN == "" {
		return nil, fmt.Errorf("business name and GSTIN are required")
	}

	existingBusiness, err := s.businessRepo.FindByGSTIN(ctx, dto.GSTIN)
	if err != nil {
		return nil, fmt.Errorf("error finding business by GSTIN: %w", err)
	}
	if existingBusiness != nil {
		return nil, fmt.Errorf("business with GSTIN %s already exists", dto.GSTIN)
	}

	bus, err := s.businessRepo.CreateBusinessByNameAndGSTIN(ctx, dto.Name, dto.GSTIN)
	if err != nil {
		return nil, fmt.Errorf("error creating business: %w", err)
	}

	if bus == nil {
		return nil, fmt.Errorf("failed to create business")
	}

	err = s.businessUserRelRepo.AddUserToBusiness(ctx, &business.BusinessUserRelation{
		UserID:       uid,
		BID:          bus.BID,
		KybInitiated: true,
		Role:         shared.Support,
		RoleClaim:    shared.Support,
	})
	if err != nil {
		return nil, fmt.Errorf("error updating KYB initiated status: %w", err)
	}

	cmd := s.rclient.Client.HSet(ctx, "xcentro:user:verification:"+uid, "is_kyb_initiated", true)
	if err := cmd.Err(); err != nil {
		return nil, fmt.Errorf("error setting Redis cache: %w", err)
	}

	return &business.CreateBusinessDTO{
		Id:    bus.BID,
		Name:  bus.Name,
		GSTIN: bus.GSTIN,
	}, nil
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

func (s *BusinessServiceImpl) UpdateBusinessRoleClaim(ctx context.Context, uid, bid string, roleClaim shared.UserRole) error {
	if uid == "" || bid == "" {
		return fmt.Errorf("user ID and business ID are required")
	}

	err := s.businessUserRelRepo.UpdateUserRole(ctx, uid, bid, roleClaim)
	if err != nil {
		return fmt.Errorf("error updating role claim: %w", err)
	}

	return nil
}

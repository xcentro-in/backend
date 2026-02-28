package business

// This file is compiled only during testing (package business, not business_test).
// It exposes internal BusinessServiceImpl constructor helpers for unit tests in
// the business_test package.
//
// Approach chosen: export_test.go constructor injection (preferred over //go:build ignore)
// because the authService field exists on BusinessServiceImpl (added in Plan 05-01 scaffold).
// Plan 05-07 wires authService into the production NewBusinessService constructor and
// calls RevokeAllUserTokens in UpdateUserRoleInBusiness, RemoveUserFromBusiness, and
// OpsUpdateUserRole. Until then, the tests fail (RED) because revocation is absent.

import (
	coreAuth "xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/adapters/database/postgres/client"
	redisClient "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/shared/logger"
)

// NewBusinessServiceWithAuth creates a minimal BusinessServiceImpl for unit testing
// revocation wiring. It sets only the fields required to run UpdateUserRoleInBusiness,
// RemoveUserFromBusiness, and OpsUpdateUserRole: txManager, businessUserRelRepo,
// userRepo, rclient, authService, and logger.
//
// All other fields (paymentRepo, verificationService, s3Config, externalClients,
// userService) are left nil; tests that exercise those paths will panic.
func NewBusinessServiceWithAuth(
	txManager client.TxManager,
	businessUserRelRepo business.BusinessUserRelationRepository,
	userRepo user.UserRepository,
	rclient *redisClient.RedisClient,
	authSvc coreAuth.AuthService,
) *BusinessServiceImpl {
	return &BusinessServiceImpl{
		txManager:           txManager,
		businessUserRelRepo: businessUserRelRepo,
		userRepo:            userRepo,
		rclient:             rclient,
		authService:         authSvc,
		logger:              logger.GetDefault(),
	}
}

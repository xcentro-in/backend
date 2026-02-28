package user

// This file is compiled only during testing (package user, not user_test).
// It exposes internal UserServiceImpl constructor helpers for unit tests in
// the user_test package.
//
// Approach chosen: export_test.go constructor injection (preferred over //go:build ignore)
// because the authService field exists on UserServiceImpl (added in Plan 05-01 scaffold).
// Plan 05-07 wires authService into the production NewUserService constructor and
// calls RevokeAllUserTokens in RejectKYC. Until then, tests fail (RED) because
// the revocation call is absent.

import (
	coreAuth "xcentro.in/internal/core/auth"
	coreBusinessRepo "xcentro.in/internal/core/business"
	coreUser "xcentro.in/internal/core/user"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/shared/logger"
)

// NewUserServiceWithAuth creates a minimal UserServiceImpl for unit testing
// revocation wiring. It sets only the fields required to run RejectKYC:
// txManager, userRepo, businessUserRelRepo, authService, and logger.
//
// All other fields (emailService, verificationService, config, rclient,
// businessRepo) are left nil; tests that exercise those paths will panic.
func NewUserServiceWithAuth(
	txManager client.TxManager,
	userRepo coreUser.UserRepository,
	businessUserRelRepo coreBusinessRepo.BusinessUserRelationRepository,
	authSvc coreAuth.AuthService,
) *UserServiceImpl {
	return &UserServiceImpl{
		txManager:           txManager,
		userRepo:            userRepo,
		businessUserRelRepo: businessUserRelRepo,
		authService:         authSvc,
		logger:              logger.GetDefault(),
	}
}

package auth

// This file is compiled only during testing (package auth, not auth_test).
// It exposes internal AuthService constructor helpers for unit tests in auth_test package.

import (
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/config"
)

// NewAuthServiceForBuildUnifiedLogin creates a minimal AuthService for unit testing
// BuildUnifiedLoginResponse. Only the three dependencies used by that method are set.
// All other fields are left at zero values (nil); tests that call other methods will panic.
func NewAuthServiceForBuildUnifiedLogin(
	userRepo user.UserRepository,
	bizUserRelRepo business.BusinessUserRelationRepository,
	bizService business.BusinessService,
) *AuthService {
	return &AuthService{
		userRepo:            userRepo,
		businessUserRelRepo: bizUserRelRepo,
		businessService:     bizService,
	}
}

// NewAuthServiceForSwitchBusiness creates a minimal AuthService for unit testing
// SwitchBusiness. Wires the four deps needed: userRepo, bizUserRelRepo, bizService,
// and jwtConfig (required to call generateToken inside the service).
// All other fields are left at zero values (nil); tests calling other methods will panic.
func NewAuthServiceForSwitchBusiness(
	userRepo user.UserRepository,
	bizUserRelRepo business.BusinessUserRelationRepository,
	bizService business.BusinessService,
	jwtConfig *config.JWTSecretConfig,
) *AuthService {
	return &AuthService{
		userRepo:            userRepo,
		businessUserRelRepo: bizUserRelRepo,
		businessService:     bizService,
		config:              jwtConfig,
	}
}

package business

import (
	"context"

	"github.com/labstack/echo/v4"
)

// BusinessService defines the business logic interface for business operations
type BusinessService interface {
	GetUserKYBInitiatedStatus(ctx context.Context, uid string) (string, error)
	CreateBusinessByNameAndGSTIN(ctx context.Context, uid string, dto *CreateBusinessDTO) (*CreateBusinessDTO, error)
	UpdateBusinessKYBData(ctx context.Context, bdto *BusinessKYBDTO) error
	GetUserRolesMiddleware() echo.MiddlewareFunc
}

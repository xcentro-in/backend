package http

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/shared/logger"
)

// OpsBusinessHandler handles ops dashboard business-related endpoints
type OpsBusinessHandler struct {
	businessService business.BusinessService
	authService     auth.AuthService
	logger          *logger.Logger
}

// NewOpsBusinessHandler creates a new ops business handler instance
func NewOpsBusinessHandler(businessService business.BusinessService, authService auth.AuthService) *OpsBusinessHandler {
	return &OpsBusinessHandler{
		businessService: businessService,
		authService:     authService,
		logger:          logger.GetDefault(),
	}
}

// RegisterOpsBusinessRoutes registers all ops dashboard business routes
func RegisterOpsBusinessRoutes(e *echo.Group, businessService business.BusinessService, authService auth.AuthService) {
	handler := NewOpsBusinessHandler(businessService, authService)

	businessGroup := e.Group("/businesses")

	// TODO: Add ops dashboard business routes here
	// Examples:
	// businessGroup.GET("", handler.ListBusinesses)
	// businessGroup.GET("/:business_id", handler.GetBusinessDetails)
	// businessGroup.PUT("/:business_id/status", handler.UpdateBusinessStatus)
	// businessGroup.GET("/:business_id/kyb", handler.GetKYBDetails)
	// businessGroup.PUT("/:business_id/kyb/verify", handler.VerifyKYB)
	// businessGroup.GET("/:business_id/users", handler.GetBusinessUsers)

	_ = handler // Suppress unused variable warning
	_ = businessGroup
}

// TODO: Add handler methods here
// Example:
// func (h *OpsBusinessHandler) ListBusinesses(c echo.Context) error {
//     // Implementation
//     return c.JSON(200, map[string]string{"message": "list businesses"})
// }

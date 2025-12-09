package opsauth

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/opsauth"
	"xcentro.in/internal/shared/logger"
)

// OpsUserHandler handles ops user profile requests
type OpsUserHandler struct {
	authService opsauth.OpsAuthService
}

// NewOpsUserHandler creates a new ops user handler
func NewOpsUserHandler(authService opsauth.OpsAuthService) *OpsUserHandler {
	return &OpsUserHandler{
		authService: authService,
	}
}

// HandleGetProfile retrieves the authenticated ops user's profile
// GET /ops/v1/user/profile (requires JWT middleware)
func (h *OpsUserHandler) HandleGetProfile(c echo.Context) error {
	// Get ops user ID from context (set by middleware)
	opsUserID, ok := c.Get("opsUserId").(string)
	if !ok || opsUserID == "" {
		return c.JSON(401, map[string]any{"message": "Unauthorized", "data": nil})
	}

	// Get user profile
	profile, err := h.authService.GetUserProfile(c.Request().Context(), opsUserID)
	if err != nil {
		logger.Errorf("Failed to get ops user profile: %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to retrieve profile", "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "Profile retrieved successfully",
		"data":    profile,
	})
}

// RegisterOpsUserRoutes registers ops user routes (all routes require JWT authentication)
func RegisterOpsUserRoutes(e *echo.Group, authService opsauth.OpsAuthService) {
	handler := NewOpsUserHandler(authService)

	userGroup := e.Group("/user")

	// Protected routes (require JWT middleware)
	userGroup.GET("/profile", handler.HandleGetProfile)
}

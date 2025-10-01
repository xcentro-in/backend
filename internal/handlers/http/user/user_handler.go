package user

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
)

type UserHandler struct {
	userService user.UserService
	authService auth.AuthService
	logger      *logger.Logger
}

func NewUserHandler(userService user.UserService, authService auth.AuthService) *UserHandler {
	return &UserHandler{
		userService: userService,
		authService: authService,
		logger:      logger.GetDefault(),
	}
}

func RegisterUserRoutes(e *echo.Group, userService user.UserService, authService auth.AuthService) {
	handler := NewUserHandler(userService, authService)

	userGroup := e.Group("/user")
	userGroup.Use(authService.GetEchoAuthMiddleware())

	userGroup.POST("/verify-mobile", handler.VerifyMobile)
	userGroup.POST("/kyc", handler.InitiateKYC)
	userGroup.GET("/kyc", handler.GetKYCStatus)
	userGroup.POST("/claim-role", handler.ClaimBusinessRole)
}

// VerifyMobile verifies user's mobile number using verification code
func (h *UserHandler) VerifyMobile(c echo.Context) error {
	var dto user.VerifyMobileDTO
	if err := c.Bind(&dto); err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request payload"})
	}

	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(400, map[string]string{"message": "Unauthorized request"})
	}

	if err := h.userService.VerifyMobile(c.Request().Context(), userID, dto.Code); err != nil {
		h.logger.Errorf("Error verifying mobile for user %s: %v", userID, err)
		return c.JSON(400, map[string]string{"message": err.Error()})
	}

	return c.JSON(200, map[string]interface{}{"message": "Mobile verified successfully", "data": nil})
}

// InitiateKYC initiates KYC process for the user
func (h *UserHandler) InitiateKYC(c echo.Context) error {
	var kycDTO user.CreateUserKYCCTO
	if err := c.Bind(&kycDTO); err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request payload"})
	}

	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(400, map[string]string{"message": "Unauthorized request"})
	}

	if err := h.userService.InitiateKYC(c.Request().Context(), userID, kycDTO.ClaimedRole, kycDTO.TaxID, kycDTO.NationalID); err != nil {
		h.logger.Errorf("Error initiating KYC for user %s: %v", userID, err)
		if err.Error() == "KYC already completed or verified" {
			return c.JSON(403, map[string]interface{}{"message": err.Error(), "data": nil})
		}
		return c.JSON(400, map[string]string{"message": err.Error()})
	}

	return c.JSON(200, map[string]interface{}{"message": "KYC process initiated successfully", "data": nil})
}

// GetKYCStatus gets user's KYC status and identity information
func (h *UserHandler) GetKYCStatus(c echo.Context) error {
	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(400, map[string]string{"message": "Unauthorized request"})
	}

	kycData, err := h.userService.GetUserKYCStatus(c.Request().Context(), userID)
	if err != nil {
		h.logger.Errorf("Error getting KYC status for user %s: %v", userID, err)
		if err.Error() == "KYC already completed and verified" {
			return c.JSON(403, map[string]interface{}{"message": err.Error(), "data": nil})
		}
		return c.JSON(500, map[string]string{"message": "Failed to get KYC status"})
	}

	return c.JSON(200, map[string]interface{}{"message": "Fetched KYC status", "data": kycData})
}

// ClaimBusinessRole allows user to claim a role in a business they initiated KYB for
func (h *UserHandler) ClaimBusinessRole(c echo.Context) error {
	var dto user.ClaimRoleDTO
	if err := c.Bind(&dto); err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request payload"})
	}

	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(400, map[string]string{"message": "Unauthorized request"})
	}

	if err := h.userService.ClaimBusinessRole(c.Request().Context(), userID, dto.BusinessID, dto.Role); err != nil {
		h.logger.Errorf("Error claiming role for user %s in business %s: %v", userID, dto.BusinessID, err)
		if err.Error() == "User not associated with this business" || err.Error() == "KYB not initiated for this business" {
			return c.JSON(403, map[string]interface{}{"message": err.Error(), "data": nil})
		}
		return c.JSON(500, map[string]string{"message": err.Error()})
	}

	return c.JSON(200, map[string]interface{}{"message": "Role claim updated successfully", "data": nil})
}

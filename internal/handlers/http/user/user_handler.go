package user

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/providers/authbridge"
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

	// Email verification routes don't require authentication
	userGroup.GET("/verify-email", handler.VerifyEmail)
	userGroup.POST("/resend-verification", handler.ResendEmailVerification)

	// Protected routes requiring authentication
	userGroup.Use(authService.GetEchoAuthMiddleware())

	userGroup.POST("/verify-mobile", handler.VerifyMobile)
	userGroup.POST("/kyc", handler.InitiateKYC)
	userGroup.GET("", handler.GetKYCStatus)
	userGroup.POST("/claim-role", handler.ClaimBusinessRole)
	userGroup.GET("/authbridge", handler.KYCHandler)
	userGroup.POST("/aadhaar/generate-link", handler.GenerateAadhaarLink)
	userGroup.POST("/aadhaar/complete-verification", handler.CompleteAadhaarVerification)
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

func (h *UserHandler) KYCHandler(c echo.Context) error {
	client, err := authbridge.NewClient()

	if err != nil {
		h.logger.Errorf("Error creating authbridge client: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to create authbridge client"})
	}

	/*
	   First link generation then comes the status check
	*/
	// data, err := client.SearchDocument(authbridge.NewAadhaarLinkRequest("txn1", "https://xlrewewjyrtacywbki5eg576xy0pqpmu.lambda-url.us-east-1.on.aws/", "http://localhost:8888/v1/aadhaar"))
	data, err := client.SearchDocument(authbridge.NewAadhaarStatusRequest("20-BFE-282478"))

	if err != nil {
		h.logger.Errorf("Error searching document in authbridge: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to search document in authbridge"})
	}

	return c.JSON(200, map[string]interface{}{"message": "Authbridge response", "data": data})
}

// GenerateAadhaarLink generates an Aadhaar verification link for the user
func (h *UserHandler) GenerateAadhaarLink(c echo.Context) error {
	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(400, map[string]string{"message": "Unauthorized request"})
	}

	linkResponse, err := h.userService.GenerateAadhaarLink(c.Request().Context(), userID)
	if err != nil {
		h.logger.Errorf("Error generating Aadhaar link for user %s: %v", userID, err)

		// Handle specific error cases
		if err.Error() == "KYC already completed or verified" {
			return c.JSON(403, map[string]interface{}{"message": err.Error(), "data": nil})
		}
		if err.Error() == "user must have tax ID (PAN) before Aadhaar verification" {
			return c.JSON(400, map[string]interface{}{"message": err.Error(), "data": nil})
		}

		return c.JSON(500, map[string]string{"message": "Failed to generate Aadhaar link"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Aadhaar verification link generated successfully",
		"data":    linkResponse,
	})
}

// CompleteAadhaarVerification completes the Aadhaar verification process
func (h *UserHandler) CompleteAadhaarVerification(c echo.Context) error {
	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(400, map[string]string{"message": "Unauthorized request"})
	}

	if err := h.userService.CompleteAadhaarVerification(c.Request().Context(), userID); err != nil {
		h.logger.Errorf("Error completing Aadhaar verification for user %s: %v", userID, err)

		// Handle specific error cases
		if err.Error() == "failed to get transaction ID: no Aadhaar transaction ID found for user" {
			return c.JSON(400, map[string]interface{}{"message": "No Aadhaar verification process initiated", "data": nil})
		}

		return c.JSON(500, map[string]string{"message": "Failed to complete Aadhaar verification"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Aadhaar verification completed successfully",
		"data":    nil,
	})
}

// VerifyEmail verifies user's email using verification token
func (h *UserHandler) VerifyEmail(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return c.JSON(400, map[string]string{"message": "Verification token is required"})
	}

	if err := h.userService.VerifyEmailWithToken(c.Request().Context(), token); err != nil {
		h.logger.Errorf("Error verifying email with token: %v", err)

		// Handle specific error cases
		switch err.Error() {
		case "verification token not found":
			return c.JSON(404, map[string]string{"message": "Invalid verification token"})
		case "verification token already used":
			return c.JSON(400, map[string]string{"message": "Verification token has already been used"})
		case "verification token has expired":
			return c.JSON(400, map[string]string{"message": "Verification token has expired"})
		default:
			return c.JSON(500, map[string]string{"message": "Failed to verify email"})
		}
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Email verified successfully. You can now log in.",
		"data":    nil,
	})
}

// ResendEmailVerification resends email verification token
func (h *UserHandler) ResendEmailVerification(c echo.Context) error {
	var dto user.ResendVerificationDTO
	if err := c.Bind(&dto); err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request payload"})
	}

	if dto.Email == "" {
		return c.JSON(400, map[string]string{"message": "Email is required"})
	}

	if err := h.userService.ResendEmailVerification(c.Request().Context(), dto.Email); err != nil {
		h.logger.Errorf("Error resending email verification for %s: %v", dto.Email, err)

		// Handle specific error cases
		switch err.Error() {
		case "user not found":
			return c.JSON(404, map[string]string{"message": "User not found"})
		case "email already verified":
			return c.JSON(400, map[string]string{"message": "Email is already verified"})
		default:
			return c.JSON(500, map[string]string{"message": "Failed to resend verification email"})
		}
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Verification email sent successfully. Please check your inbox.",
		"data":    nil,
	})
}

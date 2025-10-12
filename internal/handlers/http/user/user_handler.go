package user

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/middleware"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/types"
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

	// Public routes - don't require authentication
	userGroup.GET("/verify-email", handler.VerifyEmail)
	userGroup.POST("/resend-verification", handler.ResendEmailVerification)
	userGroup.GET("/forgot-password", handler.ForgotPassword)
	userGroup.POST("/reset-password", handler.ResetPassword)

	// Protected routes requiring authentication
	protectedUser := userGroup.Group("")
	protectedUser.Use(middleware.JWTMiddleware(authService))

	// User onboarding endpoints
	protectedUser.POST("/onboard/initiate", handler.InitiateOnboarding)
	protectedUser.POST("/onboard/complete", handler.CompleteOnboarding)
	protectedUser.GET("/onboard/status", handler.GetOnboardingStatus)

	// User profile endpoints
	protectedUser.GET("/profile", handler.GetProfile)
	protectedUser.PUT("/profile", handler.UpdateProfile)
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

// InitiateOnboarding initiates user onboarding process (identity verification only)
func (h *UserHandler) InitiateOnboarding(c echo.Context) error {
	var dto user.InitiateOnboardingDTO
	if err := c.Bind(&dto); err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request payload"})
	}

	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	response, err := h.userService.InitiateOnboarding(
		c.Request().Context(),
		userID,
		dto.Name,
		dto.DOB,
		dto.TaxID,
		dto.NationalID,
	)

	if err != nil {
		h.logger.Errorf("Error initiating onboarding for user %s: %v", userID, err)

		// Handle typed errors
		if userErr, ok := err.(*types.UserError); ok {
			switch userErr.Code {
			case types.ErrCodeKYCAlreadyCompleted:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeInvalidKYCInput:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeVerificationFailed:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeUserNotFound:
				return c.JSON(404, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			}
		}

		return c.JSON(500, map[string]string{"message": "Failed to initiate onboarding"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Onboarding initiated successfully",
		"data":    response,
	})
}

// CompleteOnboarding completes the user onboarding after Aadhaar verification
// User claims role in specific business after identity verification
func (h *UserHandler) CompleteOnboarding(c echo.Context) error {
	// Read BusinessID from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(400, map[string]string{
			"message": "X-Business-ID header is required",
		})
	}

	var dto user.CompleteOnboardingDTO
	if err := c.Bind(&dto); err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request payload"})
	}

	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	err := h.userService.CompleteOnboarding(c.Request().Context(), userID, businessID, dto.ClaimedRole)
	if err != nil {
		h.logger.Errorf("Error completing onboarding for user %s: %v", userID, err)

		// Handle typed errors
		if userErr, ok := err.(*types.UserError); ok {
			switch userErr.Code {
			case types.ErrCodeAadhaarNotInitiated:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeVerificationFailed:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeUserNotFound:
				return c.JSON(404, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeInvalidKYCInput:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			}
		}

		return c.JSON(500, map[string]string{"message": "Failed to complete onboarding"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Onboarding completed successfully",
		"data":    map[string]string{"status": "kyc_completed"},
	})
}

// GetOnboardingStatus returns the current onboarding status
func (h *UserHandler) GetOnboardingStatus(c echo.Context) error {
	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	statusData, err := h.userService.GetOnboardingStatus(c.Request().Context(), userID)
	if err != nil {
		h.logger.Errorf("Error getting onboarding status for user %s: %v", userID, err)

		// Handle typed errors
		if userErr, ok := err.(*types.UserError); ok {
			switch userErr.Code {
			case types.ErrCodeUserNotFound:
				return c.JSON(404, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			}
		}

		return c.JSON(500, map[string]string{"message": "Failed to get onboarding status"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Onboarding status fetched successfully",
		"data":    statusData,
	})
}

// ForgotPassword sends password reset email to user
func (h *UserHandler) ForgotPassword(c echo.Context) error {
	email := c.QueryParam("email")
	if email == "" {
		return c.JSON(400, map[string]string{"message": "Email is required"})
	}

	// Service always returns success to prevent email enumeration
	if err := h.userService.SendPasswordResetEmail(c.Request().Context(), email); err != nil {
		h.logger.Errorf("Error sending password reset email: %v", err)
		// Still return success to prevent information disclosure
	}

	return c.JSON(200, map[string]interface{}{
		"message": "If the email is registered, a password reset code has been sent. Please check your inbox.",
		"data":    nil,
	})
}

// ResetPassword resets user password using token and new password
func (h *UserHandler) ResetPassword(c echo.Context) error {
	var dto user.ResetPasswordDTO
	if err := c.Bind(&dto); err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request payload"})
	}

	if dto.Token == "" || dto.NewPassword == "" {
		return c.JSON(400, map[string]string{"message": "Token and new password are required"})
	}

	// Hash the new password before passing to service
	hashedPassword, err := h.authService.HashPassword(dto.NewPassword)
	if err != nil {
		h.logger.Errorf("Error hashing password: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to process password"})
	}

	// Reset password with token
	if err := h.userService.ResetPasswordWithToken(c.Request().Context(), dto.Token, hashedPassword); err != nil {
		h.logger.Errorf("Error resetting password: %v", err)

		// Handle specific error cases
		switch err.Error() {
		case "reset token not found", "invalid or expired reset token":
			return c.JSON(404, map[string]string{"message": "Invalid or expired reset token"})
		case "reset token already used":
			return c.JSON(400, map[string]string{"message": "Reset token has already been used"})
		case "reset token has expired":
			return c.JSON(400, map[string]string{"message": "Reset token has expired"})
		default:
			return c.JSON(500, map[string]string{"message": "Failed to reset password"})
		}
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Password reset successfully. You can now log in with your new password.",
		"data":    nil,
	})
}

// GetProfile retrieves the user's profile information with masked national ID
func (h *UserHandler) GetProfile(c echo.Context) error {
	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	profile, err := h.userService.GetUserProfile(c.Request().Context(), userID)
	if err != nil {
		h.logger.Errorf("Error getting profile for user %s: %v", userID, err)
		return c.JSON(500, map[string]string{"message": "Failed to get user profile"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Profile retrieved successfully",
		"data":    profile,
	})
}

// UpdateProfile updates the user's profile information (mobile only)
func (h *UserHandler) UpdateProfile(c echo.Context) error {
	var dto user.UpdateProfileDTO
	if err := c.Bind(&dto); err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request payload"})
	}

	if dto.Mobile == "" {
		return c.JSON(400, map[string]string{"message": "Mobile number is required"})
	}

	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	err := h.userService.UpdateUserProfile(c.Request().Context(), userID, dto.Mobile)
	if err != nil {
		h.logger.Errorf("Error updating profile for user %s: %v", userID, err)
		return c.JSON(500, map[string]string{"message": "Failed to update user profile"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Profile updated successfully",
		"data":    nil,
	})
}

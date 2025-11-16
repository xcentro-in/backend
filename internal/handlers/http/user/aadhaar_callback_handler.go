package user

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/templates"
	"xcentro.in/internal/shared/utils"
)

type AadhaarCallbackHandler struct {
	userService user.UserService
	frontendURL string
}

func NewAadhaarCallbackHandler(userService user.UserService, cfg *config.Config) *AadhaarCallbackHandler {
	return &AadhaarCallbackHandler{
		userService: userService,
		frontendURL: cfg.Frontend.URL,
	}
}

// CallbackRequest represents the encrypted callback request from AuthBridge
type CallbackRequest struct {
	ResponseData string `json:"responseData"`
}

// HandleRedirectCallback handles the browser redirect callback (GET) with JWT authentication
// Returns HTML response for user feedback
func (h *AadhaarCallbackHandler) HandleRedirectCallback(c echo.Context) error {
	log := logger.GetDefault()

	// Generate a unique, random nonce for CSP
	nonce, err := utils.GenerateNonce()
	if err != nil {
		log.Errorf("Failed to generate CSP nonce: %v", err)
		return c.String(http.StatusInternalServerError, "Internal server error")
	}

	// Set CSP header with nonce - overrides the global middleware policy
	csp := utils.GenerateCSPHeader(nonce)
	c.Response().Header().Set("Content-Security-Policy", csp)

	// Log all query parameters
	log.Info("Aadhaar callback received")
	log.Info("Query parameters:")
	for key, values := range c.QueryParams() {
		for _, value := range values {
			log.Infof("  %s: %s", key, value)
		}
	}

	// Return success HTML with nonce
	html := templates.GenerateResponsePage(templates.ResponsePageConfig{
		PageType:      templates.PageTypeSuccess,
		Title:         "Aadhaar Verified Successfully!",
		Message:       "Thank you for completing your Aadhaar verification.",
		SubMessage:    "✓ Your identity has been verified successfully",
		AutoCloseSec:  2,
		ShowCloseHint: true,
	}, nonce)
	return c.HTML(http.StatusOK, html)
}

// HandleWebhook handles the webhook callback (POST) without authentication
// Returns 200 status code on success
func (h *AadhaarCallbackHandler) HandleWebhook(c echo.Context) error {
	log := logger.GetDefault()
	ctx := c.Request().Context()

	// Generate a unique, random nonce for CSP
	nonce, err := utils.GenerateNonce()
	if err != nil {
		log.Errorf("Failed to generate CSP nonce: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
	}

	// Set CSP header with nonce - overrides the global middleware policy
	csp := utils.GenerateCSPHeader(nonce)
	c.Response().Header().Set("Content-Security-Policy", csp)

	// Parse encrypted request
	var req CallbackRequest
	if err := c.Bind(&req); err != nil {
		log.Errorf("Failed to parse webhook request: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request format"})
	}

	// Validate input
	if req.ResponseData == "" {
		log.Error("Empty response data in webhook request")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing response data"})
	}

	// Call service method to process the webhook
	// Service always returns nil for webhooks (errors are logged internally)
	_ = h.userService.ProcessAadhaarWebhook(ctx, req.ResponseData)

	// Always return success to acknowledge receipt
	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}


package user

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/providers/authbridge"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/crypto"
	"xcentro.in/internal/shared/logger"
)

type AadhaarCallbackHandler struct {
	userRepo           user.UserRepository
	authBridgePassword string
}

func NewAadhaarCallbackHandler(userRepo user.UserRepository, authBridgePassword string) *AadhaarCallbackHandler {
	return &AadhaarCallbackHandler{
		userRepo:           userRepo,
		authBridgePassword: authBridgePassword,
	}
}

// CallbackRequest represents the encrypted callback request from AuthBridge
type CallbackRequest struct {
	ResponseData string `json:"responseData"`
}

// HandleRedirectCallback handles the browser redirect callback (POST) with JWT authentication
// Returns HTML response for user feedback
func (h *AadhaarCallbackHandler) HandleRedirectCallback(c echo.Context) error {
	log := logger.GetDefault()
	ctx := c.Request().Context()

	// Get user ID from JWT context (set by middleware)
	userID, ok := c.Get("user_id").(string)
	if !ok || userID == "" {
		log.Error("User ID not found in JWT context")
		return c.HTML(http.StatusUnauthorized, generateErrorHTML("Unauthorized", "Please log in to continue."))
	}

	// Parse encrypted request
	var req CallbackRequest
	if err := c.Bind(&req); err != nil {
		log.Errorf("Failed to parse callback request: %v", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Invalid Request", "Unable to process verification data."))
	}

	// Decrypt response
	decryptedResponse, err := crypto.DecryptAES256CBC(req.ResponseData, h.authBridgePassword)
	if err != nil {
		log.Errorf("Failed to decrypt callback response: %v", err)
		return c.HTML(http.StatusInternalServerError, generateErrorHTML("Verification Failed", "Unable to decrypt verification data."))
	}

	// Parse decrypted response
	var responseData map[string]interface{}
	if err := json.Unmarshal([]byte(decryptedResponse), &responseData); err != nil {
		log.Errorf("Failed to parse decrypted response: %v", err)
		return c.HTML(http.StatusInternalServerError, generateErrorHTML("Verification Failed", "Invalid verification response format."))
	}

	// Check status first (must be 1 for success)
	status, ok := responseData["status"].(float64)
	if !ok || status != 1 {
		errorMsg := "Verification failed"
		if msg, exists := responseData["message"].(string); exists {
			errorMsg = msg
		}
		log.Errorf("Aadhaar verification failed for user %s: status=%v, message=%s", userID, status, errorMsg)
		return c.HTML(http.StatusOK, generateErrorHTML("Verification Failed", errorMsg))
	}

	// Extract ts_trans_id for user lookup
	tsTransID, ok := responseData["ts_trans_id"].(string)
	if !ok || tsTransID == "" {
		log.Errorf("Missing ts_trans_id in callback response for user %s", userID)
		return c.HTML(http.StatusInternalServerError, generateErrorHTML("Verification Failed", "Missing transaction ID in response."))
	}

	// Get user details to check current KYC status
	userObj, err := h.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		log.Errorf("Failed to get user details for user %s: %v", userID, err)
		return c.HTML(http.StatusInternalServerError, generateErrorHTML("Verification Failed", "Unable to verify user identity."))
	}

	// Check if KYC is already completed or verified
	if userObj.KycStatus == shared.KYCCompleted || userObj.KycStatus == shared.KYCVerified {
		log.Infof("KYC already completed for user %s, current status: %s", userID, userObj.KycStatus)
		return c.HTML(http.StatusOK, generateAlreadyCompletedHTML())
	}

	// Verify that the ts_trans_id matches the user's stored transaction ID
	if err := h.verifyTransactionID(ctx, userObj.TaxID, tsTransID); err != nil {
		log.Errorf("Transaction ID mismatch for user %s: %v", userID, err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML("Verification Failed", "Invalid transaction reference."))
	}

	// Store the full decrypted response as JSON
	responseJSON, err := json.Marshal(responseData)
	if err != nil {
		log.Errorf("Failed to marshal response data for user %s: %v", userID, err)
		return c.HTML(http.StatusInternalServerError, generateErrorHTML("Verification Failed", "Unable to process verification data."))
	}

	// Update user identity with Aadhaar verification data
	if err := h.userRepo.UpdateNationalIDData(ctx, userObj.TaxID, string(responseJSON)); err != nil {
		log.Errorf("Failed to update national ID data for user %s: %v", userID, err)
		return c.HTML(http.StatusInternalServerError, generateErrorHTML("Verification Failed", "Unable to save verification data."))
	}

	// Update KYC status to verified
	if err := h.userRepo.UpdateUserKYCStatus(ctx, userID, shared.KYCVerified); err != nil {
		log.Errorf("Failed to update KYC status for user %s: %v", userID, err)
		return c.HTML(http.StatusInternalServerError, generateErrorHTML("Verification Failed", "Unable to update verification status."))
	}

	log.Infof("Aadhaar verification completed successfully for user %s", userID)

	// Return success HTML that closes the popup
	return c.HTML(http.StatusOK, generateSuccessHTML())
}

// HandleWebhook handles the webhook callback (POST) without authentication
// Returns 200 status code on success
func (h *AadhaarCallbackHandler) HandleWebhook(c echo.Context) error {
	log := logger.GetDefault()
	ctx := c.Request().Context()

	// Parse encrypted request
	var req CallbackRequest
	if err := c.Bind(&req); err != nil {
		log.Errorf("Failed to parse webhook request: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request format"})
	}

	// Decrypt response
	decryptedResponse, err := crypto.DecryptAES256CBC(req.ResponseData, h.authBridgePassword)
	if err != nil {
		log.Errorf("Failed to decrypt webhook response: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Decryption failed"})
	}

	// Parse decrypted response
	var responseData map[string]interface{}
	if err := json.Unmarshal([]byte(decryptedResponse), &responseData); err != nil {
		log.Errorf("Failed to parse decrypted webhook response: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Invalid response format"})
	}

	// Check status first (must be 1 for success)
	status, ok := responseData["status"].(float64)
	if !ok || status != 1 {
		errorMsg := "Verification failed"
		if msg, exists := responseData["message"].(string); exists {
			errorMsg = msg
		}
		log.Errorf("Aadhaar verification failed in webhook: status=%v, message=%s", status, errorMsg)
		// Return 200 even on failure to prevent retries
		return c.JSON(http.StatusOK, map[string]string{"status": "acknowledged", "message": "Verification failed"})
	}

	// Extract ts_trans_id for user lookup
	tsTransID, ok := responseData["ts_trans_id"].(string)
	if !ok || tsTransID == "" {
		log.Errorf("Missing ts_trans_id in webhook response")
		return c.JSON(http.StatusOK, map[string]string{"status": "acknowledged", "message": "Missing transaction ID"})
	}

	// Find user by transaction ID
	identity, err := h.userRepo.GetUserIdentityByTxnID(ctx, tsTransID)
	if err != nil {
		log.Errorf("Failed to find user identity by txn_id %s: %v", tsTransID, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "acknowledged", "message": "User not found"})
	}

	// Get user details to check current KYC status
	userObj, err := h.userRepo.GetUserByTaxID(ctx, identity.TaxID)
	if err != nil {
		log.Errorf("Failed to get user by tax_id %s: %v", identity.TaxID, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "acknowledged", "message": "User not found"})
	}

	// Check if KYC is already completed or verified (idempotent)
	if userObj.KycStatus == shared.KYCCompleted || userObj.KycStatus == shared.KYCVerified {
		log.Infof("KYC already completed for user %s, current status: %s", userObj.Id, userObj.KycStatus)
		return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "Already verified"})
	}

	// Store the full decrypted response as JSON
	responseJSON, err := json.Marshal(responseData)
	if err != nil {
		log.Errorf("Failed to marshal webhook response data: %v", err)
		return c.JSON(http.StatusOK, map[string]string{"status": "acknowledged", "message": "Marshaling failed"})
	}

	// Update user identity with Aadhaar verification data
	if err := h.userRepo.UpdateNationalIDData(ctx, identity.TaxID, string(responseJSON)); err != nil {
		log.Errorf("Failed to update national ID data in webhook: %v", err)
		return c.JSON(http.StatusOK, map[string]string{"status": "acknowledged", "message": "Database update failed"})
	}

	// Update KYC status to verified
	if err := h.userRepo.UpdateUserKYCStatus(ctx, userObj.Id, shared.KYCVerified); err != nil {
		log.Errorf("Failed to update KYC status in webhook: %v", err)
		return c.JSON(http.StatusOK, map[string]string{"status": "acknowledged", "message": "Status update failed"})
	}

	log.Infof("Aadhaar verification completed successfully via webhook for user %s", userObj.Id)

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// verifyTransactionID checks if the provided transaction ID matches the stored one
func (h *AadhaarCallbackHandler) verifyTransactionID(ctx context.Context, taxID, tsTransID string) error {
	identity, err := h.userRepo.GetUserIdentityByTaxID(ctx, taxID)
	if err != nil {
		return fmt.Errorf("failed to get user identity: %w", err)
	}

	if identity.NTxnID != tsTransID {
		return fmt.Errorf("transaction ID mismatch: expected %s, got %s", identity.NTxnID, tsTransID)
	}

	return nil
}

// HTML templates for responses

func generateSuccessHTML() string {
	return `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Verification Successful - xcentro</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #34a853 0%, #1a73e8 100%);
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 16px;
            padding: 48px 32px;
            max-width: 480px;
            width: 100%;
            text-align: center;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
        }
        .icon {
            width: 80px;
            height: 80px;
            margin: 0 auto 24px;
            background: #34a853;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            animation: scaleIn 0.5s ease-out;
        }
        .icon svg {
            width: 48px;
            height: 48px;
            stroke: white;
            stroke-width: 3;
            fill: none;
        }
        h1 {
            color: #202124;
            font-size: 28px;
            font-weight: 600;
            margin-bottom: 12px;
        }
        p {
            color: #5f6368;
            font-size: 16px;
            line-height: 1.6;
            margin-bottom: 24px;
        }
        .message {
            background: #e8f5e9;
            border-left: 4px solid #34a853;
            padding: 16px;
            border-radius: 8px;
            margin-bottom: 24px;
            text-align: left;
        }
        .message p {
            color: #1e4620;
            margin: 0;
            font-size: 14px;
        }
        .footer {
            color: #80868b;
            font-size: 14px;
            margin-top: 32px;
        }
        @keyframes scaleIn {
            from {
                transform: scale(0);
                opacity: 0;
            }
            to {
                transform: scale(1);
                opacity: 1;
            }
        }
    </style>
    <script>
        // Close popup window after 3 seconds
        setTimeout(function() {
            window.close();
        }, 3000);
    </script>
</head>
<body>
    <div class="container">
        <div class="icon">
            <svg viewBox="0 0 24 24">
                <polyline points="20 6 9 17 4 12"></polyline>
            </svg>
        </div>
        <h1>Aadhaar Verified Successfully!</h1>
        <p>Thank you for completing your Aadhaar verification.</p>
        <div class="message">
            <p><strong>✓</strong> Your identity has been verified successfully</p>
        </div>
        <p>This window will close automatically in a few seconds...</p>
        <div class="footer">
            <p><strong>xcentro</strong><br>
            MerchantBridge Fintech Solutions Pvt Ltd</p>
        </div>
    </div>
</body>
</html>
`
}

func generateAlreadyCompletedHTML() string {
	return `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>KYC Already Completed - xcentro</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #1a73e8 0%, #34a853 100%);
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 16px;
            padding: 48px 32px;
            max-width: 480px;
            width: 100%;
            text-align: center;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
        }
        .icon {
            width: 80px;
            height: 80px;
            margin: 0 auto 24px;
            background: #1a73e8;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .icon svg {
            width: 48px;
            height: 48px;
            stroke: white;
            stroke-width: 3;
            fill: none;
        }
        h1 {
            color: #202124;
            font-size: 28px;
            font-weight: 600;
            margin-bottom: 12px;
        }
        p {
            color: #5f6368;
            font-size: 16px;
            line-height: 1.6;
            margin-bottom: 24px;
        }
        .message {
            background: #e3f2fd;
            border-left: 4px solid #1a73e8;
            padding: 16px;
            border-radius: 8px;
            margin-bottom: 24px;
            text-align: left;
        }
        .message p {
            color: #1565c0;
            margin: 0;
            font-size: 14px;
        }
        .footer {
            color: #80868b;
            font-size: 14px;
            margin-top: 32px;
        }
    </style>
    <script>
        setTimeout(function() {
            window.close();
        }, 3000);
    </script>
</head>
<body>
    <div class="container">
        <div class="icon">
            <svg viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="10"></circle>
                <line x1="12" y1="8" x2="12" y2="12"></line>
                <line x1="12" y1="16" x2="12.01" y2="16"></line>
            </svg>
        </div>
        <h1>KYC Already Completed</h1>
        <p>Your KYC verification has already been completed.</p>
        <div class="message">
            <p><strong>ℹ</strong> No further action is required</p>
        </div>
        <p>This window will close automatically in a few seconds...</p>
        <div class="footer">
            <p><strong>xcentro</strong><br>
            MerchantBridge Fintech Solutions Pvt Ltd</p>
        </div>
    </div>
</body>
</html>
`
}

func generateErrorHTML(title, message string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s - xcentro</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #ea4335 0%, #fbbc04 100%);
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 16px;
            padding: 48px 32px;
            max-width: 480px;
            width: 100%;
            text-align: center;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
        }
        .icon {
            width: 80px;
            height: 80px;
            margin: 0 auto 24px;
            background: #ea4335;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .icon svg {
            width: 48px;
            height: 48px;
            stroke: white;
            stroke-width: 3;
            fill: none;
        }
        h1 {
            color: #202124;
            font-size: 28px;
            font-weight: 600;
            margin-bottom: 12px;
        }
        p {
            color: #5f6368;
            font-size: 16px;
            line-height: 1.6;
            margin-bottom: 24px;
        }
        .message {
            background: #fce4ec;
            border-left: 4px solid #ea4335;
            padding: 16px;
            border-radius: 8px;
            margin-bottom: 24px;
            text-align: left;
        }
        .message p {
            color: #c62828;
            margin: 0;
            font-size: 14px;
        }
        .footer {
            color: #80868b;
            font-size: 14px;
            margin-top: 32px;
        }
    </style>
    <script>
        setTimeout(function() {
            window.close();
        }, 5000);
    </script>
</head>
<body>
    <div class="container">
        <div class="icon">
            <svg viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="10"></circle>
                <line x1="15" y1="9" x2="9" y2="15"></line>
                <line x1="9" y1="9" x2="15" y2="15"></line>
            </svg>
        </div>
        <h1>%s</h1>
        <div class="message">
            <p>%s</p>
        </div>
        <p>Please try again or contact support if the issue persists.</p>
        <p>This window will close automatically in a few seconds...</p>
        <div class="footer">
            <p><strong>xcentro</strong><br>
            MerchantBridge Fintech Solutions Pvt Ltd</p>
        </div>
    </div>
</body>
</html>
`, title, title, message)
}

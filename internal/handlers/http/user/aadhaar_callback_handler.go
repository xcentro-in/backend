package user

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
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

// HandleRedirectCallback handles the browser redirect callback (POST) with JWT authentication
// Returns HTML response for user feedback
func (h *AadhaarCallbackHandler) HandleRedirectCallback(c echo.Context) error {
	log := logger.GetDefault()
	ctx := c.Request().Context()

	// Get user ID from JWT context (set by middleware)
	userID, ok := c.Get("user_id").(string)
	if !ok || userID == "" {
		log.Error("User ID not found in JWT context")
		return c.HTML(http.StatusUnauthorized, generateErrorHTML(h.frontendURL, "Unauthorized", "Please log in to continue."))
	}

	// Parse encrypted request
	var req CallbackRequest
	if err := c.Bind(&req); err != nil {
		log.Errorf("Failed to parse callback request: %v", err)
		return c.HTML(http.StatusBadRequest, generateErrorHTML(h.frontendURL, "Invalid Request", "Unable to process verification data."))
	}

	// Validate input
	if req.ResponseData == "" {
		log.Error("Empty response data in callback request")
		return c.HTML(http.StatusBadRequest, generateErrorHTML(h.frontendURL, "Invalid Request", "Missing verification data."))
	}

	// Call service method to process the callback
	err := h.userService.ProcessAadhaarRedirectCallback(ctx, userID, req.ResponseData)
	if err != nil {
		// Check for specific error types
		if strings.Contains(err.Error(), "kyc_already_completed") {
			return c.HTML(http.StatusOK, generateAlreadyCompletedHTML(h.frontendURL))
		}

		// Generic error handling
		errorMsg := "Verification failed. Please try again."
		if strings.Contains(err.Error(), "aadhaar verification failed") {
			errorMsg = err.Error()
		}
		return c.HTML(http.StatusOK, generateErrorHTML(h.frontendURL, "Verification Failed", errorMsg))
	}

	// Return success HTML that closes the iframe via postMessage
	return c.HTML(http.StatusOK, generateSuccessHTML(h.frontendURL))
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

// HTML templates for responses

func generateSuccessHTML(frontendURL string) string {
	return fmt.Sprintf(`
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
            background: linear-gradient(135deg, #34a853 0%%, #1a73e8 100%%);
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
            width: 100%%;
            text-align: center;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
        }
        .icon {
            width: 80px;
            height: 80px;
            margin: 0 auto 24px;
            background: #34a853;
            border-radius: 50%%;
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
        (function() {
            // Configuration
            const config = {
                frontendOrigin: '%s',
                message: {
                    type: 'KYC_COMPLETE',
                    status: 'success',
                    tx_id: ''
                }
            };

            // Send postMessage to parent window (iframe case)
            function notifyParent() {
                try {
                    if (window.parent && window.parent !== window) {
                        console.log('Sending KYC completion message to parent:', config.message);
                        window.parent.postMessage(config.message, config.frontendOrigin);
                        return true;
                    }
                    console.warn('No parent window found');
                    return false;
                } catch (error) {
                    console.error('Error sending postMessage:', error);
                    return false;
                }
            }

            // Execute on load
            window.addEventListener('load', function() {
                notifyParent();
            });

            // Also try immediately in case load event already fired
            if (document.readyState === 'complete') {
                notifyParent();
            }
        })();
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
        <p>Returning to application...</p>
        <div class="footer">
            <p><strong>xcentro</strong><br>
            MerchantBridge Fintech Solutions Pvt Ltd</p>
        </div>
    </div>
</body>
</html>
`, frontendURL)
}

func generateAlreadyCompletedHTML(frontendURL string) string {
	return fmt.Sprintf(`
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
            background: linear-gradient(135deg, #1a73e8 0%%, #34a853 100%%);
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
            width: 100%%;
            text-align: center;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
        }
        .icon {
            width: 80px;
            height: 80px;
            margin: 0 auto 24px;
            background: #1a73e8;
            border-radius: 50%%;
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
        (function() {
            // Configuration
            const config = {
                frontendOrigin: '%s',
                message: {
                    type: 'KYC_COMPLETE',
                    status: 'success',
                    tx_id: ''
                }
            };

            // Send postMessage to parent window (iframe case)
            function notifyParent() {
                try {
                    if (window.parent && window.parent !== window) {
                        console.log('Sending KYC completion message to parent:', config.message);
                        window.parent.postMessage(config.message, config.frontendOrigin);
                        return true;
                    }
                    console.warn('No parent window found');
                    return false;
                } catch (error) {
                    console.error('Error sending postMessage:', error);
                    return false;
                }
            }

            // Execute on load
            window.addEventListener('load', function() {
                notifyParent();
            });

            // Also try immediately in case load event already fired
            if (document.readyState === 'complete') {
                notifyParent();
            }
        })();
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
        <p>Returning to application...</p>
        <div class="footer">
            <p><strong>xcentro</strong><br>
            MerchantBridge Fintech Solutions Pvt Ltd</p>
        </div>
    </div>
</body>
</html>
`, frontendURL)
}

func generateErrorHTML(frontendURL, title, message string) string {
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
            background: linear-gradient(135deg, #ea4335 0%%, #fbbc04 100%%);
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
            width: 100%%;
            text-align: center;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
        }
        .icon {
            width: 80px;
            height: 80px;
            margin: 0 auto 24px;
            background: #ea4335;
            border-radius: 50%%;
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
        (function() {
            // Configuration
            const config = {
                frontendOrigin: '%s',
                message: {
                    type: 'KYC_COMPLETE',
                    status: 'failed',
                    tx_id: ''
                }
            };

            // Send postMessage to parent window (iframe case)
            function notifyParent() {
                try {
                    if (window.parent && window.parent !== window) {
                        console.log('Sending KYC completion message to parent:', config.message);
                        window.parent.postMessage(config.message, config.frontendOrigin);
                        return true;
                    }
                    console.warn('No parent window found');
                    return false;
                } catch (error) {
                    console.error('Error sending postMessage:', error);
                    return false;
                }
            }

            // Execute on load
            window.addEventListener('load', function() {
                notifyParent();
            });

            // Also try immediately in case load event already fired
            if (document.readyState === 'complete') {
                notifyParent();
            }
        })();
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
        <p>Returning to application...</p>
        <div class="footer">
            <p><strong>xcentro</strong><br>
            MerchantBridge Fintech Solutions Pvt Ltd</p>
        </div>
    </div>
</body>
</html>
`, title, frontendURL, title, message)
}

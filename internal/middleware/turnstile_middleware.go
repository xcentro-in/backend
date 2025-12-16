package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

const (
	// Cloudflare Turnstile siteverify endpoint
	turnstileSiteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

	// Default header name for Turnstile token
	defaultTurnstileHeader = "CF-Turnstile-Token"
)

// TurnstileVerifyRequest represents the request to Cloudflare's siteverify endpoint
type TurnstileVerifyRequest struct {
	Secret   string `json:"secret"`
	Response string `json:"response"`
	RemoteIP string `json:"remoteip,omitempty"`
}

// TurnstileVerifyResponse represents the response from Cloudflare's siteverify endpoint
type TurnstileVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	ErrorCodes  []string `json:"error-codes,omitempty"`
	Action      string   `json:"action,omitempty"`
	CData       string   `json:"cdata,omitempty"`
}

// TurnstileMiddleware creates a middleware that validates Cloudflare Turnstile tokens
// Loads configuration from the global config singleton
// If Enabled is false or SecretKey is empty, the middleware allows all requests (for local dev)
func TurnstileMiddleware() echo.MiddlewareFunc {
	log := logger.GetDefault()

	// Load config from singleton
	cfg := config.Get()
	secretKey := cfg.Turnstile.SecretKey
	enabled := cfg.Turnstile.Enabled
	log.InfoWith("Turnstile config loaded", "secret_key_set", secretKey != "", "enabled", enabled)
	// Use default header name
	headerName := defaultTurnstileHeader

	// If Turnstile is disabled or secret key is empty, return a pass-through middleware
	if !enabled || secretKey == "" {
		log.Warn("Turnstile middleware is DISABLED - all requests will be allowed (local development mode)")
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				// Pass through without verification
				return next(c)
			}
		}
	}

	log.Info("Turnstile middleware is ENABLED - bot protection active")

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract Turnstile token from header
			token := c.Request().Header.Get(headerName)
			if token == "" {
				log.WarnWith("Turnstile token missing from request",
					"path", c.Request().URL.Path,
					"method", c.Request().Method,
					"ip", c.RealIP(),
				)
				return c.JSON(http.StatusBadRequest, map[string]any{
					"error":   "TURNSTILE_TOKEN_MISSING",
					"message": "Turnstile verification token is required",
				})
			}

			// Get client IP
			clientIP := c.RealIP()

			// Verify token with Cloudflare
			verified, errorCodes, err := verifyTurnstileToken(secretKey, token, clientIP)
			if err != nil {
				log.ErrorWith("Turnstile verification failed: %v", err,
					"path", c.Request().URL.Path,
					"method", c.Request().Method,
					"ip", clientIP,
					"error_codes", errorCodes,
				)
				return c.JSON(http.StatusInternalServerError, map[string]any{
					"error":   "TURNSTILE_VERIFICATION_FAILED",
					"message": "Failed to verify Turnstile token",
				})
			}

			if !verified {
				log.WarnWith("Turnstile verification rejected",
					"path", c.Request().URL.Path,
					"method", c.Request().Method,
					"ip", clientIP,
					"error_codes", errorCodes,
				)
				return c.JSON(http.StatusForbidden, map[string]any{
					"error":   "TURNSTILE_VERIFICATION_REJECTED",
					"message": "Turnstile verification failed - bot detection triggered",
					"details": errorCodes,
				})
			}

			// Token verified successfully - continue to next handler
			log.DebugWith("Turnstile verification successful",
				"path", c.Request().URL.Path,
				"method", c.Request().Method,
				"ip", clientIP,
			)
			return next(c)
		}
	}
}

// verifyTurnstileToken sends a verification request to Cloudflare's siteverify endpoint
func verifyTurnstileToken(secretKey, token, clientIP string) (verified bool, errorCodes []string, err error) {
	// Prepare request body
	reqBody := TurnstileVerifyRequest{
		Secret:   secretKey,
		Response: token,
		RemoteIP: clientIP,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return false, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Send POST request to Cloudflare
	resp, err := client.Post(turnstileSiteverifyURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return false, nil, fmt.Errorf("failed to send verification request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return false, nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var verifyResp TurnstileVerifyResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		return false, nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return verifyResp.Success, verifyResp.ErrorCodes, nil
}

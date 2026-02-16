package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// Fast2SMS Template IDs
const (
	TemplateSignupOTP       = "204688" // Signup verification OTP
	TemplatePasswordResetOTP = "204686" // Password reset OTP
	TemplatePaymentSuccess   = "204687" // Payment success notification (future use)
)

// Fast2SMSClient implements SMSService using Fast2SMS API
type Fast2SMSClient struct {
	apiKey      string
	senderID    string
	endpoint    string
	httpClient  *http.Client
}

// Fast2SMSRequest represents the API request payload
type Fast2SMSRequest struct {
	Route           string `json:"route"`
	SenderID        string `json:"sender_id"`
	Message         string `json:"message"`          // Template ID
	VariablesValues string `json:"variables_values"` // Pipe-separated values
	Flash           int    `json:"flash"`            // 0 = normal, 1 = flash
	Numbers         string `json:"numbers"`          // Comma-separated phone numbers
}

// Fast2SMSResponse represents the API response
type Fast2SMSResponse struct {
	Return    bool     `json:"return"`
	RequestID string   `json:"request_id"`
	Message   []string `json:"message"`
}

// NewFast2SMSClient creates a new Fast2SMS client from configuration
func NewFast2SMSClient(cfg config.SMSConfig) (*Fast2SMSClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("Fast2SMS API key is required")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://www.fast2sms.com/dev/bulkV2"
	}

	senderID := cfg.SenderID
	if senderID == "" {
		senderID = "XCNTRO"
	}

	return &Fast2SMSClient{
		apiKey:   cfg.APIKey,
		senderID: senderID,
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// SendSMS sends a single SMS message using Fast2SMS
func (c *Fast2SMSClient) SendSMS(ctx context.Context, msg *SMSMessage) (string, error) {
	if err := msg.Validate(); err != nil {
		return "", err
	}

	// Clean phone number (remove +91 prefix if present)
	phoneNumber := strings.TrimPrefix(msg.To, "+91")
	phoneNumber = strings.TrimPrefix(phoneNumber, "91")

	// For now, send OTPs using the signup template (204688)
	// The Body should contain the OTP value
	templateID := TemplateSignupOTP
	variableValues := msg.Body // The OTP value

	return c.sendBulkSMS(ctx, []string{phoneNumber}, templateID, variableValues)
}

// SendBulkSMS sends SMS to multiple recipients
func (c *Fast2SMSClient) SendBulkSMS(ctx context.Context, phoneNumbers []string, templateID string, variableValues string) (string, error) {
	if len(phoneNumbers) == 0 {
		return "", ErrRecipientRequired
	}

	// Clean phone numbers (remove +91 prefix if present)
	cleanedNumbers := make([]string, len(phoneNumbers))
	for i, num := range phoneNumbers {
		cleanedNumbers[i] = strings.TrimPrefix(strings.TrimPrefix(num, "+91"), "91")
	}

	return c.sendBulkSMS(ctx, cleanedNumbers, templateID, variableValues)
}

// sendBulkSMS is the internal method that makes the actual API call
func (c *Fast2SMSClient) sendBulkSMS(ctx context.Context, phoneNumbers []string, templateID string, variableValues string) (string, error) {
	// Build request payload
	payload := Fast2SMSRequest{
		Route:           "dlt",
		SenderID:        c.senderID,
		Message:         templateID,
		VariablesValues: variableValues,
		Flash:           0,
		Numbers:         strings.Join(phoneNumbers, ","),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf("Failed to marshal Fast2SMS request: %v", err)
		return "", fmt.Errorf("failed to prepare SMS request")
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Errorf("Failed to create Fast2SMS request: %v", err)
		return "", fmt.Errorf("failed to create SMS request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	logger.Infof("Sending SMS via Fast2SMS to %d recipient(s)", len(phoneNumbers))

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Errorf("Fast2SMS request failed: %v", err)
		return "", fmt.Errorf("failed to send SMS: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Errorf("Failed to read Fast2SMS response: %v", err)
		return "", fmt.Errorf("failed to read SMS response")
	}

	// Parse response
	var fast2smsResp Fast2SMSResponse
	if err := json.Unmarshal(body, &fast2smsResp); err != nil {
		logger.Errorf("Failed to parse Fast2SMS response: %v. Body: %s", err, string(body))
		return "", fmt.Errorf("failed to parse SMS response")
	}

	// Check if request was successful
	if !fast2smsResp.Return {
		logger.Errorf("Fast2SMS API returned failure. Response: %+v", fast2smsResp)
		return "", fmt.Errorf("SMS sending failed: %v", fast2smsResp.Message)
	}

	logger.Infof("SMS sent successfully via Fast2SMS. RequestID: %s", fast2smsResp.RequestID)
	return fast2smsResp.RequestID, nil
}

// retryWithExponentialBackoff retries an operation with exponential backoff
// Matches the email service retry pattern for consistency
func retryWithExponentialBackoff(ctx context.Context, maxRetries int, baseDelay time.Duration, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: baseDelay * 2^(attempt-1)
			backoffDuration := baseDelay * time.Duration(1<<(attempt-1))

			// Check context cancellation during backoff
			select {
			case <-time.After(backoffDuration):
				// Continue with retry
			case <-ctx.Done():
				return fmt.Errorf("operation cancelled: %w", ctx.Err())
			}

			logger.Infof("Retrying SMS send (attempt %d/%d) after %v", attempt+1, maxRetries, backoffDuration)
		}

		lastErr = operation()

		if lastErr == nil {
			return nil // Success
		}

		// Log error but continue to retry
		logger.Warnf("SMS send attempt %d/%d failed: %v", attempt+1, maxRetries, lastErr)
	}

	return fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
}

// SendOTPSMS is a helper method to send OTP SMS with proper template selection
// Includes built-in retry logic with exponential backoff (matches email service pattern)
func (c *Fast2SMSClient) SendOTPSMS(ctx context.Context, phoneNumber string, otp string, otpType string) (string, error) {
	// Select template based on OTP type
	var templateID string
	switch otpType {
	case "signup", "mobile_verification":
		templateID = TemplateSignupOTP // 204688
	case "password_reset", "change_password":
		templateID = TemplatePasswordResetOTP // 204686
	default:
		templateID = TemplateSignupOTP // Default to signup template
	}

	// Clean phone number
	cleanNumber := strings.TrimPrefix(strings.TrimPrefix(phoneNumber, "+91"), "91")

	// Send with retry logic (3 attempts with exponential backoff: 100ms, 200ms, 400ms)
	var messageID string
	err := retryWithExponentialBackoff(ctx, 3, 100*time.Millisecond, func() error {
		var sendErr error
		messageID, sendErr = c.sendBulkSMS(ctx, []string{cleanNumber}, templateID, otp)
		return sendErr
	})

	return messageID, err
}

// SendPaymentNotification sends payment success notification (future use)
func (c *Fast2SMSClient) SendPaymentNotification(ctx context.Context, phoneNumber string, amount string, payeeName string) (string, error) {
	// Template 204687: Payment of INR {var1} transferred to {var2}
	// Variable format: "amount|payeeName"
	variableValues := fmt.Sprintf("%s|%s", amount, payeeName)

	cleanNumber := strings.TrimPrefix(strings.TrimPrefix(phoneNumber, "+91"), "91")

	return c.sendBulkSMS(ctx, []string{cleanNumber}, TemplatePaymentSuccess, variableValues)
}

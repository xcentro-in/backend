package mailers

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// EmailMessage represents an email to be sent
type EmailMessage struct {
	To         []string          // Required: recipient email addresses
	Cc         []string          // Optional: CC recipients
	Bcc        []string          // Optional: BCC recipients
	From       string            // Required: sender email (must be verified identity or use domain)
	FromName   string            // Optional: sender display name
	ReplyTo    []string          // Optional: reply-to addresses
	Subject    string            // Required: email subject
	TextBody   string            // Optional: plain text body
	HTMLBody   string            // Optional: HTML body
	ConfigSet  string            // Optional: SES configuration set for tracking
	Tags       map[string]string // Optional: email tags for categorization
	ReturnPath string            // Optional: bounce address
}

// Validate checks if the email message has required fields
func (e *EmailMessage) Validate() error {
	if len(e.To) == 0 {
		return fmt.Errorf("at least one recipient (To) is required")
	}
	if e.From == "" {
		return fmt.Errorf("from address is required")
	}
	if e.Subject == "" {
		return fmt.Errorf("subject is required")
	}
	if e.TextBody == "" && e.HTMLBody == "" {
		return fmt.Errorf("either text or HTML body is required")
	}

	// Validate email format (basic check)
	allEmails := append(append(append([]string{e.From}, e.To...), e.Cc...), e.Bcc...)
	for _, email := range allEmails {
		if email != "" && !strings.Contains(email, "@") {
			return fmt.Errorf("invalid email format: %s", email)
		}
	}

	return nil
}

// BulkRecipient represents a recipient in bulk email sending
type BulkRecipient struct {
	Email           string
	ReplacementTags map[string]string // Template variables for this recipient
}

// SendQuota represents email sending limits
type SendQuota struct {
	Max24HourSend   float64
	MaxSendRate     float64
	SentLast24Hours float64
}

// SendStatistics represents email sending statistics
type SendStatistics struct {
	Timestamp        string
	DeliveryAttempts int64
	Bounces          int64
	Complaints       int64
	Rejects          int64
}

// formatAddress creates "Name <email@domain.com>" format if name is provided
func formatAddress(email, name string) string {
	if name == "" {
		return email
	}
	// Escape quotes in name and wrap in quotes if it contains special chars
	if strings.ContainsAny(name, ",;\"") {
		name = strings.ReplaceAll(name, "\"", "\\\"")
		return fmt.Sprintf("\"%s\" <%s>", name, email)
	}
	return fmt.Sprintf("%s <%s>", name, email)
}

// retryWithExponentialBackoff executes a function with retry logic using exponential backoff
// maxRetries: number of retry attempts (3 means total 4 attempts: 1 initial + 3 retries)
// baseDelay: initial delay duration (will be doubled on each retry)
func retryWithExponentialBackoff(ctx context.Context, maxRetries int, baseDelay time.Duration, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Execute the operation
		err := operation()
		if err == nil {
			// Success
			return nil
		}

		lastErr = err

		// Don't sleep after the last attempt
		if attempt < maxRetries {
			// Calculate exponential backoff: baseDelay * 2^attempt
			backoffDuration := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))

			logger.GetDefault().Warnf("Operation failed (attempt %d/%d), retrying after %v: %v",
				attempt+1, maxRetries+1, backoffDuration, err)

			// Check if context is cancelled before sleeping
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoffDuration):
				// Continue to next retry
			}
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", maxRetries+1, lastErr)
}

// SESClient implements EmailService interface for AWS SES
type SESClient struct {
	client           *ses.Client
	defaultFromEmail string // Default verified identity
	defaultFromName  string // Default sender name
	defaultConfigSet string // Default configuration set
}

// Ensure SESClient implements EmailService interface at compile time
var _ EmailService = (*SESClient)(nil)

// NewSESClient creates a new SES client with ambient AWS credentials
func NewSESClient(defaultFromEmail string) (*SESClient, error) {
	if defaultFromEmail == "" {
		return nil, fmt.Errorf("default from email is required")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	return &SESClient{
		client:           ses.NewFromConfig(awsCfg),
		defaultFromEmail: defaultFromEmail,
	}, nil
}

// NewSESClientFromConfig builds SES client using structured application config
func NewSESClientFromConfig(c config.SESConfig) (*SESClient, error) {
	if c.DefaultFromEmail == "" {
		return nil, fmt.Errorf("default from email is required")
	}
	if c.Region == "" {
		return nil, fmt.Errorf("ses region is required")
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(c.Region)}

	if c.Profile != "" {
		loadOpts = append(loadOpts, awsconfig.WithSharedConfigProfile(c.Profile))
	}

	if c.Endpoint != "" {
		// Custom endpoint (e.g., localstack for testing)
		loadOpts = append(loadOpts, awsconfig.WithBaseEndpoint(c.Endpoint))
	}

	// Static credentials override if provided
	if c.AccessKey != "" && c.SecretKey != "" {
		staticCreds := credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, c.SessionToken)
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(staticCreds))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	// Defensive region fallback
	if awsCfg.Region == "" {
		if envRegion := os.Getenv("AWS_REGION"); envRegion != "" {
			awsCfg.Region = envRegion
		}
	}
	if awsCfg.Region == "" {
		return nil, fmt.Errorf("aws region could not be resolved")
	}

	return &SESClient{
		client:           ses.NewFromConfig(awsCfg),
		defaultFromEmail: c.DefaultFromEmail,
		defaultFromName:  c.DefaultFromName,
		defaultConfigSet: c.DefaultConfigSet,
	}, nil
}

// SendEmail sends an email via SES with retry logic (3 retries with exponential backoff)
func (s *SESClient) SendEmail(ctx context.Context, msg *EmailMessage) (messageID string, err error) {
	if err := msg.Validate(); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	// Use defaults if not specified
	fromAddr := msg.From
	if fromAddr == "" {
		fromAddr = s.defaultFromEmail
	}
	fromName := msg.FromName
	if fromName == "" {
		fromName = s.defaultFromName
	}

	// Build destination
	dest := &types.Destination{
		ToAddresses: msg.To,
	}
	if len(msg.Cc) > 0 {
		dest.CcAddresses = msg.Cc
	}
	if len(msg.Bcc) > 0 {
		dest.BccAddresses = msg.Bcc
	}

	// Build message body
	body := &types.Body{}
	if msg.TextBody != "" {
		body.Text = &types.Content{
			Charset: aws.String("UTF-8"),
			Data:    aws.String(msg.TextBody),
		}
	}
	if msg.HTMLBody != "" {
		body.Html = &types.Content{
			Charset: aws.String("UTF-8"),
			Data:    aws.String(msg.HTMLBody),
		}
	}

	input := &ses.SendEmailInput{
		Source:      aws.String(formatAddress(fromAddr, fromName)),
		Destination: dest,
		Message: &types.Message{
			Subject: &types.Content{
				Charset: aws.String("UTF-8"),
				Data:    aws.String(msg.Subject),
			},
			Body: body,
		},
	}

	// Optional fields
	if len(msg.ReplyTo) > 0 {
		input.ReplyToAddresses = msg.ReplyTo
	}
	if msg.ConfigSet != "" {
		input.ConfigurationSetName = aws.String(msg.ConfigSet)
	} else if s.defaultConfigSet != "" {
		input.ConfigurationSetName = aws.String(s.defaultConfigSet)
	}
	if msg.ReturnPath != "" {
		input.ReturnPath = aws.String(msg.ReturnPath)
	}
	if len(msg.Tags) > 0 {
		tags := make([]types.MessageTag, 0, len(msg.Tags))
		for k, v := range msg.Tags {
			tags = append(tags, types.MessageTag{
				Name:  aws.String(k),
				Value: aws.String(v),
			})
		}
		input.Tags = tags
	}

	// Retry logic: 3 retries with exponential backoff starting at 1 second
	// Total attempts: 4 (initial + 3 retries)
	// Backoff delays: 1s, 2s, 4s
	var resp *ses.SendEmailOutput
	err = retryWithExponentialBackoff(ctx, 3, 1*time.Second, func() error {
		var sendErr error
		resp, sendErr = s.client.SendEmail(ctx, input)
		return sendErr
	})

	if err != nil {
		return "", fmt.Errorf("failed to send email after retries: %w", err)
	}

	return aws.ToString(resp.MessageId), nil
}

// SendBulkEmail sends personalized emails to multiple recipients with retry logic for each email
func (s *SESClient) SendBulkEmail(ctx context.Context, recipients []BulkRecipient, subject, htmlBody, textBody string) ([]string, error) {
	if len(recipients) == 0 {
		return nil, fmt.Errorf("at least one recipient required")
	}
	if subject == "" || (htmlBody == "" && textBody == "") {
		return nil, fmt.Errorf("subject and body required")
	}

	messageIDs := make([]string, 0, len(recipients))
	var failedRecipients []string

	for _, recipient := range recipients {
		msg := &EmailMessage{
			To:       []string{recipient.Email},
			From:     s.defaultFromEmail,
			FromName: s.defaultFromName,
			Subject:  subject,
			HTMLBody: htmlBody,
			TextBody: textBody,
			Tags:     recipient.ReplacementTags,
		}

		// Simple template replacement if tags provided
		if len(recipient.ReplacementTags) > 0 {
			for key, value := range recipient.ReplacementTags {
				placeholder := fmt.Sprintf("{{%s}}", key)
				msg.Subject = strings.ReplaceAll(msg.Subject, placeholder, value)
				msg.HTMLBody = strings.ReplaceAll(msg.HTMLBody, placeholder, value)
				msg.TextBody = strings.ReplaceAll(msg.TextBody, placeholder, value)
			}
		}

		// SendEmail now has retry logic built-in (3 retries with exponential backoff)
		msgID, err := s.SendEmail(ctx, msg)
		if err != nil {
			// Log failed recipient and continue with others
			logger.GetDefault().Errorf("Failed to send email to %s after retries: %v", recipient.Email, err)
			failedRecipients = append(failedRecipients, recipient.Email)
			continue
		}
		messageIDs = append(messageIDs, msgID)
	}

	if len(messageIDs) == 0 {
		return nil, fmt.Errorf("failed to send any emails to %d recipients", len(recipients))
	}

	// Log summary if there were any failures
	if len(failedRecipients) > 0 {
		logger.GetDefault().Warnf("Bulk email completed with %d successes and %d failures. Failed recipients: %v",
			len(messageIDs), len(failedRecipients), failedRecipients)
	}

	return messageIDs, nil
}

// GetSendQuota retrieves the current sending limits for the AWS account
func (s *SESClient) GetSendQuota(ctx context.Context) (*SendQuota, error) {
	resp, err := s.client.GetSendQuota(ctx, &ses.GetSendQuotaInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to get send quota: %w", err)
	}

	return &SendQuota{
		Max24HourSend:   resp.Max24HourSend,
		MaxSendRate:     resp.MaxSendRate,
		SentLast24Hours: resp.SentLast24Hours,
	}, nil
}

// GetSendStatistics retrieves sending statistics (SES-specific, not in interface)
func (s *SESClient) GetSendStatistics(ctx context.Context) ([]SendStatistics, error) {
	resp, err := s.client.GetSendStatistics(ctx, &ses.GetSendStatisticsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to get send statistics: %w", err)
	}

	stats := make([]SendStatistics, len(resp.SendDataPoints))
	for i, point := range resp.SendDataPoints {
		stats[i] = SendStatistics{
			Timestamp:        point.Timestamp.String(),
			DeliveryAttempts: point.DeliveryAttempts,
			Bounces:          point.Bounces,
			Complaints:       point.Complaints,
			Rejects:          point.Rejects,
		}
	}
	return stats, nil
}

// VerifyEmailIdentity initiates verification for an email address
func (s *SESClient) VerifyEmailIdentity(ctx context.Context, email string) error {
	if email == "" || !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email address")
	}

	_, err := s.client.VerifyEmailIdentity(ctx, &ses.VerifyEmailIdentityInput{
		EmailAddress: aws.String(email),
	})
	if err != nil {
		return fmt.Errorf("failed to verify email identity: %w", err)
	}
	return nil
}

// ListVerifiedEmailAddresses retrieves all verified email addresses
func (s *SESClient) ListVerifiedEmailAddresses(ctx context.Context) ([]string, error) {
	resp, err := s.client.ListVerifiedEmailAddresses(ctx, &ses.ListVerifiedEmailAddressesInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list verified emails: %w", err)
	}
	return resp.VerifiedEmailAddresses, nil
}

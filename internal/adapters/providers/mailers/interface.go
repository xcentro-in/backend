package mailers

import "context"

// EmailService defines the interface for email service providers
// This allows dependency injection and easy swapping of providers (SES, SendGrid, Mailgun, etc.)
type EmailService interface {
	// SendEmail sends a single email
	SendEmail(ctx context.Context, msg *EmailMessage) (messageID string, err error)

	// SendBulkEmail sends personalized emails to multiple recipients
	SendBulkEmail(ctx context.Context, recipients []BulkRecipient, subject, htmlBody, textBody string) (messageIDs []string, err error)

	// GetSendQuota retrieves current sending limits
	GetSendQuota(ctx context.Context) (*SendQuota, error)

	// VerifyEmailIdentity initiates verification for an email address/domain
	VerifyEmailIdentity(ctx context.Context, email string) error

	// ListVerifiedEmailAddresses retrieves all verified email addresses
	ListVerifiedEmailAddresses(ctx context.Context) ([]string, error)
}

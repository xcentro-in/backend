package mailers

import (
	"context"
	"fmt"
	"log"

	"xcentro.in/internal/shared/config"
)

// ExampleSESUsage demonstrates how to use the SES mailer with configuration
func ExampleSESUsage() {
	// Load configuration from environment variables or config file
	cfg, err := config.Load("") // empty string means env vars only
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create SES client from configuration
	sesClient, err := NewSESClientFromConfig(cfg.SES)
	if err != nil {
		log.Fatalf("Failed to create SES client: %v", err)
	}

	// Example: Send a simple email
	ctx := context.Background()

	message := &EmailMessage{
		To:       []string{"recipient@example.com"},
		Subject:  "Welcome to XCentro!",
		TextBody: "Welcome to our platform. We're excited to have you on board!",
		HTMLBody: "<h1>Welcome to XCentro!</h1><p>We're excited to have you on board!</p>",
		Tags: map[string]string{
			"campaign": "welcome",
			"type":     "transactional",
		},
	}

	messageID, err := sesClient.SendEmail(ctx, message)
	if err != nil {
		log.Fatalf("Failed to send email: %v", err)
	}

	fmt.Printf("Email sent successfully! Message ID: %s\n", messageID)

	// Example: Send bulk emails
	recipients := []BulkRecipient{
		{
			Email: "user1@example.com",
			ReplacementTags: map[string]string{
				"name": "John Doe",
				"plan": "Premium",
			},
		},
		{
			Email: "user2@example.com",
			ReplacementTags: map[string]string{
				"name": "Jane Smith",
				"plan": "Basic",
			},
		},
	}

	subject := "Hello {{name}}, welcome to your {{plan}} plan!"
	htmlBody := "<h1>Hello {{name}}!</h1><p>Welcome to your {{plan}} plan. Enjoy our services!</p>"
	textBody := "Hello {{name}}! Welcome to your {{plan}} plan. Enjoy our services!"

	messageIDs, err := sesClient.SendBulkEmail(ctx, recipients, subject, htmlBody, textBody)
	if err != nil {
		log.Fatalf("Failed to send bulk emails: %v", err)
	}

	fmt.Printf("Bulk emails sent successfully! Message IDs: %v\n", messageIDs)

	// Example: Check send quota
	quota, err := sesClient.GetSendQuota(ctx)
	if err != nil {
		log.Fatalf("Failed to get send quota: %v", err)
	}

	fmt.Printf("Send Quota - Max 24h: %.0f, Max Rate: %.2f/sec, Sent Last 24h: %.0f\n",
		quota.Max24HourSend, quota.MaxSendRate, quota.SentLast24Hours)

	// Example: Verify email identity
	err = sesClient.VerifyEmailIdentity(ctx, "noreply@yourdomain.com")
	if err != nil {
		log.Printf("Failed to verify email identity: %v", err)
	} else {
		fmt.Println("Email identity verification initiated")
	}

	// Example: List verified email addresses
	verifiedEmails, err := sesClient.ListVerifiedEmailAddresses(ctx)
	if err != nil {
		log.Printf("Failed to list verified emails: %v", err)
	} else {
		fmt.Printf("Verified email addresses: %v\n", verifiedEmails)
	}
}

// Configuration Environment Variables Guide:
// Required:
// - SES_DEFAULT_FROM_EMAIL: The default sender email address (must be verified in SES)
// - SES_REGION: AWS region for SES (e.g., us-east-1, eu-west-1)
//
// Optional:
// - SES_DEFAULT_FROM_NAME: Default sender display name
// - SES_DEFAULT_CONFIG_SET: Default SES configuration set name for tracking
// - SES_ENDPOINT: Custom endpoint for testing (e.g., localstack)
// - SES_ACCESS_KEY: Static AWS access key (if not using IAM roles)
// - SES_SECRET_KEY: Static AWS secret key (if not using IAM roles)
// - SES_SESSION_TOKEN: AWS session token (for temporary credentials)
// - AWS_PROFILE: AWS profile name from shared credentials file
//
// Example .env file:
// SES_DEFAULT_FROM_EMAIL=noreply@yourdomain.com
// SES_DEFAULT_FROM_NAME=XCentro Support
// SES_REGION=us-east-1
// SES_DEFAULT_CONFIG_SET=transactional-emails
//
// For production, consider using IAM roles instead of static credentials

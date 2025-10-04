# SES Mailer Documentation

This package provides a robust AWS SES (Simple Email Service) client implementation for sending emails in the XCentro backend application.

## Features

- **Simple Email Sending**: Send individual emails with text and/or HTML content
- **Bulk Email Support**: Send personalized emails to multiple recipients with template variables
- **Configuration Management**: Flexible configuration using environment variables or config files
- **Factory Pattern**: Easy instantiation and dependency injection
- **AWS Integration**: Full AWS SDK v2 integration with support for IAM roles and static credentials
- **Validation**: Email message validation before sending
- **Monitoring**: Send quota and statistics retrieval
- **Identity Management**: Email address verification and listing

## Configuration

### Environment Variables

#### Required
- `SES_DEFAULT_FROM_EMAIL`: Default sender email address (must be verified in SES)
- `SES_REGION`: AWS region for SES (e.g., `us-east-1`, `eu-west-1`)

#### Optional
- `SES_DEFAULT_FROM_NAME`: Default sender display name
- `SES_DEFAULT_CONFIG_SET`: Default SES configuration set for tracking
- `SES_ENDPOINT`: Custom endpoint for testing (e.g., localstack)
- `SES_ACCESS_KEY`: Static AWS access key (if not using IAM roles)
- `SES_SECRET_KEY`: Static AWS secret key (if not using IAM roles)
- `SES_SESSION_TOKEN`: AWS session token (for temporary credentials)
- `AWS_PROFILE`: AWS profile name from shared credentials file

### Example .env Configuration

```env
SES_DEFAULT_FROM_EMAIL=noreply@yourdomain.com
SES_DEFAULT_FROM_NAME=XCentro Support
SES_REGION=us-east-1
SES_DEFAULT_CONFIG_SET=transactional-emails
```

## Usage Examples

### 1. Basic Email Sending

```go
package main

import (
    "context"
    "log"
    
    "xcentro.in/internal/adapters/providers/mailers"
    "xcentro.in/internal/shared/config"
)

func main() {
    // Load configuration
    cfg, err := config.Load("")
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Create SES client
    sesClient, err := mailers.NewSESClientFromConfig(cfg.SES)
    if err != nil {
        log.Fatalf("Failed to create SES client: %v", err)
    }

    // Send email
    message := &mailers.EmailMessage{
        To:       []string{"user@example.com"},
        Subject:  "Welcome!",
        TextBody: "Welcome to our platform!",
        HTMLBody: "<h1>Welcome!</h1><p>Welcome to our platform!</p>",
    }

    messageID, err := sesClient.SendEmail(context.Background(), message)
    if err != nil {
        log.Fatalf("Failed to send email: %v", err)
    }

    log.Printf("Email sent! Message ID: %s", messageID)
}
```

### 2. Using the Factory Pattern

```go
package main

import (
    "context"
    "log"
    
    "xcentro.in/internal/adapters/providers/mailers"
    "xcentro.in/internal/shared/config"
)

func main() {
    // Load configuration and initialize factory
    cfg, err := config.Load("")
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Initialize global factory (typically done at app startup)
    mailers.InitializeDefaultFactory(cfg)

    // Get email service
    emailService, err := mailers.GetDefaultEmailService()
    if err != nil {
        log.Fatalf("Failed to get email service: %v", err)
    }

    // Use the service
    message := &mailers.EmailMessage{
        To:      []string{"user@example.com"},
        Subject: "Hello from Factory!",
        TextBody: "This email was sent using the factory pattern.",
    }

    _, err = emailService.SendEmail(context.Background(), message)
    if err != nil {
        log.Fatalf("Failed to send email: %v", err)
    }
}
```

### 3. Bulk Email with Template Variables

```go
recipients := []mailers.BulkRecipient{
    {
        Email: "john@example.com",
        ReplacementTags: map[string]string{
            "name": "John Doe",
            "plan": "Premium",
        },
    },
    {
        Email: "jane@example.com",
        ReplacementTags: map[string]string{
            "name": "Jane Smith",
            "plan": "Basic",
        },
    },
}

subject := "Hello {{name}}, welcome to your {{plan}} plan!"
htmlBody := "<h1>Hello {{name}}!</h1><p>Welcome to your {{plan}} plan!</p>"
textBody := "Hello {{name}}! Welcome to your {{plan}} plan!"

messageIDs, err := sesClient.SendBulkEmail(context.Background(), recipients, subject, htmlBody, textBody)
if err != nil {
    log.Fatalf("Failed to send bulk emails: %v", err)
}

log.Printf("Sent %d emails with IDs: %v", len(messageIDs), messageIDs)
```

### 4. Advanced Email Features

```go
message := &mailers.EmailMessage{
    To:         []string{"primary@example.com"},
    Cc:         []string{"cc@example.com"},
    Bcc:        []string{"bcc@example.com"},
    From:       "custom@yourdomain.com", // Override default
    FromName:   "Custom Sender",
    ReplyTo:    []string{"support@yourdomain.com"},
    Subject:    "Advanced Email Features",
    TextBody:   "Plain text version",
    HTMLBody:   "<h1>HTML version</h1>",
    ConfigSet:  "marketing-emails", // Override default
    ReturnPath: "bounces@yourdomain.com",
    Tags: map[string]string{
        "campaign": "product-launch",
        "segment":  "premium-users",
    },
}

messageID, err := sesClient.SendEmail(context.Background(), message)
if err != nil {
    log.Fatalf("Failed to send email: %v", err)
}
```

## Integration with Handlers

For HTTP handlers, you can inject the email service:

```go
type UserHandler struct {
    emailService mailers.EmailService
}

func NewUserHandler(emailService mailers.EmailService) *UserHandler {
    return &UserHandler{
        emailService: emailService,
    }
}

func (h *UserHandler) RegisterUser(w http.ResponseWriter, r *http.Request) {
    // ... user registration logic ...
    
    // Send welcome email
    message := &mailers.EmailMessage{
        To:       []string{user.Email},
        Subject:  "Welcome to XCentro!",
        HTMLBody: generateWelcomeHTML(user),
        TextBody: generateWelcomeText(user),
        Tags: map[string]string{
            "type": "welcome",
            "user_id": user.ID,
        },
    }

    _, err := h.emailService.SendEmail(r.Context(), message)
    if err != nil {
        log.Printf("Failed to send welcome email: %v", err)
        // Don't fail the registration, just log the error
    }
}
```

## AWS SES Setup Requirements

1. **Verify Email Addresses/Domains**: Before sending emails, you must verify sender identities in the AWS SES console
2. **Move out of Sandbox**: New SES accounts are in sandbox mode and can only send to verified addresses
3. **Configure DNS**: For domain verification, add required DNS records
4. **Set up Configuration Sets** (optional): For tracking and monitoring email events

## Security Best Practices

1. **Use IAM Roles**: Prefer IAM roles over static credentials in production
2. **Minimal Permissions**: Grant only necessary SES permissions
3. **Environment Variables**: Store sensitive configuration in environment variables
4. **Validate Inputs**: Always validate email addresses and content before sending
5. **Rate Limiting**: Respect SES sending limits to avoid throttling

## Error Handling

The mailer includes comprehensive error handling:

```go
messageID, err := sesClient.SendEmail(ctx, message)
if err != nil {
    // Check for specific AWS errors
    var awsErr *types.MessageRejected
    if errors.As(err, &awsErr) {
        log.Printf("Message rejected by SES: %v", awsErr)
    }
    
    var rateLimitErr *types.SendingPausedException
    if errors.As(err, &rateLimitErr) {
        log.Printf("Sending paused - rate limit exceeded: %v", rateLimitErr)
    }
    
    return fmt.Errorf("failed to send email: %w", err)
}
```

## Monitoring and Observability

Monitor your email sending:

```go
// Check send quota
quota, err := sesClient.GetSendQuota(ctx)
if err == nil {
    log.Printf("Remaining sends today: %.0f", quota.Max24HourSend - quota.SentLast24Hours)
}

// Get sending statistics
stats, err := sesClient.GetSendStatistics(ctx)
if err == nil {
    for _, stat := range stats {
        log.Printf("Stats for %s: Sent=%d, Bounced=%d, Complained=%d", 
            stat.Timestamp, stat.DeliveryAttempts, stat.Bounces, stat.Complaints)
    }
}
```
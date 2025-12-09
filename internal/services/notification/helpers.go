package notification

import (
	"context"
	"fmt"
	"strings"

	"xcentro.in/internal/adapters/providers/mailers"
	"xcentro.in/internal/core/notification"
)

// UserInfo contains user information for notification
type UserInfo struct {
	UserID string
	Email  string
	Name   string
}

// buildEmailHTML wraps email content in a simple, clean HTML template with header and footer
func buildEmailHTML(title, body string, actions []notification.ActionButton) string {
	var sb strings.Builder

	// HTML header
	sb.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333333;
            background-color: #f4f4f4;
            margin: 0;
            padding: 0;
        }
        .container {
            max-width: 600px;
            margin: 20px auto;
            background-color: #ffffff;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: #ffffff;
            padding: 30px 20px;
            text-align: center;
        }
        .header h1 {
            margin: 0;
            font-size: 24px;
            font-weight: 600;
        }
        .body {
            padding: 30px 20px;
        }
        .body p {
            margin: 0 0 15px 0;
            font-size: 16px;
        }
        .actions {
            margin: 30px 0;
            text-align: center;
        }
        .button {
            display: inline-block;
            padding: 12px 30px;
            margin: 5px;
            background-color: #667eea;
            color: #ffffff;
            text-decoration: none;
            border-radius: 5px;
            font-weight: 500;
        }
        .footer {
            background-color: #f8f9fa;
            padding: 20px;
            text-align: center;
            font-size: 14px;
            color: #888888;
            border-top: 1px solid #e9ecef;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>` + title + `</h1>
        </div>
        <div class="body">
`)

	// Format body content (convert newlines to paragraphs if not already HTML)
	if strings.Contains(body, "<p>") || strings.Contains(body, "<div>") {
		sb.WriteString("            " + body + "\n")
	} else {
		lines := strings.Split(body, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				sb.WriteString("            <p>" + line + "</p>\n")
			}
		}
	}

	// Add action buttons if present
	if len(actions) > 0 {
		sb.WriteString(`            <div class="actions">
`)
		for _, action := range actions {
			sb.WriteString(fmt.Sprintf(`                <a href="%s" class="button">%s</a>
`, action.Href, action.Text))
		}
		sb.WriteString(`            </div>
`)
	}

	// Footer
	sb.WriteString(`        </div>
        <div class="footer">
            <p>Xcentro </p>
            <p>This is a system generated mail. Please do not reply to this email.</p>
        </div>
    </div>
</body>
</html>`)

	return sb.String()
}

// SendEmailNotification sends a formatted HTML email notification
// The notificationID is used as an idempotency key to prevent duplicate sends
func SendEmailNotification(ctx context.Context, emailClient *mailers.SESClient, notificationID int64, to, subject, body string, actions []notification.ActionButton, fromEmail, fromName string) error {
	htmlBody := buildEmailHTML(subject, body, actions)

	msg := &mailers.EmailMessage{
		From:     fromEmail,
		FromName: fromName,
		To:       []string{to},
		Subject:  subject,
		HTMLBody: htmlBody,
		TextBody: stripHTML(body),
		Tags: map[string]string{
			"notification_id": fmt.Sprintf("%d", notificationID),
			"source":          "xcentro_notification_system",
		},
	}

	_, err := emailClient.SendEmail(ctx, msg)
	return err
}

// stripHTML removes HTML tags for plain text version
func stripHTML(html string) string {
	text := strings.ReplaceAll(html, "<br>", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<p>", "")
	text = strings.ReplaceAll(text, "</p>", "\n\n")

	for strings.Contains(text, "<") && strings.Contains(text, ">") {
		start := strings.Index(text, "<")
		end := strings.Index(text, ">")
		if start < end {
			text = text[:start] + text[end+1:]
		} else {
			break
		}
	}

	return strings.TrimSpace(text)
}

// buildNotificationContent builds the notification content JSONB
func buildNotificationContent(payload notification.NotificationEventPayload) map[string]interface{} {
	content := map[string]interface{}{
		"title":      payload.Message.Title,
		"body":       payload.Message.Body,
		"event_type": payload.EventType,
	}

	if len(payload.Actions) > 0 {
		content["actions"] = payload.Actions
	}

	// Include entity references for the UI to create deep links
	if payload.Entities.PaymentID != nil {
		content["payment_id"] = *payload.Entities.PaymentID
	}
	if payload.Entities.VoucherID != nil {
		content["voucher_id"] = *payload.Entities.VoucherID
	}
	if payload.Entities.PayeeID != nil {
		content["payee_id"] = *payload.Entities.PayeeID
	}
	if payload.Entities.ReportID != nil {
		content["report_id"] = *payload.Entities.ReportID
	}

	return content
}

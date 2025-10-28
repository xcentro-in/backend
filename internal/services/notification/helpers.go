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

// SendEmailNotification sends an email notification with idempotency protection
// The notificationID is used as an idempotency key to prevent duplicate sends
func SendEmailNotification(ctx context.Context, emailClient *mailers.SESClient, notificationID int64, to, subject, body string) error {
	msg := &mailers.EmailMessage{
		To:       []string{to},
		Subject:  subject,
		HTMLBody: body,
		TextBody: stripHTML(body),
		Tags: map[string]string{
			"notification_id": fmt.Sprintf("%d", notificationID), // Idempotency key
			"source":          "xcentro_notification_system",
		},
	}

	_, err := emailClient.SendEmail(ctx, msg)
	return err
}

// stripHTML removes HTML tags for plain text version (basic implementation)
func stripHTML(html string) string {
	// Basic HTML tag removal
	text := strings.ReplaceAll(html, "<br>", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<p>", "")
	text = strings.ReplaceAll(text, "</p>", "\n\n")

	// Remove all other tags
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
// This is called by the fanout service to create the content for user_notifications
func buildNotificationContent(payload notification.NotificationEventPayload) map[string]interface{} {
	content := map[string]interface{}{
		"title": payload.Message.Title,
		"body":  payload.Message.Body,
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

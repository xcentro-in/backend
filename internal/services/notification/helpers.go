package notification

import (
	"context"
	"fmt"
	"strings"
	"time"

	"xcentro.in/internal/adapters/providers/mailers"
	"xcentro.in/internal/core/notification"
)

// formatTimestamp returns current time in RFC3339 format
func formatTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// GetUsersByTargetInfo retrieves user IDs and emails based on target info
// This function queries business_user_relation to find target users
func GetUsersByTargetInfo(ctx context.Context, db interface{}, target notification.TargetInfo) ([]UserInfo, error) {
	var users []UserInfo

	switch target.Type {
	case notification.TargetUnicast:
		// Specific user IDs provided
		if len(target.IDs) == 0 {
			return nil, fmt.Errorf("unicast requires at least one user ID")
		}
		users = getUsersByIDs(ctx, db, target.IDs)

	case notification.TargetMulticast:
		// Multiple users with optional role filter
		if target.MinimumRole != nil {
			users = getUsersByBusinessAndRole(ctx, db, target.BusinessID, *target.MinimumRole)
		} else if len(target.IDs) > 0 {
			users = getUsersByIDs(ctx, db, target.IDs)
		}

	case notification.TargetBroadcast:
		// All users with minimum role in business
		minRole := notification.RoleSupport // Default to all roles
		if target.MinimumRole != nil {
			minRole = *target.MinimumRole
		}
		users = getUsersByBusinessAndRole(ctx, db, target.BusinessID, minRole)

	default:
		return nil, fmt.Errorf("invalid target type: %s", target.Type)
	}

	return users, nil
}

// UserInfo contains user information for notification
type UserInfo struct {
	UserID string
	Email  string
	Name   string
}

// getUsersByIDs is a placeholder - implement based on your user repository
func getUsersByIDs(ctx context.Context, db interface{}, userIDs []string) []UserInfo {
	// TODO: Implement actual query to fetch user info by IDs
	// This should query user_table to get email and name
	return []UserInfo{}
}

// getUsersByBusinessAndRole is a placeholder - implement based on your business repository
func getUsersByBusinessAndRole(ctx context.Context, db interface{}, businessID, minimumRole string) []UserInfo {
	// TODO: Implement actual query to fetch users from business_user_relation
	// Filter by role hierarchy: owner > admin > finance > support
	return []UserInfo{}
}

// SendEmailNotification sends an email notification
func SendEmailNotification(ctx context.Context, emailClient *mailers.SESClient, to, subject, body string) error {
	msg := &mailers.EmailMessage{
		To:       []string{to},
		Subject:  subject,
		HTMLBody: body,
		TextBody: stripHTML(body),
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

// ShouldSendToChannel checks if notification should be sent to a channel based on user preferences
func ShouldSendToChannel(prefs *notification.NotificationPreferences, channel string) bool {
	switch channel {
	case notification.ChannelEmail:
		return prefs.EmailEnabled
	case notification.ChannelPush:
		return prefs.PushEnabled
	case notification.ChannelWeb:
		return prefs.MessagesEnabled
	default:
		return false
	}
}

package notification

import (
	"context"
	"fmt"
	"math"
	"time"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/providers/mailers"
	"xcentro.in/internal/core/notification"
	"xcentro.in/internal/shared/logger"
)

// NotificationService handles notification operations
type NotificationService struct {
	notifEventRepo  notification.NotificationEventRepository
	userNotifRepo   notification.UserNotificationRepository
	preferencesRepo notification.NotificationPreferencesRepository
	emailClient     *mailers.SESClient
	txManager       client.TxManager
	db              client.DB
	logger          *logger.Logger
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	notifEventRepo notification.NotificationEventRepository,
	userNotifRepo notification.UserNotificationRepository,
	preferencesRepo notification.NotificationPreferencesRepository,
	emailClient *mailers.SESClient,
	txManager client.TxManager,
	db client.DB,
	logger *logger.Logger,
) *NotificationService {
	return &NotificationService{
		notifEventRepo:  notifEventRepo,
		userNotifRepo:   userNotifRepo,
		preferencesRepo: preferencesRepo,
		emailClient:     emailClient,
		txManager:       txManager,
		db:              db,
		logger:          logger,
	}
}

// GetUserNotifications retrieves notifications for a user
func (s *NotificationService) GetUserNotifications(ctx context.Context, userID string, limit, offset int) ([]*notification.UserNotification, error) {
	return s.userNotifRepo.GetUserNotifications(ctx, userID, limit, offset)
}

// GetUnreadCount gets count of unread notifications for a user
func (s *NotificationService) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	return s.userNotifRepo.GetUnreadCount(ctx, userID)
}

// MarkAsRead marks a notification as read
func (s *NotificationService) MarkAsRead(ctx context.Context, notificationID int64, userID string) error {
	return s.userNotifRepo.MarkAsRead(ctx, notificationID, userID)
}

// MarkMultipleAsRead marks multiple notifications as read
func (s *NotificationService) MarkMultipleAsRead(ctx context.Context, notificationIDs []int64, userID string) error {
	return s.userNotifRepo.MarkMultipleAsRead(ctx, notificationIDs, userID)
}

// GetPreferences retrieves user's notification preferences
func (s *NotificationService) GetPreferences(ctx context.Context, userID string) (*notification.NotificationPreferences, error) {
	return s.preferencesRepo.GetPreferences(ctx, userID)
}

// UpdatePreferences updates user's notification preferences
func (s *NotificationService) UpdatePreferences(ctx context.Context, prefs *notification.NotificationPreferences) error {
	return s.preferencesRepo.UpsertPreferences(ctx, prefs)
}

// ManualTriggerRequest represents a manual notification trigger from ops
type ManualTriggerRequest struct {
	EventType   string                           `json:"event_type"`
	Target      notification.TargetInfo          `json:"target"`
	Message     notification.MessageContent      `json:"message"`
	Actions     []notification.ActionButton      `json:"actions,omitempty"`
	Entities    notification.EntityReferences    `json:"entities,omitempty"`
}

// TriggerManualNotification creates a notification event manually (for ops/campaigns)
func (s *NotificationService) TriggerManualNotification(ctx context.Context, req ManualTriggerRequest) error {
	// Create notification event payload
	payload := notification.NotificationEventPayload{
		Timestamp: formatTimestamp(),
		EventType: req.EventType,
		Target:    req.Target,
		Entities:  req.Entities,
		Message:   req.Message,
		Actions:   req.Actions,
	}

	// Create notification event
	event := &notification.NotificationEvent{
		EventType: req.EventType,
		Payload:   payload,
	}

	return s.notifEventRepo.CreateEvent(ctx, event)
}

// ProcessPendingNotificationEvents fetches, processes, and sends notifications
// This is called by the notification worker
// Transaction covers DB operations only; email/push sending happens AFTER commit
func (s *NotificationService) ProcessPendingNotificationEvents(ctx context.Context, batchSize int, maxRetries int) (int, error) {
	var processedCount int
	var notificationsToSend []NotificationToSend

	// Step 1: DB operations in transaction
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Create transactional repository instances
		txEventRepo := s.notifEventRepo.WithTx(tx)
		txUserNotifRepo := s.userNotifRepo.WithTx(tx)
		txPrefsRepo := s.preferencesRepo.WithTx(tx)

		// Fetch pending events with FOR UPDATE SKIP LOCKED (within transaction)
		events, err := txEventRepo.FetchPendingEvents(ctx, batchSize)
		if err != nil {
			return fmt.Errorf("failed to fetch pending events: %w", err)
		}

		if len(events) == 0 {
			return nil // No events to process
		}

		// Process each event
		var eventIDs []int64
		for _, event := range events {
			// Get target users
			// MVP_TODO: Implement getUsersByTarget - currently returns empty slice
			users, err := s.getUsersByTarget(ctx, event.Payload.Target)
			if err != nil {
				s.logger.Errorf("Failed to get target users for event %d: %v", event.ID, err)
				continue
			}

			if len(users) == 0 {
				// No users to notify, but mark event as processed
				eventIDs = append(eventIDs, event.ID)
				continue
			}

			// Create user notifications and prepare email/push queue
			for _, user := range users {
				// Get user preferences
				prefs, err := txPrefsRepo.GetPreferences(ctx, user.UserID)
				if err != nil {
					s.logger.Errorf("Failed to get preferences for user %s: %v", user.UserID, err)
					continue
				}

				// Build notification content
				content := buildNotificationContent(event.Payload, user)

				// Determine business_id
				var businessID *string
				if event.Payload.Target.BusinessID != "" {
					businessID = &event.Payload.Target.BusinessID
				}

				// Create single notification entry in DB
				userNotif := &notification.UserNotification{
					UserID:     user.UserID,
					BusinessID: businessID,
					EventID:    event.ID,
					Channel:    notification.ChannelWeb, // Primary channel
					Status:     notification.StatusPending,
					Content:    content,
				}

				if err := txUserNotifRepo.CreateUserNotification(ctx, userNotif); err != nil {
					s.logger.Errorf("Failed to create user notification: %v", err)
					continue
				}

				// Queue for sending AFTER transaction commits
				notificationsToSend = append(notificationsToSend, NotificationToSend{
					User:        user,
					Preferences: prefs,
					Content:     content,
				})
			}

			eventIDs = append(eventIDs, event.ID)
		}

		// Mark events as processed
		if len(eventIDs) > 0 {
			if err := txEventRepo.MarkEventsProcessed(ctx, eventIDs); err != nil {
				return fmt.Errorf("failed to mark events as processed: %w", err)
			}
		}

		processedCount = len(eventIDs)
		return nil
	})

	if err != nil {
		return 0, err
	}

	// Step 2: Send notifications AFTER transaction commit (outside transaction)
	for _, notif := range notificationsToSend {
		if err := s.sendNotification(ctx, notif, maxRetries); err != nil {
			s.logger.Errorf("Failed to send notification to user %s: %v", notif.User.UserID, err)
			// Don't fail the entire batch - notification is in user's inbox
		}
	}

	return processedCount, nil
}

// NotificationToSend represents a notification ready to be sent
type NotificationToSend struct {
	User        UserInfo
	Preferences *notification.NotificationPreferences
	Content     map[string]interface{}
}

// getUsersByTarget retrieves users based on target info
// MVP_TODO: Implement user resolution by querying business_user_relation table
func (s *NotificationService) getUsersByTarget(ctx context.Context, target notification.TargetInfo) ([]UserInfo, error) {
	// MVP_TODO: Implement using GetUsersByTargetInfo from helpers.go
	// This should query:
	// SELECT u.id, u.email, u.name FROM user_table u
	// JOIN business_user_relation bur ON u.id = bur.user_id
	// WHERE conditions based on target type (unicast/multicast/broadcast)

	// For now, return empty to avoid errors
	return []UserInfo{}, nil
}

// sendNotification sends notification via enabled channels with retry
func (s *NotificationService) sendNotification(ctx context.Context, notif NotificationToSend, maxRetries int) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Try email if enabled
		if notif.Preferences.EmailEnabled && notif.User.Email != "" {
			title, _ := notif.Content["title"].(string)
			body, _ := notif.Content["body"].(string)

			if err := SendEmailNotification(ctx, s.emailClient, notif.User.Email, title, body); err == nil {
				return nil // Success
			} else {
				lastErr = err
			}
		}

		// Try push if enabled (placeholder for MVP)
		if notif.Preferences.PushEnabled {
			// PHASE2_TODO: Implement push notification (FCM/APNS)
			return nil
		}

		// Web notifications are already in database (user_notifications table)
		if notif.Preferences.MessagesEnabled {
			return nil
		}

		// Exponential backoff
		if attempt < maxRetries-1 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			time.Sleep(backoff)
		}
	}

	return fmt.Errorf("all retry attempts failed: %w", lastErr)
}

// buildNotificationContent builds the notification content JSONB
func buildNotificationContent(payload notification.NotificationEventPayload, user UserInfo) map[string]interface{} {
	content := map[string]interface{}{
		"title": payload.Message.Title,
		"body":  payload.Message.Body,
	}

	if len(payload.Actions) > 0 {
		content["actions"] = payload.Actions
	}

	if payload.Entities.PaymentID != nil {
		content["payment_id"] = *payload.Entities.PaymentID
	}

	return content
}

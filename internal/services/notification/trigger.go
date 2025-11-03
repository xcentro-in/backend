package notification

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/notification"
)

// Global notification trigger (singleton pattern)
var (
	globalTrigger     *NotificationTrigger
	globalTriggerOnce sync.Once
)

// NotificationTrigger provides a simple interface to trigger notifications from anywhere
// This avoids passing repositories to every service
type NotificationTrigger struct {
	eventRepo notification.NotificationEventRepository
}

// InitializeNotificationTrigger initializes the global notification trigger
// Call this once during application startup (in main.go or API initialization)
func InitializeNotificationTrigger(eventRepo notification.NotificationEventRepository) {
	globalTriggerOnce.Do(func() {
		globalTrigger = &NotificationTrigger{
			eventRepo: eventRepo,
		}
	})
}

// Trigger creates a notification event
// This is the main function other services should call
func Trigger(ctx context.Context, payload notification.NotificationEventPayload) error {
	if globalTrigger == nil {
		return fmt.Errorf("notification trigger not initialized - call InitializeNotificationTrigger in main.go")
	}

	event := &notification.NotificationEvent{
		EventType: payload.EventType,
		Payload:   payload,
	}

	return globalTrigger.eventRepo.CreateEvent(ctx, event)
}

// TriggerWithTx creates a notification event within an existing transaction
// Use this when you need to ensure the notification is created in the same transaction
func TriggerWithTx(ctx context.Context, tx client.Tx, payload notification.NotificationEventPayload) error {
	if globalTrigger == nil {
		return fmt.Errorf("notification trigger not initialized - call InitializeNotificationTrigger in main.go")
	}

	event := &notification.NotificationEvent{
		EventType: payload.EventType,
		Payload:   payload,
	}

	// Create a transaction-aware event repository
	txEventRepo := globalTrigger.eventRepo.WithTx(tx)
	return txEventRepo.CreateEvent(ctx, event)
}

// TriggerAsync creates a notification event asynchronously (fire and forget)
// Use this when you don't want to block on notification creation
// Logs errors but doesn't return them
func TriggerAsync(ctx context.Context, payload notification.NotificationEventPayload) {
	go func() {
		if err := Trigger(ctx, payload); err != nil {
			// Log error but don't fail (fire and forget)
			fmt.Printf("Failed to trigger notification (async): %v\n", err)
		}
	}()
}

// Helper functions for common notification patterns

// TriggerUnicast sends notification to a single user
func TriggerUnicast(ctx context.Context, businessID, userID, eventType, title, body string, entities notification.EntityReferences, actions []notification.ActionButton) error {
	payload := notification.NotificationEventPayload{
		Timestamp: time.Now().Format(time.RFC3339),
		EventType: eventType,
		Target: notification.TargetInfo{
			BusinessID: businessID,
			Type:       notification.TargetUnicast,
			IDs:        []string{userID},
		},
		Entities: entities,
		Message: notification.MessageContent{
			Title: title,
			Body:  body,
		},
		Actions: actions,
	}

	return Trigger(ctx, payload)
}

// TriggerUnicastWithTx sends notification to a single user within an existing transaction
func TriggerUnicastWithTx(ctx context.Context, tx client.Tx, businessID, userID, eventType, title, body string, entities notification.EntityReferences, actions []notification.ActionButton) error {
	payload := notification.NotificationEventPayload{
		Timestamp: time.Now().Format(time.RFC3339),
		EventType: eventType,
		Target: notification.TargetInfo{
			BusinessID: businessID,
			Type:       notification.TargetUnicast,
			IDs:        []string{userID},
		},
		Entities: entities,
		Message: notification.MessageContent{
			Title: title,
			Body:  body,
		},
		Actions: actions,
	}

	return TriggerWithTx(ctx, tx, payload)
}

// TriggerMulticast sends notification to users based on role or specific user IDs
// If minimumRole is provided, sends to all users with role >= minimumRole
// Otherwise, sends to specific users in userIDs list (fallback)
func TriggerMulticast(ctx context.Context, businessID, eventType, title, body string, minimumRole *string, userIDs []string, entities notification.EntityReferences, actions []notification.ActionButton) error {
	payload := notification.NotificationEventPayload{
		Timestamp: time.Now().Format(time.RFC3339),
		EventType: eventType,
		Target: notification.TargetInfo{
			BusinessID:  businessID,
			Type:        notification.TargetMulticast,
			MinimumRole: minimumRole,
			IDs:         userIDs,
		},
		Entities: entities,
		Message: notification.MessageContent{
			Title: title,
			Body:  body,
		},
		Actions: actions,
	}

	return Trigger(ctx, payload)
}

// TriggerMulticastWithTx sends notification to users based on role or specific user IDs within an existing transaction
// If minimumRole is provided, sends to all users with role >= minimumRole
// Otherwise, sends to specific users in userIDs list (fallback)
func TriggerMulticastWithTx(ctx context.Context, tx client.Tx, businessID, eventType, title, body string, minimumRole *string, userIDs []string, entities notification.EntityReferences, actions []notification.ActionButton) error {
	payload := notification.NotificationEventPayload{
		Timestamp: time.Now().Format(time.RFC3339),
		EventType: eventType,
		Target: notification.TargetInfo{
			BusinessID:  businessID,
			Type:        notification.TargetMulticast,
			MinimumRole: minimumRole,
			IDs:         userIDs,
		},
		Entities: entities,
		Message: notification.MessageContent{
			Title: title,
			Body:  body,
		},
		Actions: actions,
	}

	return TriggerWithTx(ctx, tx, payload)
}

// TriggerBroadcast sends notification to all users in a business with minimum role
func TriggerBroadcast(ctx context.Context, businessID, minimumRole, eventType, title, body string, entities notification.EntityReferences, actions []notification.ActionButton) error {
	payload := notification.NotificationEventPayload{
		Timestamp: time.Now().Format(time.RFC3339),
		EventType: eventType,
		Target: notification.TargetInfo{
			BusinessID:  businessID,
			Type:        notification.TargetBroadcast,
			MinimumRole: &minimumRole,
		},
		Entities: entities,
		Message: notification.MessageContent{
			Title: title,
			Body:  body,
		},
		Actions: actions,
	}

	return Trigger(ctx, payload)
}

// TriggerBroadcastWithTx sends notification to all users in a business with minimum role within an existing transaction
func TriggerBroadcastWithTx(ctx context.Context, tx client.Tx, businessID, minimumRole, eventType, title, body string, entities notification.EntityReferences, actions []notification.ActionButton) error {
	payload := notification.NotificationEventPayload{
		Timestamp: time.Now().Format(time.RFC3339),
		EventType: eventType,
		Target: notification.TargetInfo{
			BusinessID:  businessID,
			Type:        notification.TargetBroadcast,
			MinimumRole: &minimumRole,
		},
		Entities: entities,
		Message: notification.MessageContent{
			Title: title,
			Body:  body,
		},
		Actions: actions,
	}

	return TriggerWithTx(ctx, tx, payload)
}

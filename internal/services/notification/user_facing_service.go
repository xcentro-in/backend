package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/core/notification"
	"xcentro.in/internal/shared/logger"
)

// UserFacingNotificationService implements the NotificationService interface
// This handles user-facing operations like getting notifications, marking as read, and preferences
type UserFacingNotificationService struct {
	notifEventRepo  notification.NotificationEventRepository
	userNotifRepo   notification.UserNotificationRepository
	preferencesRepo notification.NotificationPreferencesRepository
	redisClient     *redis.Client
	logger          *logger.Logger
}

// NewUserFacingNotificationService creates a new user-facing notification service
func NewUserFacingNotificationService(
	notifEventRepo notification.NotificationEventRepository,
	userNotifRepo notification.UserNotificationRepository,
	preferencesRepo notification.NotificationPreferencesRepository,
	redisClient *redis.Client,
	logger *logger.Logger,
) *UserFacingNotificationService {
	return &UserFacingNotificationService{
		notifEventRepo:  notifEventRepo,
		userNotifRepo:   userNotifRepo,
		preferencesRepo: preferencesRepo,
		redisClient:     redisClient,
		logger:          logger,
	}
}

// GetUserNotifications retrieves notifications for a user
func (s *UserFacingNotificationService) GetUserNotifications(ctx context.Context, userID string, limit, offset int) ([]*notification.UserNotification, error) {
	return s.userNotifRepo.GetUserNotifications(ctx, userID, limit, offset)
}

// GetUnreadCount gets count of unread notifications for a user
func (s *UserFacingNotificationService) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	return s.userNotifRepo.GetUnreadCount(ctx, userID)
}

// MarkAsRead marks a notification as read
func (s *UserFacingNotificationService) MarkAsRead(ctx context.Context, notificationID int64, userID string) error {
	return s.userNotifRepo.MarkAsRead(ctx, notificationID, userID)
}

// MarkMultipleAsRead marks multiple notifications as read
func (s *UserFacingNotificationService) MarkMultipleAsRead(ctx context.Context, notificationIDs []int64, userID string) error {
	return s.userNotifRepo.MarkMultipleAsRead(ctx, notificationIDs, userID)
}

// GetPreferences retrieves user's notification preferences (with Redis caching)
func (s *UserFacingNotificationService) GetPreferences(ctx context.Context, userID string) (*notification.NotificationPreferences, error) {
	cacheKey := keys.Notification.Preferences(userID)

	// Try to get from cache
	cachedData, err := s.redisClient.Get(ctx, cacheKey).Result()
	if err == nil {
		// Cache hit
		var prefs notification.NotificationPreferences
		if err := json.Unmarshal([]byte(cachedData), &prefs); err == nil {
			return &prefs, nil
		}
		s.logger.Warnf("Failed to unmarshal cached preferences for user %s: %v", userID, err)
	}

	// Cache miss: Get from DB
	prefs, err := s.preferencesRepo.GetPreferences(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get preferences from DB: %w", err)
	}

	// Set cache for next time
	prefsJSON, err := json.Marshal(prefs)
	if err == nil {
		if err := s.redisClient.Set(ctx, cacheKey, prefsJSON, keys.TTLNotificationPreferences).Err(); err != nil {
			s.logger.Warnf("Failed to cache preferences for user %s: %v", userID, err)
		}
	}

	return prefs, nil
}

// UpdatePreferences updates user's notification preferences and invalidates cache
func (s *UserFacingNotificationService) UpdatePreferences(ctx context.Context, prefs *notification.NotificationPreferences) error {
	// Update in database
	if err := s.preferencesRepo.UpsertPreferences(ctx, prefs); err != nil {
		return fmt.Errorf("failed to update preferences: %w", err)
	}

	// Invalidate cache (explicit invalidation as per design document)
	cacheKey := keys.Notification.Preferences(prefs.UserID)
	if err := s.redisClient.Del(ctx, cacheKey).Err(); err != nil {
		s.logger.Warnf("Failed to invalidate cache for user %s: %v", prefs.UserID, err)
		// Don't return error - update succeeded, cache will expire eventually
	}

	return nil
}

// TriggerManualNotification creates a notification event manually (for ops/campaigns)
func (s *UserFacingNotificationService) TriggerManualNotification(ctx context.Context, payload notification.NotificationEventPayload) error {
	event := &notification.NotificationEvent{
		EventType: payload.EventType,
		Payload:   payload,
	}

	return s.notifEventRepo.CreateEvent(ctx, event)
}

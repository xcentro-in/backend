package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/notification"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
)

// FanoutService handles fan-out logic: notification_events -> user_notifications
// This service processes pending notification events and creates user notification rows
type FanoutService struct {
	notifEventRepo      notification.NotificationEventRepository
	userNotifRepo       notification.UserNotificationRepository
	preferencesRepo     notification.NotificationPreferencesRepository
	userRepo            user.UserRepository
	businessRelationRepo business.BusinessUserRelationRepository
	redisClient         *redis.Client
	txManager           client.TxManager
	logger              *logger.Logger
}

// NewFanoutService creates a new fanout service
func NewFanoutService(
	notifEventRepo notification.NotificationEventRepository,
	userNotifRepo notification.UserNotificationRepository,
	preferencesRepo notification.NotificationPreferencesRepository,
	userRepo user.UserRepository,
	businessRelationRepo business.BusinessUserRelationRepository,
	redisClient *redis.Client,
	txManager client.TxManager,
	logger *logger.Logger,
) *FanoutService {
	return &FanoutService{
		notifEventRepo:      notifEventRepo,
		userNotifRepo:       userNotifRepo,
		preferencesRepo:     preferencesRepo,
		userRepo:            userRepo,
		businessRelationRepo: businessRelationRepo,
		redisClient:         redisClient,
		txManager:           txManager,
		logger:              logger,
	}
}

// ProcessPendingEvents fetches pending notification events and creates user notifications
// This is the "Fan-out" worker logic from the design document
func (s *FanoutService) ProcessPendingEvents(ctx context.Context, batchSize int) (int, error) {
	var processedCount int

	// All operations happen in a single transaction
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Create transactional repository instances
		txEventRepo := s.notifEventRepo.WithTx(tx)
		txUserNotifRepo := s.userNotifRepo.WithTx(tx)
		txPrefsRepo := s.preferencesRepo.WithTx(tx)

		// 1. Fetch pending events with FOR UPDATE SKIP LOCKED (within transaction)
		events, err := txEventRepo.FetchPendingEvents(ctx, batchSize)
		if err != nil {
			return fmt.Errorf("failed to fetch pending events: %w", err)
		}

		if len(events) == 0 {
			return nil // No events to process
		}

		// 2. Process each event
		var eventIDs []int64
		for _, event := range events {
			// Get target users based on target info
			users, err := s.getUsersByTarget(ctx, event.Payload.Target)
			if err != nil {
				s.logger.Errorf("Failed to get target users for event %d: %v", event.ID, err)
				// Mark event as processed anyway to avoid infinite retry
				eventIDs = append(eventIDs, event.ID)
				continue
			}

			if len(users) == 0 {
				// No users to notify, mark event as processed
				eventIDs = append(eventIDs, event.ID)
				continue
			}

			// 3. Accumulate ALL notifications for this event across all users (batch insert optimization)
			var allNotificationsForEvent []*notification.UserNotification

			for _, targetUser := range users {
				// Get user preferences (uses Redis cache with read-through pattern)
				prefs, err := s.getPreferencesWithCache(ctx, txPrefsRepo, targetUser.UserID)
				if err != nil {
					s.logger.Errorf("Failed to get preferences for user %s: %v", targetUser.UserID, err)
					continue
				}

				// Build notification content
				content := buildNotificationContent(event.Payload)

				// Determine business_id
				var businessID *string
				if event.Payload.Target.BusinessID != "" {
					businessID = &event.Payload.Target.BusinessID
				}

				// 4. ALWAYS create 'web' notification (shown in website/app, web shows ALL notifications regardless of category)
				allNotificationsForEvent = append(allNotificationsForEvent, &notification.UserNotification{
					UserID:     targetUser.UserID,
					BusinessID: businessID,
					EventID:    event.ID,
					Channel:    notification.ChannelWeb,
					Status:     notification.StatusPending,
					Content:    content,
				})

				// 5. Create 'email' notification if email is enabled AND category is enabled
				if prefs.EmailEnabled && notification.IsCategoryEnabled(prefs.CategoryPreferences, event.EventType) {
					allNotificationsForEvent = append(allNotificationsForEvent, &notification.UserNotification{
						UserID:     targetUser.UserID,
						BusinessID: businessID,
						EventID:    event.ID,
						Channel:    notification.ChannelEmail,
						Status:     notification.StatusPending,
						Content:    content,
					})
				}

				// NOTE: Push/SMS not implemented in MVP, but we keep the structure
				// if prefs.PushEnabled && notification.IsCategoryEnabled(prefs.CategoryPreferences, event.EventType) {
				// 	allNotificationsForEvent = append(allNotificationsForEvent, &notification.UserNotification{
				// 		UserID:     targetUser.UserID,
				// 		BusinessID: businessID,
				// 		EventID:    event.ID,
				// 		Channel:    notification.ChannelPush,
				// 		Status:     notification.StatusPending,
				// 		Content:    content,
				// 	})
				// }
				//
				// if prefs.MessagesEnabled && notification.IsCategoryEnabled(prefs.CategoryPreferences, event.EventType) {
				// 	allNotificationsForEvent = append(allNotificationsForEvent, &notification.UserNotification{
				// 		UserID:     targetUser.UserID,
				// 		BusinessID: businessID,
				// 		EventID:    event.ID,
				// 		Channel:    "sms", // SMS channel
				// 		Status:     notification.StatusPending,
				// 		Content:    content,
				// 	})
				// }
			}

			// 6. Single batch insert for ALL notifications of this event (one DB round trip instead of N)
			if len(allNotificationsForEvent) > 0 {
				if err := txUserNotifRepo.CreateUserNotifications(ctx, allNotificationsForEvent); err != nil {
					s.logger.Errorf("Failed to create user notifications for event %d: %v", event.ID, err)
					// Don't mark event as processed if insert fails, will retry
					continue
				}
			}

			eventIDs = append(eventIDs, event.ID)
		}

		// 7. Delete events after fanout (keeps table tiny)
		// Events are deleted in same transaction, so if anything fails, tx rolls back
		if len(eventIDs) > 0 {
			if err := txEventRepo.DeleteEvents(ctx, eventIDs); err != nil {
				return fmt.Errorf("failed to delete events: %w", err)
			}
		}

		processedCount = len(eventIDs)
		return nil
	})

	if err != nil {
		return 0, err
	}

	return processedCount, nil
}

// getPreferencesWithCache retrieves user preferences using Redis cache with read-through pattern
// This implements the caching logic from the design document
func (s *FanoutService) getPreferencesWithCache(ctx context.Context, prefsRepo notification.NotificationPreferencesRepository, userID string) (*notification.NotificationPreferences, error) {
	cacheKey := keys.Notification.Preferences(userID)

	// 1. Try to get from cache
	cachedData, err := s.redisClient.Get(ctx, cacheKey).Result()
	if err == nil {
		// Cache hit
		var prefs notification.NotificationPreferences
		if err := json.Unmarshal([]byte(cachedData), &prefs); err == nil {
			return &prefs, nil
		}
		// If unmarshal fails, fall through to DB
		s.logger.Warnf("Failed to unmarshal cached preferences for user %s: %v", userID, err)
	}

	// 2. Cache miss: Get from DB
	prefs, err := prefsRepo.GetPreferences(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get preferences from DB: %w", err)
	}

	// 3. Set cache for next time (use TTL as fallback, explicit invalidation is primary)
	prefsJSON, err := json.Marshal(prefs)
	if err == nil {
		if err := s.redisClient.Set(ctx, cacheKey, prefsJSON, keys.WithJitter(keys.TTLNotificationPreferences)).Err(); err != nil {
			s.logger.Warnf("Failed to cache preferences for user %s: %v", userID, err)
		}
	}

	return prefs, nil
}

// getUsersByTarget retrieves users based on target info
// This uses the existing user and business repositories
func (s *FanoutService) getUsersByTarget(ctx context.Context, target notification.TargetInfo) ([]UserInfo, error) {
	switch target.Type {
	case notification.TargetUnicast:
		// Specific user IDs provided
		if len(target.IDs) == 0 {
			return nil, fmt.Errorf("unicast requires at least one user ID")
		}
		return s.getUsersByIDs(ctx, target.IDs)

	case notification.TargetMulticast:
		// Multiple users with optional role filter
		if target.MinimumRole != nil {
			return s.getUsersByBusinessAndRole(ctx, target.BusinessID, *target.MinimumRole)
		} else if len(target.IDs) > 0 {
			return s.getUsersByIDs(ctx, target.IDs)
		}
		return nil, fmt.Errorf("multicast requires either minimum_role or user IDs")

	case notification.TargetBroadcast:
		// All users with minimum role in business
		minRole := notification.RoleSupport // Default to all roles
		if target.MinimumRole != nil {
			minRole = *target.MinimumRole
		}
		return s.getUsersByBusinessAndRole(ctx, target.BusinessID, minRole)

	default:
		return nil, fmt.Errorf("invalid target type: %s", target.Type)
	}
}

// getUsersByIDs fetches user info by user IDs using the user repository (optimized: only selects id, email, name)
func (s *FanoutService) getUsersByIDs(ctx context.Context, userIDs []string) ([]UserInfo, error) {
	var users []UserInfo

	for _, userID := range userIDs {
		id, email, name, err := s.userRepo.GetUserBasicInfo(ctx, userID)
		if err != nil {
			s.logger.Warnf("Failed to get user %s: %v", userID, err)
			continue
		}

		users = append(users, UserInfo{
			UserID: id,
			Email:  email,
			Name:   name,
		})
	}

	return users, nil
}

// getUsersByBusinessAndRole fetches users from business_user_relation
// Filters by role hierarchy: owner > admin > finance > support
func (s *FanoutService) getUsersByBusinessAndRole(ctx context.Context, businessID, minimumRole string) ([]UserInfo, error) {
	// Get all users in the business using GetBusinessUsersWithDetails
	businessUsers, err := s.businessRelationRepo.GetBusinessUsersWithDetails(ctx, businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get business users: %w", err)
	}

	// Role hierarchy for filtering
	// Maps business roles to hierarchy levels (higher = more permissions)
	roleHierarchy := map[string]int{
		notification.RoleOwner:   4, // "owner"
		notification.RoleAdmin:   3, // "admin"
		"manager":                2, // Manager role from business module
		notification.RoleFinance: 2, // "finance" - same level as manager
		"auditor":                1, // Auditor role from business module
		notification.RoleSupport: 1, // "support" - same level as auditor
	}

	minRoleLevel := roleHierarchy[minimumRole]

	var users []UserInfo
	for _, bu := range businessUsers {
		// Filter by minimum role level
		userRoleLevel := roleHierarchy[string(bu.Role)]
		if userRoleLevel >= minRoleLevel {
			users = append(users, UserInfo{
				UserID: bu.UserID,
				Email:  bu.Email,
				Name:   bu.Name,
			})
		}
	}

	return users, nil
}

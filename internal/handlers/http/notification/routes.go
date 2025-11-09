package notification

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/notification"
	"xcentro.in/internal/middleware"
	"xcentro.in/internal/shared/ratelimiter"
)

// RegisterNotificationRoutes registers all notification endpoints
func RegisterNotificationRoutes(group *echo.Group, notifService notification.NotificationService, authService auth.AuthService) {
	// Create notification handler
	notifHandler := NewNotificationHandler(notifService)

	// Create notification group
	notifGroup := group.Group("/notifications")

	// Apply JWT middleware first, then rate limiter (so rate limiter can use user ID)
	notifGroup.Use(middleware.JWTMiddleware(authService))
	notifGroup.Use(ratelimiter.MiddlewareDefault())

	// Get user notifications (paginated)
	notifGroup.GET("", notifHandler.GetUserNotifications)

	// Get unread count
	notifGroup.GET("/unread/count", notifHandler.GetUnreadCount)

	// Mark single notification as read
	notifGroup.POST("/:id/read", notifHandler.MarkAsRead)

	// Mark multiple notifications as read
	notifGroup.POST("/read", notifHandler.MarkMultipleAsRead)

	// Get user notification preferences
	notifGroup.GET("/preferences", notifHandler.GetPreferences)

	// Update user notification preferences
	notifGroup.PUT("/preferences", notifHandler.UpdatePreferences)
}

package notification

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/notification"
	customValidator "xcentro.in/internal/shared/validator"
)

// NotificationHandler handles notification HTTP requests
// Handler is kept minimal - only input validation
// All business logic is in the service layer
type NotificationHandler struct {
	notificationService notification.NotificationService
}

// NewNotificationHandler creates a new notification handler
func NewNotificationHandler(notificationService notification.NotificationService) *NotificationHandler {
	return &NotificationHandler{
		notificationService: notificationService,
	}
}

// GetUserNotifications handles GET /v1/notifications
func (h *NotificationHandler) GetUserNotifications(c echo.Context) error {
	// Get user ID from JWT context
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if offset < 0 {
		offset = 0
	}

	// Get notifications
	notifications, err := h.notificationService.GetUserNotifications(c.Request().Context(), userID, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "FETCH_FAILED",
			"message": "Failed to fetch notifications",
		})
	}

	// Get unread count
	unreadCount, _ := h.notificationService.GetUnreadCount(c.Request().Context(), userID)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Notifications retrieved successfully",
		"data": map[string]any{
			"notifications": notifications,
			"unread_count":  unreadCount,
			"limit":         limit,
			"offset":        offset,
		},
	})
}

// GetUnreadCount handles GET /v1/notifications/unread/count
func (h *NotificationHandler) GetUnreadCount(c echo.Context) error {
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	count, err := h.notificationService.GetUnreadCount(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "FETCH_FAILED",
			"message": "Failed to get unread count",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Unread count retrieved successfully",
		"data": map[string]any{
			"unread_count": count,
		},
	})
}

// MarkAsRead handles POST /v1/notifications/:id/read
func (h *NotificationHandler) MarkAsRead(c echo.Context) error {
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	notificationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_NOTIFICATION_ID",
			"message": "Invalid notification ID",
		})
	}

	if err := h.notificationService.MarkAsRead(c.Request().Context(), notificationID, userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "MARK_READ_FAILED",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Notification marked as read",
	})
}

// MarkMultipleAsRead handles POST /v1/notifications/read
func (h *NotificationHandler) MarkMultipleAsRead(c echo.Context) error {
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	var req notification.MarkMultipleAsReadRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(http.StatusBadRequest, map[string]any{
				"error":   "VALIDATION_FAILED",
				"message": "Validation failed",
				"errors":  ve.Errors,
			})
		}
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "VALIDATION_FAILED",
			"message": "Validation failed",
		})
	}

	if err := h.notificationService.MarkMultipleAsRead(c.Request().Context(), req.NotificationIDs, userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "MARK_READ_FAILED",
			"message": "Failed to mark notifications as read",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Notifications marked as read",
		"count":   len(req.NotificationIDs),
	})
}

// GetPreferences handles GET /v1/notifications/preferences
func (h *NotificationHandler) GetPreferences(c echo.Context) error {
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	prefs, err := h.notificationService.GetPreferences(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "FETCH_FAILED",
			"message": "Failed to fetch preferences",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Preferences retrieved successfully",
		"data":    prefs,
	})
}

// UpdatePreferences handles PUT /v1/notifications/preferences
func (h *NotificationHandler) UpdatePreferences(c echo.Context) error {
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	var req notification.UpdatePreferencesRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	// Get current preferences
	prefs, err := h.notificationService.GetPreferences(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "FETCH_FAILED",
			"message": "Failed to fetch current preferences",
		})
	}

	// Update fields if provided
	if req.EmailEnabled != nil {
		prefs.EmailEnabled = *req.EmailEnabled
	}
	if req.PushEnabled != nil {
		prefs.PushEnabled = *req.PushEnabled
	}
	if req.MessagesEnabled != nil {
		prefs.MessagesEnabled = *req.MessagesEnabled
	}
	if req.CategoryPreferences != nil {
		prefs.CategoryPreferences = req.CategoryPreferences
	}

	// Save updated preferences
	if err := h.notificationService.UpdatePreferences(c.Request().Context(), prefs); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "UPDATE_FAILED",
			"message": "Failed to update preferences",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Preferences updated successfully",
		"data":    prefs,
	})
}

// TriggerManualNotification handles POST /v1/ops/notifications/trigger (ops only)
func (h *NotificationHandler) TriggerManualNotification(c echo.Context) error {
	var payload notification.NotificationEventPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	if err := h.notificationService.TriggerManualNotification(c.Request().Context(), payload); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "TRIGGER_FAILED",
			"message": "Failed to trigger notification",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Notification triggered successfully",
	})
}

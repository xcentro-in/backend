package notification

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	coreNotification "xcentro.in/internal/core/notification"
	"xcentro.in/internal/middleware"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/validator"
)

// OpsNotificationHandler handles ops dashboard notification-related endpoints
type OpsNotificationHandler struct {
	notificationService coreNotification.NotificationService
	authService         auth.AuthService
	logger              *logger.Logger
}

// NewOpsNotificationHandler creates a new ops notification handler instance
func NewOpsNotificationHandler(notificationService coreNotification.NotificationService, authService auth.AuthService) *OpsNotificationHandler {
	return &OpsNotificationHandler{
		notificationService: notificationService,
		authService:         authService,
		logger:              logger.GetDefault(),
	}
}

// RegisterOpsNotificationRoutes registers all ops dashboard notification routes
func RegisterOpsNotificationRoutes(e *echo.Group, notificationService coreNotification.NotificationService, cookieManager *httputil.CookieManager) {
	handler := NewOpsNotificationHandler(notificationService, nil)

	notificationGroup := e.Group("/notification")

	// POST routes (require CSRF middleware)
	notificationGroup.Use(middleware.CSRFMiddleware(cookieManager))
	notificationGroup.POST("/trigger", handler.TriggerNotifications)
}

// TriggerNotifications handles POST /notifications/trigger
func (h *OpsNotificationHandler) TriggerNotifications(c echo.Context) error {
	// Parse request body
	var req coreNotification.OpsBulkTriggerRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Trim and validate
	validator.TrimStrings(&req)
	if err := c.Validate(&req); err != nil {
		if ve, ok := err.(*validator.ValidationError); ok {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error":  "Validation failed",
				"errors": ve.Errors,
			})
		}
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	// Additional validation for target types
	for i, target := range req.Targets {
		switch target.Type {
		case coreNotification.TargetUnicast:
			// Unicast must have exactly one ID
			if len(target.IDs) != 1 {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Unicast target must have exactly one user ID",
					"index": string(rune(i)),
				})
			}
		case coreNotification.TargetMulticast:
			// Multicast must have either IDs or MinimumRole, but not both
			hasIDs := len(target.IDs) > 0
			hasRole := target.MinimumRole != nil && *target.MinimumRole != ""
			if hasIDs && hasRole {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Multicast target cannot have both IDs and MinimumRole",
					"index": string(rune(i)),
				})
			}
			if !hasIDs && !hasRole {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Multicast target must have either IDs or MinimumRole",
					"index": string(rune(i)),
				})
			}
		case coreNotification.TargetBroadcast:
			// Broadcast optionally has MinimumRole (nil means all users)
			// No additional validation needed for broadcast
		default:
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Invalid target type",
				"index": string(rune(i)),
			})
		}
	}

	// Call service layer
	response, err := h.notificationService.BulkTriggerNotifications(c.Request().Context(), &req)
	if err != nil {
		h.logger.Errorf("Error triggering bulk notifications: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to trigger notifications",
		})
	}

	return c.JSON(http.StatusOK, response)
}

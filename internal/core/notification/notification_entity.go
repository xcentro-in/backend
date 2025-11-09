package notification

import "time"

// #############################################################################
// # CONSTANTS (Primitives - No Dependencies)
// #############################################################################

// Common notification event types (for reference and type safety)
// Format: category.action.details where category matches CategoryPreferences fields
const (
	// Payments category (payments)
	EventPaymentApprovalRequired = "payments.approval.required"
	EventPaymentApproved         = "payments.approved"
	EventPaymentRejected         = "payments.rejected"
	EventPaymentCompleted        = "payments.completed"
	EventPaymentFailed           = "payments.failed"
	EventPaymentRefunded         = "payments.refunded"

	// Payments category - settlements
	EventSettlementReceived = "payments.settlement.received"
	EventSettlementFailed   = "payments.settlement.failed"

	// Payees category (payees)
	EventPayeeVerificationFailed = "payees.verification.failed"
	EventPayeeVerified           = "payees.verified"

	// Payments category - payouts
	EventPayoutCompleted = "payments.payout.completed"
	EventPayoutFailed    = "payments.payout.failed"

	// Insights category (insights)
	EventReportReady = "insights.report.ready"

	// Announcements category (announcements)
	EventSystemMaintenance = "announcements.maintenance"
	EventSystemAlert       = "announcements.alert"

	// Security category (security)
	EventUserInvited     = "security.user.invited"
	EventUserRoleChanged = "security.user.role.changed"

	// Approvals category (approvals)
	EventVoucherApprovalRequested = "approvals.voucher.requested"
	EventVoucherApprovalApproved  = "approvals.voucher.approved"
	EventVoucherApprovalRejected  = "approvals.voucher.rejected"
)

// Notification channels
const (
	ChannelEmail = "email"
	ChannelPush  = "push"
	ChannelWeb   = "web"
)

// Notification statuses
const (
	StatusPending    = "pending"
	StatusProcessing = "processing" // Intermediate status to prevent duplicate processing
	StatusSent       = "sent"
	StatusFailed     = "failed"
	StatusRead       = "read"
)

// Target types
const (
	TargetUnicast   = "unicast"   // Single specific user
	TargetMulticast = "multicast" // Multiple specific users (with optional role filter)
	TargetBroadcast = "broadcast" // All users with minimum role in business
)

// User roles (for minimum_role filtering)
const (
	RoleOwner   = "owner"
	RoleAdmin   = "admin"
	RoleFinance = "finance"
	RoleSupport = "support"
)

// Action button types (for standardized UI rendering)
const (
	ButtonTypePositive = "button-positive" // Green/primary button (Approve, Accept, Confirm)
	ButtonTypeNegative = "button-negative" // Red/destructive button (Reject, Deny, Cancel)
	ButtonTypeNeutral  = "button-neutral"  // Gray/secondary button (View, Details, Maybe Later)
	ButtonTypeLink     = "link"            // Text link style (Learn More, View Report)
)

// #############################################################################
// # BASIC TYPES (Level 1 - Depends only on constants)
// #############################################################################

// ActionButton represents a clickable action in the notification
type ActionButton struct {
	Type string `json:"type" validate:"required,oneof=button-positive button-negative button-neutral link"` // Button style
	Text string `json:"text" validate:"required,max=30"`                                                    // Button text
	Href string `json:"href" validate:"required"`                                                           // Relative URL or deep link
}

// MessageContent represents the notification message
type MessageContent struct {
	Title string `json:"title" validate:"required,max=100"` // Short title for push/email subject
	Body  string `json:"body" validate:"required,max=500"`  // Main message content
}

// EntityReferences contains IDs of entities related to this notification
type EntityReferences struct {
	BusinessID *string `json:"business_id,omitempty"` // Optional: may be redundant with target
	PaymentID  *string `json:"payment_id,omitempty"`  // e.g., "pay_12345"
	PayeeID    *string `json:"payee_id,omitempty"`    // e.g., "payee_6789"
	VoucherID  *string `json:"voucher_id,omitempty"`  // e.g., "voucher_456"
	UserID     *string `json:"user_id,omitempty"`     // e.g., "user_123"
	ReportID   *string `json:"report_id,omitempty"`   // e.g., "report_789"
}

// TargetInfo specifies who should receive this notification
type TargetInfo struct {
	BusinessID  string   `json:"business_id" validate:"required"`                                               // Required: Which business this notification is for
	Type        string   `json:"type" validate:"required,oneof=unicast multicast broadcast"`                    // "unicast", "multicast", or "broadcast"
	MinimumRole *string  `json:"minimum_role,omitempty" validate:"omitempty,oneof=owner admin finance support"` // For multicast/broadcast: minimum role required
	IDs         []string `json:"ids,omitempty" validate:"omitempty,dive,required"`                              // For unicast/multicast: specific user IDs
}

// CategoryPreferences represents the structured JSONB for category_preferences
// This is a helper type for type-safe access to category preferences
type CategoryPreferences struct {
	Announcements bool `json:"announcements"`
	Rewards       bool `json:"rewards"`
	Payments      bool `json:"payments"`
	Payees        bool `json:"payees"`
	Approvals     bool `json:"approvals"`
	Security      bool `json:"security"`
	Insights      bool `json:"insights"`
}

// GetEventCategory extracts the category from event type (first segment before dot)
// Returns empty string if event type doesn't follow category.* format
func GetEventCategory(eventType string) string {
	if eventType == "" {
		return ""
	}
	// Find first dot
	for i := 0; i < len(eventType); i++ {
		if eventType[i] == '.' {
			return eventType[:i]
		}
	}
	return eventType // No dot found, return entire string
}

// IsCategoryEnabled checks if a category is enabled in user preferences
func IsCategoryEnabled(categoryPrefs map[string]interface{}, eventType string) bool {
	category := GetEventCategory(eventType)
	if category == "" {
		return true // Unknown category, allow by default
	}

	// Check if category exists and is enabled in preferences
	if enabled, ok := categoryPrefs[category]; ok {
		if boolVal, ok := enabled.(bool); ok {
			return boolVal
		}
	}

	return true // Default to enabled if category not found
}

// #############################################################################
// # COMPOSITE TYPES (Level 2 - Depends on basic types)
// #############################################################################

// NotificationEventPayload represents the JSON schema for notification_events.payload
// This contains everything needed to build and send a notification to users.
type NotificationEventPayload struct {
	Timestamp string           `json:"timestamp" validate:"required"` // RFC3339 timestamp - source of truth for when event occurred
	EventType string           `json:"event_type" validate:"required"`
	Target    TargetInfo       `json:"target" validate:"required"`
	Entities  EntityReferences `json:"entities,omitempty"`
	Message   MessageContent   `json:"message" validate:"required"`
	Actions   []ActionButton   `json:"actions,omitempty" validate:"omitempty,dive"`
}

// UpdatePreferencesRequest represents the request to update user notification preferences
type UpdatePreferencesRequest struct {
	EmailEnabled        *bool                  `json:"email_enabled,omitempty"`
	PushEnabled         *bool                  `json:"push_enabled,omitempty"`
	MessagesEnabled     *bool                  `json:"messages_enabled,omitempty"`
	CategoryPreferences map[string]interface{} `json:"category_preferences,omitempty"`
}

// MarkMultipleAsReadRequest represents the request to mark multiple notifications as read
type MarkMultipleAsReadRequest struct {
	NotificationIDs []int64 `json:"notification_ids" validate:"required,min=1,dive,gt=0"`
}

// #############################################################################
// # MAIN ENTITIES (Level 3 - Database tables, depends on composite types)
// #############################################################################

// NotificationPreferences represents a row in the notification_preferences table
type NotificationPreferences struct {
	UserID              string                 `db:"user_id" json:"user_id"`
	EmailEnabled        bool                   `db:"email_enabled" json:"email_enabled"`
	PushEnabled         bool                   `db:"push_enabled" json:"push_enabled"`
	MessagesEnabled     bool                   `db:"messages_enabled" json:"messages_enabled"`
	CategoryPreferences map[string]interface{} `db:"category_preferences" json:"category_preferences"` // JSONB: {announcements, rewards, payments, payees, approvals, security, insights}
	CreatedAt           time.Time              `db:"created_at" json:"created_at"`
	UpdatedAt           *time.Time             `db:"updated_at" json:"updated_at,omitempty"`
}

// NotificationEvent represents a row in the notification_events table (queue)
type NotificationEvent struct {
	ID          int64                    `db:"id" json:"id"`
	ProcessedAt *time.Time               `db:"processed_at" json:"processed_at,omitempty"`
	EventType   string                   `db:"event_type" json:"event_type"`
	Payload     NotificationEventPayload `db:"payload" json:"payload"` // JSONB field
	CreatedAt   time.Time                `db:"created_at" json:"created_at"`
}

// UserNotification represents a row in the user_notifications table (user inbox)
type UserNotification struct {
	ID         int64                  `db:"id" json:"id"`
	UserID     string                 `db:"user_id" json:"user_id"`
	BusinessID *string                `db:"business_id" json:"business_id,omitempty"`
	EventID    int64                  `db:"event_id" json:"event_id"`
	Channel    string                 `db:"channel" json:"channel"`     // "email", "push", "web"
	Status     string                 `db:"status" json:"status"`       // "pending", "processing", "sent", "failed", "read"
	Content    map[string]interface{} `db:"content" json:"content"`     // JSONB: rendered message
	RetryCount int                    `db:"retry_count" json:"retry_count"` // Number of retry attempts
	CreatedAt  time.Time              `db:"created_at" json:"created_at"`
	SentAt     *time.Time             `db:"sent_at" json:"sent_at,omitempty"`
	ReadAt     *time.Time             `db:"read_at" json:"read_at,omitempty"`
}

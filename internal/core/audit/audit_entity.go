package audit

import "time"

// #############################################################################
// # CONSTANTS (Primitives - No Dependencies)
// #############################################################################

// Common event types (for reference and type safety)
const (
	// Payment events
	EventPaymentInitiated      = "payment.initiated"
	EventPaymentApproved       = "payment.approved"
	EventPaymentRejected       = "payment.rejected"
	EventPaymentCompleted      = "payment.completed"
	EventPaymentFailed         = "payment.failed"
	EventPaymentCancelled      = "payment.cancelled"
	EventPaymentRefunded       = "payment.refund.initiated"
	EventPaymentRefundComplete = "payment.refund.completed"

	// Settlement events
	EventSettlementReceived = "settlement.received"
	EventSettlementFailed   = "settlement.failed"

	// Payee events
	EventPayeeAdded              = "payee.added"
	EventPayeeUpdated            = "payee.updated"
	EventPayeeVerified           = "payee.verified"
	EventPayeeVerificationFailed = "payee.verification.failed"

	// Voucher events
	EventVoucherCreated   = "voucher.created"
	EventVoucherFinalized = "voucher.finalized"
	EventVoucherPosted    = "voucher.posted"

	// Payout events
	EventPayoutInitiated = "payout.initiated"
	EventPayoutCompleted = "payout.completed"
	EventPayoutFailed    = "payout.failed"

	// Business events
	EventBusinessCreated = "business.created"
	EventBusinessUpdated = "business.updated"
	EventUserInvited     = "business.user.invited"
	EventUserRoleChanged = "business.user.role.changed"
	EventUserRemoved     = "business.user.removed"

	// User events
	EventUserSignup       = "user.signup"
	EventUserLogin        = "user.login"
	EventUserKYCSubmitted = "user.kyc.submitted"
	EventUserKYCApproved  = "user.kyc.approved"
	EventUserKYCRejected  = "user.kyc.rejected"
)

// #############################################################################
// # BASIC TYPES (Level 1 - Depends only on constants)
// #############################################################################

// ChangeLogEntry represents a single field change
type ChangeLogEntry struct {
	Field    string `json:"field" validate:"required"` // Field name that changed
	OldValue any    `json:"old_value"`                 // Previous value (can be null)
	NewValue any    `json:"new_value"`                 // New value (can be null)
}

// EntityReferences contains IDs of all relevant entities for this event
type EntityReferences struct {
	BusinessID string  `json:"business_id" validate:"required"` // Required: every event belongs to a business
	PaymentID  *string `json:"payment_id,omitempty"`            // Optional: e.g., "pay_12345"
	PayeeID    *string `json:"payee_id,omitempty"`              // Optional: e.g., "payee_6789"
	VoucherID  *string `json:"voucher_id,omitempty"`            // Optional: e.g., "voucher_456"
	UserID     *string `json:"user_id,omitempty"`               // Optional: e.g., "user_123"
	WalletID   *string `json:"wallet_id,omitempty"`             // Optional: e.g., "wallet_789"
}

// ActorInfo represents who performed the action
type ActorInfo struct {
	Type      string  `json:"type" validate:"required,oneof=user system"`   // "user" or "system"
	ID        string  `json:"id" validate:"required"`                       // user_id or "system"
	Name      string  `json:"name" validate:"required"`                     // "Michael Chen (Financial Controller)" or "System"
	IPAddress *string `json:"ip_address,omitempty" validate:"omitempty,ip"` // Optional IP address (for user actions)
}

// #############################################################################
// # COMPOSITE TYPES (Level 2 - Depends on basic types)
// #############################################################################

// AuditEventPayload represents the JSON schema for audit_events.payload
// This is a minimal, factual record of what happened in the system.
type AuditEventPayload struct {
	Timestamp string           `json:"timestamp" validate:"required"` // RFC3339 timestamp - source of truth for when event occurred
	EventType string           `json:"event_type" validate:"required"`
	Actor     ActorInfo        `json:"actor" validate:"required"`
	Entities  EntityReferences `json:"entities" validate:"required"`
	Note      *string          `json:"note,omitempty"`
	ChangeLog []ChangeLogEntry `json:"change_log,omitempty" validate:"omitempty,dive"`
}

// #############################################################################
// # MAIN ENTITIES (Level 3 - Database tables, depends on composite types)
// #############################################################################

// AuditEvent represents a row in the audit_events table (queue)
type AuditEvent struct {
	ID          int64             `db:"id" json:"id"`
	ProcessedAt *time.Time        `db:"processed_at" json:"processed_at,omitempty"`
	EventType   string            `db:"event_type" json:"event_type"`
	Payload     AuditEventPayload `db:"payload" json:"payload"` // JSONB field
	CreatedAt   time.Time         `db:"created_at" json:"created_at"`
}

// AuditTrailEntry represents a row in the audit_trail table (permanent log)
// This is the normalized design - the event is stored once, links via audit_trail_entities
type AuditTrailEntry struct {
	ID             int64          `db:"id" json:"id"`
	BusinessID     string         `db:"business_id" json:"business_id"`
	EventTimestamp time.Time      `db:"event_timestamp" json:"event_timestamp"`
	EventType      string         `db:"event_type" json:"event_type"` // Human-readable title
	ActorType      string         `db:"actor_type" json:"actor_type"` // "user" or "system"
	ActorID        string         `db:"actor_id" json:"actor_id"`     // user_id UUID or "system"
	ActorName      string         `db:"actor_name" json:"actor_name"` // "Michael Chen (Financial Controller)"
	Note           *string        `db:"note" json:"note,omitempty"`
	Details        map[string]any `db:"details" json:"details,omitempty"`       // JSONB: {ip_address, change_log, etc.}
	SourceEventID  int64          `db:"source_event_id" json:"source_event_id"` // FK to audit_events.id
}

// AuditTrailEntity represents a row in the audit_trail_entities table (entity links)
// This table creates the many-to-many relationship between events and entities
type AuditTrailEntity struct {
	ID            int64  `db:"id" json:"id"`
	BusinessID    string `db:"business_id" json:"business_id"`
	AuditTrailID  int64  `db:"audit_trail_id" json:"audit_trail_id"`
	EntityType    string `db:"entity_type" json:"entity_type"` // 'payment', 'voucher', 'payee', 'user', 'wallet', 'payout'
	EntityID      string `db:"entity_id" json:"entity_id"`     // UUID of the entity
}

// Entity type constants for audit_trail_entities
const (
	EntityTypePayment = "payment"
	EntityTypeVoucher = "voucher"
	EntityTypePayee   = "payee"
	EntityTypeUser    = "user"
	EntityTypeWallet  = "wallet"
	EntityTypePayout  = "payout"
)

# Notification Button Types Usage Guide

This guide shows how to use the standardized button types when triggering notifications.

## Available Button Types

All button type constants are defined in `notification_entity.go`:

```go
const (
    ButtonTypePositive = "button-positive" // Green/primary button
    ButtonTypeNegative = "button-negative" // Red/destructive button
    ButtonTypeNeutral  = "button-neutral"  // Gray/secondary button
    ButtonTypeLink     = "link"            // Text link style
)
```

## Usage Examples

### Example 1: Approval Required (Positive + Negative)

```go
import "xcentro.in/internal/core/notification"

paymentID := "pay_123"
notification.Trigger(ctx, notification.NotificationEventPayload{
    EventType: notification.EventPaymentApprovalRequired,
    Target: notification.TargetInfo{
        BusinessID: businessID,
        Type: notification.TargetMulticast,
        MinimumRole: &notification.RoleFinance,
    },
    Entities: notification.EntityReferences{
        PaymentID: &paymentID,
    },
    Message: notification.MessageContent{
        Title: "Approval Required",
        Body: "John submitted a payment of ₹50,000 for approval",
    },
    Actions: []notification.ActionButton{
        {
            Type: notification.ButtonTypePositive, // ← Green approve button
            Text: "Approve",
            Href: "/payments/pay_123/approve",
        },
        {
            Type: notification.ButtonTypeNegative, // ← Red reject button
            Text: "Reject",
            Href: "/payments/pay_123/reject",
        },
    },
})
```

### Example 2: Payment Completed (Neutral View)

```go
notification.Trigger(ctx, notification.NotificationEventPayload{
    EventType: notification.EventPaymentCompleted,
    Target: notification.TargetInfo{
        BusinessID: businessID,
        Type: notification.TargetUnicast,
        IDs: []string{userID},
    },
    Entities: notification.EntityReferences{
        PaymentID: &paymentID,
    },
    Message: notification.MessageContent{
        Title: "Payment Successful",
        Body: "Your payment of ₹50,000 has been processed successfully",
    },
    Actions: []notification.ActionButton{
        {
            Type: notification.ButtonTypeNeutral, // ← Gray view details button
            Text: "View Details",
            Href: "/payments/pay_123",
        },
    },
})
```

### Example 3: Report Ready (Link Style)

```go
reportID := "report_456"
notification.Trigger(ctx, notification.NotificationEventPayload{
    EventType: notification.EventReportReady,
    Target: notification.TargetInfo{
        BusinessID: businessID,
        Type: notification.TargetUnicast,
        IDs: []string{userID},
    },
    Entities: notification.EntityReferences{
        ReportID: &reportID,
    },
    Message: notification.MessageContent{
        Title: "Monthly Report Ready",
        Body: "Your monthly financial report for December 2024 is ready to download",
    },
    Actions: []notification.ActionButton{
        {
            Type: notification.ButtonTypeLink, // ← Text link style
            Text: "Download Report",
            Href: "/reports/report_456/download",
        },
    },
})
```

### Example 4: System Alert (Multiple Actions)

```go
notification.Trigger(ctx, notification.NotificationEventPayload{
    EventType: notification.EventSystemAlert,
    Target: notification.TargetInfo{
        BusinessID: businessID,
        Type: notification.TargetBroadcast,
        MinimumRole: &notification.RoleOwner,
    },
    Message: notification.MessageContent{
        Title: "Action Required: Update Payment Method",
        Body: "Your payment method expires in 5 days. Please update it to avoid service interruption.",
    },
    Actions: []notification.ActionButton{
        {
            Type: notification.ButtonTypePositive, // ← Green primary action
            Text: "Update Now",
            Href: "/settings/billing/payment-method",
        },
        {
            Type: notification.ButtonTypeNeutral, // ← Gray secondary action
            Text: "Remind Me Later",
            Href: "/settings/billing/remind",
        },
    },
})
```

## Frontend Implementation

The frontend should map these button types to consistent styles:

| Button Type | Recommended Style | Use Cases |
|------------|------------------|-----------|
| `button-positive` | Green/Primary | Approve, Accept, Confirm, Continue, Submit |
| `button-negative` | Red/Destructive | Reject, Deny, Cancel, Delete, Remove |
| `button-neutral` | Gray/Secondary | View, Details, Maybe Later, Skip, Dismiss |
| `link` | Text Link | Learn More, View Report, Read More, See Details |

## Validation

The `ActionButton` type includes validation:

```go
type ActionButton struct {
    Type string `json:"type" validate:"required,oneof=button-positive button-negative button-neutral link"`
    Text string `json:"text" validate:"required,max=30"`
    Href string `json:"href" validate:"required"`
}
```

Invalid button types will be rejected at the validation layer.

## Best Practices

1. **Use constants, not strings**: Always use `notification.ButtonTypePositive` instead of `"button-positive"`
2. **Limit actions**: Keep to 1-3 action buttons per notification for better UX
3. **Primary action first**: Put the most important action (usually positive) first
4. **Clear text**: Keep button text short and action-oriented (max 30 chars)
5. **Relative URLs**: Use relative paths in `Href` for easier environment management

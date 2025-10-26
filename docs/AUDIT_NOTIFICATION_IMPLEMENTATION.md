# Audit & Notification System Implementation

This document provides an overview of the audit trail and notification system implementation based on the Phase 1 PostgreSQL-only design.

## 🏗️ Architecture Overview

The system follows Clean Architecture principles with clear separation of concerns:

**Layered Responsibilities:**

1. **Handlers** (HTTP layer): Minimal code
   - Extract parameters from request (URL params, query params, headers)
   - Get `userId` from context (injected by JWT middleware)
   - Validate input
   - Call service methods
   - Return HTTP responses

2. **Workers** (Background jobs): Minimal code
   - Call single service method with batch configuration
   - Log progress and errors
   - No business logic or transaction management

3. **Services** (Business logic): Own all orchestration
   - Manage transactions via `txManager.WithTx()`
   - Orchestrate multiple repository calls
   - Transform data between layers
   - Handle external API calls (email, push notifications)
   - Make business decisions
   - **Key Pattern**: Email/push sending happens AFTER DB transaction commits

4. **Repositories** (Data access): Pure data operations
   - Implement `WithTx(tx)` methods for transactional operations
   - Receive DB/TX client from caller (service)
   - Execute SQL queries
   - No business logic

**Transaction Pattern:**
- Services receive `txManager` via dependency injection
- Services call `txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error { ... })`
- Inside transaction, services use `repo.WithTx(tx)` to get transactional repository instances
- FOR UPDATE SKIP LOCKED is always executed within service-controlled transactions
- External API calls (email/push) happen AFTER transaction commits

**Design Principle**: Services own all orchestration logic. Handlers and workers remain thin and delegate to services.

## 📁 File Structure

### Core Domain (Entities & Interfaces)
```
internal/core/
├── audit/
│   ├── audit_entity.go          # Entities: AuditEvent, AuditTrailEntry
│   └── audit_repository.go      # Repository interfaces (with WithTx methods)
└── notification/
    ├── notification_entity.go   # Entities: NotificationEvent, UserNotification
    └── notification_repository.go  # Repository interfaces (with WithTx methods)
```

### Repository Adapters (PostgreSQL)
```
internal/adapters/database/postgres/
├── audit/
│   ├── audit_event_repository.go    # Queue operations (FOR UPDATE SKIP LOCKED) + WithTx
│   └── audit_trail_repository.go    # Permanent trail operations + WithTx
└── notification/
    ├── notification_event_repository.go          # Queue operations + WithTx
    ├── user_notification_repository.go           # User inbox operations + WithTx
    └── notification_preferences_repository.go    # User preferences + WithTx
```

### Services (Business Logic)
```
internal/services/
├── audit/
│   └── audit_service.go         # GetTrailByBusinessID, GetTrailByPaymentID
└── notification/
    ├── notification_service.go  # User notifications, preferences, manual triggers
    └── helpers.go               # Email sending, user resolution helpers
```

### Workers (Background Processing)
```
internal/workers/
├── audit_worker.go              # Processes audit events queue
└── notification_worker.go       # Processes notification events queue

cmd/workers/
├── audit_worker/
│   └── main.go                  # Audit worker entry point
└── notification_worker/
    └── main.go                  # Notification worker entry point
```

### HTTP Handlers (API Endpoints)
```
internal/handlers/http/
├── audit/
│   └── audit_handler.go         # GET /audit/business/:id, /audit/payment/:id
└── notification/
    └── notification_handler.go  # User notifications & preferences endpoints
```

### Configuration (Shared Config System)
```
internal/shared/config/
└── config.go                    # WorkersConfig, AuditWorkerConfig, NotificationWorkerConfig
```

All worker configuration is managed through the centralized config system, automatically loading from environment variables.

## 🚀 Workers

### Audit Worker
**Entry Point:** `cmd/workers/audit_worker/main.go`

**Configuration:**
Worker configuration is automatically loaded from environment variables via the shared config system. See `internal/shared/config/config.go` for details.

```bash
AUDIT_WORKER_BATCH_SIZE=100              # Number of events to process per cycle
AUDIT_WORKER_POLL_INTERVAL_SECONDS=2     # Polling interval in seconds
```

**Build & Run:**
```bash
go build -o bin/audit_worker ./cmd/workers/audit_worker
./bin/audit_worker
```

**Architecture:**
- **Worker** (`internal/workers/audit_worker.go`): Minimal - calls `auditService.ProcessPendingAuditEvents()`
- **Service** (`internal/services/audit/audit_service.go`): Orchestrates all logic within transaction:
  1. Begin transaction
  2. Fetch pending events (FOR UPDATE SKIP LOCKED within tx)
  3. Transform events to audit trail entries
  4. Batch insert into `audit_trail` (ON CONFLICT DO NOTHING)
  5. Mark events as processed
  6. Commit transaction (auto-rollback on error)

**Transaction Safety:**
- Single transaction wraps entire batch processing
- FOR UPDATE SKIP LOCKED prevents race conditions between workers
- ON CONFLICT ensures idempotency
- Service orchestrates transaction via `txManager.WithTx()`
- Repositories receive transaction client from service

### Notification Worker
**Entry Point:** `cmd/workers/notification_worker/main.go`

**Configuration:**
Worker configuration is automatically loaded from environment variables via the shared config system. See `internal/shared/config/config.go` for details.

```bash
NOTIFICATION_WORKER_BATCH_SIZE=50               # Number of events to process per cycle
NOTIFICATION_WORKER_POLL_INTERVAL_SECONDS=2     # Polling interval in seconds
NOTIFICATION_WORKER_MAX_RETRIES=3               # Max retry attempts for sending
```

**Build & Run:**
```bash
go build -o bin/notification_worker ./cmd/workers/notification_worker
./bin/notification_worker
```

**Architecture:**
- **Worker** (`internal/workers/notification_worker.go`): Minimal - calls `notificationService.ProcessPendingNotificationEvents()`
- **Service** (`internal/services/notification/notification_service.go`): Orchestrates all logic:

  **Phase 1: DB Operations (within transaction)**
  1. Begin transaction
  2. Fetch pending events (FOR UPDATE SKIP LOCKED within tx)
  3. Resolve target users (unicast/multicast/broadcast) - **MVP_TODO: Implement getUsersByTarget()**
  4. Create entries in `user_notifications` table (single entry per user)
  5. Mark events as processed
  6. Commit transaction

  **Phase 2: Email/Push Sending (AFTER transaction commit)**
  7. Send emails via AWS SES with exponential backoff retry
  8. Send push notifications (placeholder for MVP)
  9. Respect user notification preferences

**Channel Support:**
- ✅ **Email**: Integrated with AWS SES (sent after DB commit)
- ⏳ **Push**: Placeholder (PHASE2_TODO: Implement FCM/APNS)
- ✅ **Web**: Stored in `user_notifications` table

**Transaction Safety:**
- Single transaction wraps all DB operations
- Email/push sending happens AFTER commit (prevents blocking transaction)
- FOR UPDATE SKIP LOCKED prevents race conditions
- ON CONFLICT (event_id, user_id, channel) DO NOTHING ensures idempotency
- Service orchestrates transaction via `txManager.WithTx()`
- Repositories receive transaction client from service

## 📡 API Endpoints

**Authentication & Authorization:**
- JWT middleware handles authentication (extracts and validates token)
- Role middleware handles authorization (checks user's role in business)
- Handlers receive `userId` from `c.Get("userId")` (injected by JWT middleware)
- Handlers receive `X-Business-ID` header for business context
- No manual JWT parsing in handlers

### Audit Trail Endpoints

#### Get Business Audit Trail
```
GET /v1/audit/business/:business_id?limit=50&offset=0
Headers:
  Authorization: Bearer <JWT>  # JWT middleware validates and injects userId
```

**Handler Flow:**
1. JWT middleware validates token and injects `userId` into context
2. Handler extracts `business_id` from URL params
3. Handler calls `auditService.GetTrailByBusinessID()`

#### Get Payment Audit Trail
```
GET /v1/audit/payment/:payment_id
Headers:
  Authorization: Bearer <JWT>  # JWT middleware validates and injects userId
  X-Business-ID: <business_uuid>
```

**Handler Flow:**
1. JWT middleware validates token and injects `userId` into context
2. Handler extracts `payment_id` from URL params
3. Handler extracts `business_id` from `X-Business-ID` header
4. Handler calls `auditService.GetTrailByPaymentID()`

### Notification Endpoints (User)

#### Get User Notifications
```
GET /v1/notifications?limit=50&offset=0
Headers: Authorization: Bearer <JWT>

Response:
{
  "message": "Notifications retrieved successfully",
  "data": {
    "notifications": [...],
    "unread_count": 5,
    "limit": 50,
    "offset": 0
  }
}
```

#### Get Unread Count
```
GET /v1/notifications/unread/count
Headers: Authorization: Bearer <JWT>
```

#### Mark Single Notification as Read
```
POST /v1/notifications/:id/read
Headers: Authorization: Bearer <JWT>
```

#### Mark Multiple Notifications as Read
```
POST /v1/notifications/read
Headers: Authorization: Bearer <JWT>
Body:
{
  "notification_ids": [1, 2, 3, 4, 5]
}
```

#### Get Notification Preferences
```
GET /v1/notifications/preferences
Headers: Authorization: Bearer <JWT>
```

#### Update Notification Preferences
```
PUT /v1/notifications/preferences
Headers: Authorization: Bearer <JWT>
Body:
{
  "email_enabled": true,
  "push_enabled": false,
  "messages_enabled": true,
  "category_preferences": {
    "announcements": true,
    "payments": true,
    "payees": false
  }
}
```

### Notification Endpoints (Ops/Campaign)

#### Trigger Manual Notification
```
POST /v1/ops/notifications/trigger
Headers: Authorization: Bearer <JWT> (ops role required)
Body:
{
  "event_type": "payment.completed",
  "target": {
    "business_id": "bus_123",
    "type": "broadcast",
    "minimum_role": "finance"
  },
  "message": {
    "title": "Payment Completed",
    "body": "Your payment of ₹10,000 was successful"
  },
  "actions": [
    {
      "type": "button-positive",
      "text": "View Details",
      "href": "/payments/pay_123"
    }
  ],
  "entities": {
    "payment_id": "pay_123"
  }
}
```

**Target Types:**
- `unicast`: Send to specific user IDs (provide `ids` array)
- `multicast`: Send to multiple users with optional role filter
- `broadcast`: Send to all users in business with minimum role

## 🔧 Integration Guide

### Creating Audit Events (From Your Code)

```go
import (
    "xcentro.in/internal/core/audit"
)

// In your transaction:
auditEvent := &audit.AuditEvent{
    EventType: audit.EventPaymentCompleted,
    Payload: audit.AuditEventPayload{
        Timestamp: time.Now().UTC().Format(time.RFC3339),
        EventType: audit.EventPaymentCompleted,
        Actor: audit.ActorInfo{
            Type: "user",
            ID:   userID,
            Name:  userName,
            IPAddress: &ipAddr,
        },
        Entities: audit.EntityReferences{
            BusinessID: &businessID,
            PaymentID:  &paymentID,
        },
        Note: stringPtr("Payment processed successfully"),
    },
}

err := auditEventRepo.CreateEvent(ctx, auditEvent)
```

### Creating Notification Events (From Your Code)

```go
import (
    "xcentro.in/internal/core/notification"
)

// In your transaction:
notifEvent := &notification.NotificationEvent{
    EventType: notification.EventPaymentCompleted,
    Payload: notification.NotificationEventPayload{
        Timestamp: time.Now().UTC().Format(time.RFC3339),
        EventType: notification.EventPaymentCompleted,
        Target: notification.TargetInfo{
            BusinessID:  businessID,
            Type:        notification.TargetUnicast,
            IDs:         []string{userID},
        },
        Message: notification.MessageContent{
            Title: "Payment Successful",
            Body:  "Your payment of ₹10,000 has been completed",
        },
        Actions: []notification.ActionButton{
            {
                Type: "button-positive",
                Text: "View Receipt",
                Href: "/payments/" + paymentID,
            },
        },
        Entities: notification.EntityReferences{
            PaymentID: &paymentID,
        },
    },
}

err := notifEventRepo.CreateEvent(ctx, notifEvent)
```

## 📊 Database Schema

The schema is already created in migration `000023_create_audit_notification_tables.up.sql`:

**Tables:**
- `xcentro.audit_events` - Audit event queue
- `xcentro.audit_trail` - Permanent audit log
- `xcentro.notification_events` - Notification event queue
- `xcentro.user_notifications` - User notification inbox
- `xcentro.notification_preferences` - User preferences

## ⚙️ Configuration

Add these to your `.env` file:

```bash
# =============================================================================
# WORKER CONFIGURATION
# =============================================================================
# Audit Worker
AUDIT_WORKER_BATCH_SIZE=100
AUDIT_WORKER_POLL_INTERVAL_SECONDS=2

# Notification Worker
NOTIFICATION_WORKER_BATCH_SIZE=50
NOTIFICATION_WORKER_POLL_INTERVAL_SECONDS=2
NOTIFICATION_WORKER_MAX_RETRIES=3
```

## 🏃 Running the System

1. **Run migrations:**
```bash
./bin/migrate -action=up
```

2. **Start API server:**
```bash
go run ./cmd/api
```

3. **Start workers (in separate terminals):**
```bash
# Terminal 1: Audit Worker
go run ./cmd/workers/audit_worker

# Terminal 2: Notification Worker
go run ./cmd/workers/notification_worker
```

## 📝 Implementation Status

### ✅ Completed
- ✅ **Clean Architecture**: Services orchestrate all business logic and transactions
- ✅ **Minimal Workers/Handlers**: Thin layer that delegates to services
- ✅ **Transaction Management**: Services use `txManager.WithTx()` pattern
- ✅ **Repository WithTx Pattern**: All repositories support transactional operations
- ✅ **Audit Service**: Includes `ProcessPendingAuditEvents()` for worker orchestration
- ✅ **Notification Service**: Includes `ProcessPendingNotificationEvents()` for worker orchestration
- ✅ **Email After Commit**: Notifications sent AFTER DB transaction commits
- ✅ **PostgreSQL repository adapters** for all entities
- ✅ **HTTP handlers** using middleware-injected `userId`
- ✅ **Shared configuration system** integration
- ✅ **Email notifications** via AWS SES (with retry)
- ✅ **Idempotent processing** via ON CONFLICT clauses
- ✅ **FOR UPDATE SKIP LOCKED** within service-controlled transactions

### ⏳ TODO: Remaining Tasks

**High Priority:**
1. **User Resolution in Notification Service** (`internal/services/notification/notification_service.go`):
   - Implement `getUsersByTarget()` method to query `business_user_relation` table
   - Support unicast (specific user IDs), multicast (role-filtered), and broadcast (all users with minimum role)
   - Include user email addresses for notification delivery
   - Marked with `MVP_TODO` in code

**Medium Priority:**
2. **Push Notification Support**:
   - Currently returns success as placeholder
   - Needs integration with push notification service (FCM/APNS)

**Low Priority:**
3. **API Route Registration**:
   - Wire up audit handlers in main API router
   - Wire up notification handlers in main API router
   - Add role-based middleware for ops endpoints

**Optional Enhancements:**
4. **Metrics & Monitoring**:
   - Add CloudWatch metrics for worker processing rates
   - Track notification delivery success/failure rates
   - Monitor audit trail growth

These items are marked with TODO comments in the respective files.

## 🔒 Security & Best Practices

### Security
- ✅ Audit trails are **append-only** (no update/delete methods in repository interfaces)
- ✅ JWT middleware handles authentication (validates token, injects `userId`)
- ✅ Role middleware handles authorization (checks user role in business)
- ✅ Handlers use middleware-injected `userId` (no manual JWT parsing)
- ✅ `X-Business-ID` header provides business context
- ⏳ Ops endpoints should be protected with role-based middleware (not yet implemented)
- ✅ User notifications respect privacy via notification_preferences table

### Clean Architecture Pattern

**The Golden Rule**: Services own all orchestration. Handlers and workers are thin.

1. **Handlers** (HTTP layer):
   - ✅ Extract request parameters
   - ✅ Get `userId` from `c.Get("userId")` (middleware-injected)
   - ✅ Call service methods
   - ✅ Return HTTP responses
   - ❌ No transaction management
   - ❌ No business logic

2. **Workers** (Background jobs):
   - ✅ Call single service method with batch size
   - ✅ Log progress and errors
   - ❌ No transaction management
   - ❌ No business logic
   - ❌ No direct repository access

3. **Services** (Business logic):
   - ✅ Receive `txManager` via dependency injection
   - ✅ Orchestrate transactions via `txManager.WithTx()`
   - ✅ Transform data between layers
   - ✅ Make business decisions
   - ✅ Call external APIs (email/push AFTER DB commit)
   - ✅ Provide methods for both handlers and workers

4. **Repositories** (Data access):
   - ✅ Implement `WithTx(tx)` methods
   - ✅ Receive DB/TX client from service
   - ✅ Execute SQL queries only
   - ❌ No business logic
   - ❌ No transaction management

### Transaction Management Pattern

**Service Transaction Control:**
```go
// Service receives txManager via dependency injection
func (s *AuditService) ProcessPendingAuditEvents(ctx context.Context, batchSize int) (int, error) {
    var processedCount int

    // Service orchestrates transaction
    err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
        // Get transactional repository instances
        txEventRepo := s.auditEventRepo.WithTx(tx)
        txTrailRepo := s.auditTrailRepo.WithTx(tx)

        // FOR UPDATE SKIP LOCKED within transaction
        events, err := txEventRepo.FetchPendingEvents(ctx, batchSize)
        // ... business logic ...

        return nil // Commit on nil, rollback on error
    })

    return processedCount, err
}
```

**Key Points:**
- ✅ Services control transaction boundaries
- ✅ FOR UPDATE SKIP LOCKED executed within service-controlled transaction
- ✅ Auto-commit on success, auto-rollback on error
- ✅ Repositories are stateless - receive client from caller
- ✅ External API calls (email/push) happen AFTER transaction commits

## 📈 Scaling Path (Phase 2)

When ready to scale, add Redis for push-based processing:
- Add Redis LPUSH after DB commits
- Workers use BLPOP instead of polling
- Add sweeper process for orphaned events (5-minute recovery window)

This is documented in the original design doc but not needed for MVP.

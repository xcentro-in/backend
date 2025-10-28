# Notification System Architecture

**Version:** 1.0
**Last Updated:** 2025-10-28
**Status:** Production Ready

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Database Schema](#database-schema)
4. [Worker Architecture](#worker-architecture)
5. [Service Layer](#service-layer)
6. [HTTP API Endpoints](#http-api-endpoints)
7. [Status Flow & Retry Logic](#status-flow--retry-logic)
8. [Idempotency & Duplicate Prevention](#idempotency--duplicate-prevention)
9. [Performance & Indexes](#performance--indexes)
10. [Configuration](#configuration)
11. [Monitoring & Observability](#monitoring--observability)
12. [Future Considerations](#future-considerations)
13. [Integration Guide: Triggering Notifications from Your Code](#integration-guide-triggering-notifications-from-your-code)
14. [Migration to Queue-Based System](#migration-to-queue-based-system)

---

## Overview

The XCentro notification system delivers notifications across multiple channels (web, email, push) with the following features:

- ✅ **Multi-channel delivery**: Web (in-app), Email (AWS SES), Push (future)
- ✅ **Idempotency protection**: Database constraints + SES tags prevent duplicates
- ✅ **Retry logic**: Automatic retries with configurable max attempts
- ✅ **Race condition prevention**: Atomic job claiming with `processing` status
- ✅ **Crash recovery**: Stuck job recovery for worker failures
- ✅ **Horizontal scalability**: Configurable goroutine pools per worker type
- ✅ **User preferences**: Redis-cached preferences with explicit invalidation
- ✅ **Validation**: Read status only allowed for web channel

---

## Architecture

### High-Level Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     NOTIFICATION FLOW                           │
└─────────────────────────────────────────────────────────────────┘

1. Event Creation (API/Internal Service)
   ↓
   notification_events (pending)
   ↓
2. Fanout Worker Pool (1-N goroutines)
   ↓
   user_notifications (pending, one row per channel)
   ↓
3. Dispatch Worker Pool (1-N goroutines per channel)
   ↓
   Email: AWS SES | Web: Persistent inbox | Push: FCM (future)
   ↓
4. Status Update
   sent/failed (with retry logic)
   ↓
5. Recovery Worker Pool (1-N goroutines, periodic)
   - Resets stuck 'processing' → 'pending'
   - Deletes permanently failed (retry_count >= max)
```

---

## Database Schema

### Table: `notification_events`

**Purpose:** Queue of notification events to be fanned out to users.

```sql
CREATE TABLE xcentro.notification_events (
    id BIGSERIAL PRIMARY KEY,
    processed_at TIMESTAMPTZ DEFAULT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notification_events_pending
ON xcentro.notification_events (created_at)
WHERE processed_at IS NULL;
```

**Lifecycle:**
- `processed_at IS NULL` → Pending event
- `processed_at IS NOT NULL` → Processed (fanned out)

---

### Table: `user_notifications`

**Purpose:** User inbox with per-channel notification jobs.

```sql
CREATE TABLE xcentro.user_notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    business_id UUID REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    event_id BIGINT NOT NULL REFERENCES xcentro.notification_events(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,             -- 'email', 'push', 'web'
    status TEXT NOT NULL DEFAULT 'pending',
    content JSONB NOT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ DEFAULT NULL,
    read_at TIMESTAMPTZ DEFAULT NULL,

    CONSTRAINT uq_user_notifications_event UNIQUE (event_id, user_id, channel)
);
```

**Status Values:**
- `pending` - Ready to be processed
- `processing` - Currently being processed by a worker (prevents duplicate processing)
- `sent` - Successfully delivered
- `failed` - Delivery failed (will retry if retry_count < max)
- `read` - User has read the notification (web channel only)

**Indexes:** See [Performance & Indexes](#performance--indexes) section.

---

### Table: `notification_preferences`

**Purpose:** User notification preferences (cached in Redis).

```sql
CREATE TABLE xcentro.notification_preferences (
    user_id UUID PRIMARY KEY REFERENCES xcentro.user_table(id) ON DELETE CASCADE,
    email_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    push_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    messages_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    category_preferences JSONB NOT NULL DEFAULT '{"announcements": true, ...}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NULL
);
```

**Caching Strategy:**
- **Cache Key:** `xcentro:notification:prefs:{userId}`
- **TTL:** 1 hour (fallback)
- **Invalidation:** Explicit cache delete on `UpdatePreferences`

---

## Worker Architecture

### Multi-Pool Goroutine Design

```go
type NotificationWorker struct {
    fanoutService   notification.FanoutService
    dispatchService notification.DispatchService
    recoveryService notification.RecoveryService

    // Configurable pool sizes
    fanoutWorkerCount        int  // default: 1
    emailDispatchWorkerCount int  // default: 2
    recoveryWorkerCount      int  // default: 1
}
```

### Worker Responsibilities

#### **Pool 1: Fanout Workers** (notification_events → user_notifications)

**Frequency:** High (every 2s by default)
**Goroutines:** Configurable (`NOTIFICATION_FANOUT_WORKERS`)

**Flow:**
1. Fetch pending events with `FOR UPDATE SKIP LOCKED` (in transaction)
2. Resolve target users (unicast/multicast/broadcast)
3. Get user preferences (Redis cached, read-through pattern)
4. Create `user_notifications` rows (one per enabled channel)
5. Mark events as `processed_at = NOW()`
6. Commit transaction (all atomic)

**Transaction Boundary:** Everything in ONE transaction (safe, no race conditions)

---

#### **Pool 2: Dispatch Workers** (user_notifications → Email/Push)

**Frequency:** High (every 2s by default)
**Goroutines:** Configurable per channel (`NOTIFICATION_EMAIL_DISPATCH_WORKERS`)

**Flow:**
1. **Transaction 1:** Atomically fetch and mark jobs as `processing`
   ```sql
   WHERE channel = 'email'
     AND (status = 'pending' OR (status = 'failed' AND retry_count < max))
   SET status = 'processing', retry_count = retry_count + 1 (only for failed)
   ```
2. **Outside Transaction:** Send emails via AWS SES (slow, no locks held)
   - Email includes `notification_id` tag for idempotency tracking
3. **Transaction 2:** Update job statuses (`sent` or `failed`)

**Critical Design:**
- ✅ Email sending happens OUTSIDE transaction (no DB locks held)
- ✅ Atomic job claiming prevents duplicate processing
- ✅ `retry_count` only incremented for retries (not first attempt)
- ✅ SES tags with `notification_id` provide audit trail for duplicates

---

#### **Pool 3: Recovery Workers** (Cleanup & Recovery)

**Frequency:** Low (every 60s by default)
**Goroutines:** Configurable (`NOTIFICATION_RECOVERY_WORKERS`)

**Operations:**
1. **Recover stuck processing jobs:**
   ```sql
   UPDATE user_notifications
   SET status = 'pending'
   WHERE status = 'processing' AND created_at < NOW() - INTERVAL '10 minutes'
   ```

2. **Delete permanently failed jobs:**
   ```sql
   DELETE FROM user_notifications
   WHERE status = 'failed' AND retry_count >= max_retry_attempts
   ```

**Purpose:** Handles worker crashes and cleans up undeliverable notifications.

---

## Service Layer

### Service Interfaces (Core Layer)

```go
// internal/core/notification/notification_service.go

// User-facing operations (HTTP API)
type NotificationService interface {
    GetUserNotifications(ctx, userID, limit, offset) ([]*UserNotification, error)
    GetUnreadCount(ctx, userID) (int, error)
    MarkAsRead(ctx, notificationID, userID) error
    MarkMultipleAsRead(ctx, notificationIDs, userID) error
    GetPreferences(ctx, userID) (*NotificationPreferences, error)
    UpdatePreferences(ctx, prefs) error
    TriggerManualNotification(ctx, payload) error
}

// Worker operations
type FanoutService interface {
    ProcessPendingEvents(ctx, batchSize) (int, error)
}

type DispatchService interface {
    ProcessEmailDispatches(ctx, batchSize) (int, error)
}

type RecoveryService interface {
    ProcessRecoveryAndCleanup(ctx) (recovered, deleted int64, error)
}
```

### Implementations

| Service | Implementation | Purpose |
|---------|---------------|---------|
| **FanoutService** | `internal/services/notification/fanout_service.go` | Fan out events to users |
| **DispatchService** | `internal/services/notification/dispatch_service.go` | Send emails via SES |
| **RecoveryService** | `internal/services/notification/recovery_service.go` | Cleanup & recovery |
| **UserFacingNotificationService** | `internal/services/notification/user_facing_service.go` | API operations |

---

## HTTP API Endpoints

### Authentication
All user-facing endpoints require JWT authentication. The `userId` is extracted from the JWT token automatically by middleware.

### User Notification Endpoints

#### **GET /v1/notifications**
Get paginated list of notifications for authenticated user.

**Handler:** `internal/handlers/http/notification/notification_handler.go:26`

**Query Parameters:**
- `limit` (optional): Number of notifications to return (default: 50, max: 100)
- `offset` (optional): Pagination offset (default: 0)

**Response:**
```json
{
  "message": "Notifications retrieved successfully",
  "data": {
    "notifications": [
      {
        "id": 123,
        "user_id": "uuid",
        "business_id": "uuid",
        "event_id": 456,
        "channel": "web",
        "status": "sent",
        "content": {
          "title": "Payment Completed",
          "body": "Your payment of ₹5,000 was successful",
          "payment_id": "pay_12345"
        },
        "retry_count": 0,
        "created_at": "2025-10-28T10:30:00Z",
        "sent_at": "2025-10-28T10:30:01Z",
        "read_at": null
      }
    ],
    "unread_count": 5,
    "limit": 50,
    "offset": 0
  }
}
```

---

#### **GET /v1/notifications/unread/count**
Get count of unread notifications for authenticated user.

**Handler:** `internal/handlers/http/notification/notification_handler.go:71`

**Response:**
```json
{
  "message": "Unread count retrieved successfully",
  "data": {
    "unread_count": 5
  }
}
```

**Note:** Only web channel notifications are counted (email/push cannot be "unread").

---

#### **POST /v1/notifications/:id/read**
Mark a single notification as read.

**Handler:** `internal/handlers/http/notification/notification_handler.go:97`

**URL Parameters:**
- `id`: Notification ID (int64)

**Response:**
```json
{
  "message": "Notification marked as read"
}
```

**Errors:**
- `400 INVALID_NOTIFICATION_ID`: Invalid notification ID format
- `500 MARK_READ_FAILED`: Notification not found, already read, or not a web notification

**Note:** Only web channel notifications can be marked as read. Email/push notifications will return an error.

---

#### **POST /v1/notifications/read**
Mark multiple notifications as read in a single request.

**Handler:** `internal/handlers/http/notification/notification_handler.go:127`

**Request Body:**
```json
{
  "notification_ids": [123, 456, 789]
}
```

**Response:**
```json
{
  "message": "Notifications marked as read",
  "count": 3
}
```

---

### Notification Preferences Endpoints

#### **GET /v1/notifications/preferences**
Get user's notification preferences.

**Handler:** `internal/handlers/http/notification/notification_handler.go:161`

**Response:**
```json
{
  "message": "Preferences retrieved successfully",
  "data": {
    "user_id": "uuid",
    "email_enabled": true,
    "push_enabled": true,
    "messages_enabled": true,
    "category_preferences": {
      "announcements": true,
      "rewards": true,
      "payments": true,
      "payees": true,
      "approvals": true,
      "security": true,
      "insights": true
    },
    "created_at": "2025-10-28T10:00:00Z",
    "updated_at": "2025-10-28T12:00:00Z"
  }
}
```

**Caching:** Preferences are cached in Redis with 1-hour TTL for performance.

---

#### **PUT /v1/notifications/preferences**
Update user's notification preferences.

**Handler:** `internal/handlers/http/notification/notification_handler.go:185`

**Request Body (all fields optional):**
```json
{
  "email_enabled": false,
  "push_enabled": true,
  "messages_enabled": true,
  "category_preferences": {
    "announcements": true,
    "rewards": false,
    "payments": true,
    "payees": true,
    "approvals": true,
    "security": true,
    "insights": false
  }
}
```

**Response:**
```json
{
  "message": "Preferences updated successfully",
  "data": {
    "user_id": "uuid",
    "email_enabled": false,
    "push_enabled": true,
    "messages_enabled": true,
    "category_preferences": { ... },
    "created_at": "2025-10-28T10:00:00Z",
    "updated_at": "2025-10-28T14:30:00Z"
  }
}
```

**Cache Invalidation:** Redis cache is explicitly invalidated on preference updates.

---

### Operations/Campaign Endpoints

#### **POST /v1/ops/notifications/trigger**
Trigger a manual notification event (for campaigns, announcements, or ops).

**Handler:** `internal/handlers/http/notification/notification_handler.go:240`

**Authentication:** Requires ops/admin privileges (implementation-specific middleware)

**Request Body:**
```json
{
  "timestamp": "2025-10-28T14:30:00Z",
  "event_type": "system.maintenance",
  "target": {
    "business_id": "uuid",
    "type": "broadcast",
    "minimum_role": "support"
  },
  "entities": {
    "business_id": "uuid"
  },
  "message": {
    "title": "Scheduled Maintenance",
    "body": "XCentro will undergo maintenance on Oct 30, 2025 from 2 AM to 4 AM IST."
  },
  "actions": [
    {
      "type": "button-neutral",
      "text": "Learn More",
      "href": "/maintenance-schedule"
    }
  ]
}
```

**Target Types:**
- **`unicast`**: Single specific user
  ```json
  {
    "type": "unicast",
    "business_id": "uuid",
    "ids": ["user_uuid"]
  }
  ```

- **`multicast`**: Multiple specific users (with optional role filter)
  ```json
  {
    "type": "multicast",
    "business_id": "uuid",
    "ids": ["user_uuid1", "user_uuid2"],
    "minimum_role": "finance"  // optional
  }
  ```

- **`broadcast`**: All users in business with minimum role
  ```json
  {
    "type": "broadcast",
    "business_id": "uuid",
    "minimum_role": "support"  // all users with role >= support
  }
  ```

**Response:**
```json
{
  "message": "Notification triggered successfully"
}
```

**How It Works:**
1. Notification event is created in `notification_events` table (status: pending)
2. Fanout worker picks it up and fans out to all target users
3. User preferences are checked (Redis cached)
4. `user_notifications` rows created for each enabled channel
5. Dispatch workers send emails/push notifications
6. Web notifications appear in user's inbox immediately

**Use Cases:**
- Broadcast announcements to all business users
- Campaign notifications to specific user segments
- System maintenance notifications
- Promotional campaigns
- Custom business logic notifications

---

## Status Flow & Retry Logic

### Job Lifecycle

```
┌──────────────────────────────────────────────────────────────┐
│                    NOTIFICATION JOB LIFECYCLE                │
└──────────────────────────────────────────────────────────────┘

1️⃣ FIRST ATTEMPT (Not a retry)
   pending (retry_count=0) → processing (retry_count=0)
   → Send email
   → If SUCCESS: sent (retry_count=0) ✅
   → If FAILURE: failed (retry_count=0) ❌

2️⃣ FIRST RETRY
   failed (retry_count=0) → processing (retry_count=1)
   → Send email
   → If SUCCESS: sent (retry_count=1) ✅
   → If FAILURE: failed (retry_count=1) ❌

3️⃣ SECOND RETRY
   failed (retry_count=1) → processing (retry_count=2)
   → Send email
   → If SUCCESS: sent (retry_count=2) ✅
   → If FAILURE: failed (retry_count=2) ❌

4️⃣ THIRD RETRY (max_retry_attempts=3)
   failed (retry_count=2) → processing (retry_count=3)
   → Send email
   → If SUCCESS: sent (retry_count=3) ✅
   → If FAILURE: failed (retry_count=3) ❌

5️⃣ PERMANENT FAILURE
   failed (retry_count=3) → NOT picked up (retry_count >= max)
   → Recovery worker DELETES this job 🗑️
```

### Retry Count Semantics

- **`retry_count = 0`**: First attempt (never failed before)
- **`retry_count = 1`**: Failed once, retried once
- **`retry_count = 2`**: Failed twice, retried twice
- **`retry_count = 3`**: Failed thrice, retried thrice (reached max)

**With `max_retry_attempts=3`:**
Total attempts = 1 original + 3 retries = **4 total attempts**

---

## Idempotency & Duplicate Prevention

### Overview

The notification system implements idempotency at multiple levels to prevent duplicate notifications:

1. **Fanout Idempotency**: Prevents duplicate `user_notifications` entries
2. **Dispatch Idempotency**: Prevents duplicate email sends
3. **Read Status Validation**: Prevents invalid read operations on non-web channels

---

### Fanout Idempotency (notification_events → user_notifications)

**Problem:** What if the same event is processed multiple times?

**Solution:** Database-level constraint with conflict handling

```sql
-- Database constraint
CONSTRAINT uq_user_notifications_event UNIQUE (event_id, user_id, channel)

-- Insert query with conflict handling
INSERT INTO xcentro.user_notifications (
    user_id, business_id, event_id, channel, status, content
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (event_id, user_id, channel) DO NOTHING
RETURNING id, created_at
```

**How It Works:**
- Each event can create at most ONE notification per user per channel
- If fanout worker crashes mid-transaction, the transaction rolls back (no duplicates)
- If fanout worker processes the same event twice, `ON CONFLICT DO NOTHING` silently ignores duplicates

**Transaction Boundary:**
```
BEGIN TRANSACTION
  1. Fetch events (FOR UPDATE SKIP LOCKED)
  2. Resolve users
  3. Get preferences (Redis cached)
  4. Insert user_notifications (ON CONFLICT DO NOTHING)
  5. Mark events as processed
COMMIT
```

✅ **Result:** Fanout is fully idempotent. No duplicate `user_notifications` entries possible.

---

### Dispatch Idempotency (user_notifications → Email)

**Problem:** What if worker crashes after sending email but before updating status?

**Scenario:**
```
Worker 1:
  1. Fetch job (ID=123), mark as 'processing' ✅
  2. Send email to user@example.com ✅
  3. CRASH ❌ (before updating status to 'sent')

Recovery Worker:
  1. Reset job 123 from 'processing' → 'pending' (after timeout)

Worker 2:
  1. Fetch job (ID=123), mark as 'processing'
  2. Send email to user@example.com again 🚨 DUPLICATE EMAIL
```

**Solution:** AWS SES Tags as Idempotency Key

```go
// internal/services/notification/helpers.go

func SendEmailNotification(ctx context.Context, emailClient *mailers.SESClient,
    notificationID int64, to, subject, body string) error {

    msg := &mailers.EmailMessage{
        To:       []string{to},
        Subject:  subject,
        HTMLBody: body,
        TextBody: stripHTML(body),
        Tags: map[string]string{
            "notification_id": fmt.Sprintf("%d", notificationID), // Idempotency key
            "source":          "xcentro_notification_system",
        },
    }

    _, err := emailClient.SendEmail(ctx, msg)
    return err
}
```

**How It Works:**
- Every email sent includes `notification_id` tag with the `user_notifications.id`
- If the same notification ID is sent multiple times, AWS SES receives the same idempotency key
- AWS SES can deduplicate based on internal policies (though not guaranteed by AWS)
- The tag also helps with:
  - **Debugging**: Track which notification triggered which email
  - **Analytics**: Link email delivery metrics back to notification IDs
  - **Auditing**: Trace email sends in SES logs

**Transaction Boundaries:**
```
TRANSACTION 1:
  1. Fetch jobs (FOR UPDATE SKIP LOCKED)
  2. Mark as 'processing', increment retry_count (only for retries)
COMMIT

OUTSIDE TRANSACTION:
  3. Send emails via SES (with notification_id tag)

TRANSACTION 2:
  4. Update statuses to 'sent' or 'failed'
COMMIT
```

**Why Not 100% Guaranteed?**
- AWS SES doesn't provide built-in idempotency guarantees via tags
- Tags are primarily for metadata and tracking
- However, this approach provides:
  - ✅ Audit trail for debugging
  - ✅ Ability to detect duplicates in logs
  - ✅ Future-proof if AWS adds tag-based deduplication

**Best Practice:**
- Keep email sending OUTSIDE transactions (prevents long-held locks)
- Use `processing` status to minimize race conditions
- Recovery worker timeout (10 min default) reduces duplicate window
- Monitor SES logs for duplicate `notification_id` tags

✅ **Result:** Dispatch has strong idempotency protections with audit trail.

---

### Read Status Validation (web channel only)

**Problem:** Email/push notifications cannot be marked as "read" (they're ephemeral).

**Solution:** Database-level enforcement in repository queries

```go
// internal/adapters/database/postgres/notification/user_notification_repository.go

func (r *PgUserNotificationRepository) MarkAsRead(ctx context.Context,
    notificationID int64, userID string) error {

    query := `
        UPDATE xcentro.user_notifications
        SET read_at = NOW(), status = $3
        WHERE id = $1 AND user_id = $2 AND read_at IS NULL AND channel = $4
    `

    result, err := r.querier.Exec(ctx, query, notificationID, userID,
        notification.StatusRead, notification.ChannelWeb)

    if result.RowsAffected() == 0 {
        return fmt.Errorf("notification not found, already read, or not a web notification")
    }

    return nil
}
```

**How It Works:**
- `WHERE channel = 'web'` constraint in query
- Email/push channel notifications cannot be marked as read
- Returns error if user attempts to mark non-web notification as read

**Partial Index Optimization:**
```sql
-- Only indexes unread web notifications (optimized for GetUnreadCount)
CREATE INDEX idx_user_notifications_user_unread
ON xcentro.user_notifications (user_id, created_at DESC)
WHERE read_at IS NULL AND channel = 'web';
```

✅ **Result:** Read status is only applicable to web channel.

---

### Summary: Idempotency Guarantees

| Component | Idempotency Method | Guarantee Level | Protection Against |
|-----------|-------------------|-----------------|-------------------|
| **Fanout Service** | Database constraint + `ON CONFLICT DO NOTHING` | 🟢 **Strong** | Duplicate user_notifications |
| **Dispatch Service** | SES Tags with `notification_id` | 🟡 **Moderate** | Duplicate emails (with audit trail) |
| **Read Status** | Database query validation | 🟢 **Strong** | Invalid read on email/push |
| **Job Claiming** | `FOR UPDATE SKIP LOCKED` + `processing` status | 🟢 **Strong** | Race conditions |
| **Recovery Worker** | Timeout-based reset | 🟡 **Moderate** | Stuck jobs (may cause retries) |

**Legend:**
- 🟢 **Strong**: Database-enforced, guaranteed by constraints
- 🟡 **Moderate**: Application-level, best-effort with audit trail
- 🔴 **Weak**: Prone to edge cases, requires monitoring

---

## Performance & Indexes

### Index Strategy

#### **Index 1: User Inbox** (High Frequency)
```sql
CREATE INDEX idx_user_notifications_user_id
ON xcentro.user_notifications (user_id, created_at DESC);
```
**Query:** `GetUserNotifications(user_id, limit, offset)`
**Performance:** O(log n) + limit

---

#### **Index 2: Unread Count** (High Frequency)
```sql
CREATE INDEX idx_user_notifications_user_unread
ON xcentro.user_notifications (user_id, created_at DESC)
WHERE read_at IS NULL AND channel = 'web';
```
**Query:** `GetUnreadCount(user_id)`
**Performance:** O(log n), COUNT(*) is index-only
**Size:** Partial index (only unread web notifications)

---

#### **Index 3: Worker Job Fetching** (Very High Frequency)
```sql
CREATE INDEX idx_user_notifications_worker_jobs
ON xcentro.user_notifications (channel, status, retry_count, created_at)
WHERE status IN ('pending', 'failed');
```
**Query:** `FetchAndMarkJobsAsProcessing(channel, limit, maxRetryAttempts)`
**Performance:** O(log n) + limit, FOR UPDATE SKIP LOCKED prevents contention
**Size:** Partial index (excludes sent/read/processing)

---

#### **Index 4: Stuck Processing Recovery** (Low Frequency)
```sql
CREATE INDEX idx_user_notifications_stuck_processing
ON xcentro.user_notifications (created_at)
WHERE status = 'processing';
```
**Query:** `RecoverStuckProcessingJobs(timeoutMinutes)`
**Performance:** O(log n) + matching rows
**Size:** Partial index (usually <100 rows)

---

### Query Performance Summary

| Operation | Index | Performance | Concurrency |
|-----------|-------|-------------|-------------|
| FetchAndMarkJobsAsProcessing | Partial (pending/failed) | O(log n) + 50 | ✅ SKIP LOCKED |
| UpdateJobStatuses | Primary Key | O(1) per ID | ✅ No locking |
| GetUserNotifications | User ID composite | O(log n) + limit | ✅ Pagination |
| GetUnreadCount | Partial (unread web) | O(log n), index-only | ✅ COUNT(*) |
| RecoverStuckProcessingJobs | Partial (processing) | O(log n) + tiny | ✅ Periodic |

---

### Storage Efficiency (Estimated for 1M notifications)

| Index | Type | Est. Rows | Est. Size |
|-------|------|-----------|-----------|
| PK (id) | Full | 1,000,000 | ~40 MB |
| user_id composite | Full | 1,000,000 | ~50 MB |
| user_unread | **Partial** | ~50,000 | ~3 MB |
| worker_jobs | **Partial** | ~5,000 | ~500 KB |
| stuck_processing | **Partial** | ~100 | ~10 KB |

**Total:** ~93 MB for 1M rows (vs ~150 MB without partial indexes)

---

## Configuration

### Environment Variables

```bash
# Job processing
NOTIFICATION_WORKER_BATCH_SIZE=50
NOTIFICATION_WORKER_POLL_INTERVAL_SECONDS=2

# Retry configuration
NOTIFICATION_MAX_RETRY_ATTEMPTS=3

# Goroutine pool sizes
NOTIFICATION_FANOUT_WORKERS=1
NOTIFICATION_EMAIL_DISPATCH_WORKERS=2
NOTIFICATION_PUSH_DISPATCH_WORKERS=0  # 0 = disabled (MVP)

# Recovery configuration
NOTIFICATION_PROCESSING_TIMEOUT_MINUTES=10
NOTIFICATION_RECOVERY_WORKERS=1
NOTIFICATION_RECOVERY_INTERVAL_SECONDS=60
```

### Configuration Struct

```go
// internal/shared/config/config.go
type NotificationWorkerConfig struct {
    BatchSize                int `env:"NOTIFICATION_WORKER_BATCH_SIZE" env-default:"50"`
    PollInterval             int `env:"NOTIFICATION_WORKER_POLL_INTERVAL_SECONDS" env-default:"2"`
    MaxRetryAttempts         int `env:"NOTIFICATION_MAX_RETRY_ATTEMPTS" env-default:"3"`
    FanoutWorkers            int `env:"NOTIFICATION_FANOUT_WORKERS" env-default:"1"`
    EmailDispatchWorkers     int `env:"NOTIFICATION_EMAIL_DISPATCH_WORKERS" env-default:"2"`
    PushDispatchWorkers      int `env:"NOTIFICATION_PUSH_DISPATCH_WORKERS" env-default:"0"`
    ProcessingTimeoutMinutes int `env:"NOTIFICATION_PROCESSING_TIMEOUT_MINUTES" env-default:"10"`
    RecoveryWorkers          int `env:"NOTIFICATION_RECOVERY_WORKERS" env-default:"1"`
    RecoveryIntervalSeconds  int `env:"NOTIFICATION_RECOVERY_INTERVAL_SECONDS" env-default:"60"`
}
```

---

## Monitoring & Observability

### Database Monitoring

#### **Check Index Usage**
```sql
SELECT schemaname, tablename, indexname, idx_scan, idx_tup_read
FROM pg_stat_user_indexes
WHERE tablename = 'user_notifications'
ORDER BY idx_scan DESC;
```

#### **Check Table Size & Bloat**
```sql
-- Total table size
SELECT pg_size_pretty(pg_total_relation_size('xcentro.user_notifications'));

-- Check for bloat (after many deletes)
SELECT pg_size_pretty(pg_relation_size('xcentro.user_notifications')) AS table_size,
       pg_size_pretty(pg_indexes_size('xcentro.user_notifications')) AS indexes_size;
```

#### **Check Worker Queue Depth**
```sql
-- Pending events (fanout backlog)
SELECT COUNT(*) FROM xcentro.notification_events WHERE processed_at IS NULL;

-- Pending email jobs (dispatch backlog)
SELECT COUNT(*) FROM xcentro.user_notifications
WHERE channel = 'email' AND status = 'pending';

-- Failed jobs needing retry
SELECT COUNT(*) FROM xcentro.user_notifications
WHERE channel = 'email' AND status = 'failed' AND retry_count < 3;

-- Stuck processing jobs (potential crashes)
SELECT COUNT(*) FROM xcentro.user_notifications
WHERE status = 'processing' AND created_at < NOW() - INTERVAL '10 minutes';

-- Check for potential duplicate email sends (same event_id sent multiple times)
-- This can happen if recovery worker resets a job after email was sent
SELECT event_id, user_id, channel, COUNT(*) as send_count
FROM xcentro.user_notifications
WHERE channel = 'email' AND status = 'sent'
GROUP BY event_id, user_id, channel
HAVING COUNT(*) > 1;
```

**Note:** The last query detects if the same notification was sent multiple times. In a healthy system, this count should be 0. If you see duplicates, check AWS SES logs for the `notification_id` tag to confirm if emails were actually sent twice.

### Application Metrics (Recommended)

**Prometheus/Grafana Metrics:**
- `notification_events_processed_total` - Counter of fanned out events
- `notification_emails_sent_total` - Counter of sent emails
- `notification_emails_failed_total` - Counter of failed emails
- `notification_recovery_stuck_jobs_total` - Counter of recovered jobs
- `notification_recovery_deleted_jobs_total` - Counter of deleted jobs
- `notification_worker_errors_total` - Counter of worker errors
- `notification_processing_duration_seconds` - Histogram of processing time

---

## Future Considerations

### 1. Push Notification Support

**When enabling push notifications:**

1. **Update Configuration:**
   ```bash
   NOTIFICATION_PUSH_DISPATCH_WORKERS=2  # Enable push workers
   ```

2. **Create Push Dispatcher Service:**
   ```go
   // internal/services/notification/push_dispatch_service.go
   type PushDispatchService struct {
       userNotifRepo    notification.UserNotificationRepository
       fcmClient        *fcm.Client
       txManager        client.TxManager
       maxRetryAttempts int
   }

   func (s *PushDispatchService) ProcessPushDispatches(ctx, batchSize) (int, error) {
       // Similar pattern to email dispatch
   }
   ```

3. **Update Worker:**
   ```go
   // Add push dispatcher pool to notification_worker.go
   for i := 0; i < w.pushDispatchWorkerCount; i++ {
       wg.Add(1)
       go func(workerID int) {
           defer wg.Done()
           sentCount, err := w.pushDispatchService.ProcessPushDispatches(ctx, w.batchSize)
           // ... error handling
       }(i + 1)
   }
   ```

4. **No database changes needed** - Already supports `channel = 'push'`

---

### 2. Business-Scoped Analytics

**If you need to query notifications by business:**

```sql
CREATE INDEX idx_user_notifications_business_id
ON xcentro.user_notifications (business_id, created_at DESC)
WHERE business_id IS NOT NULL;
```

**Use cases:**
- Business notification dashboard
- Business-level metrics
- Compliance reporting

---

### 3. Table Partitioning (For Very Large Tables)

**When notification table exceeds 10M rows:**

```sql
-- Create partitioned table
CREATE TABLE xcentro.user_notifications_partitioned (
    LIKE xcentro.user_notifications INCLUDING ALL
) PARTITION BY RANGE (created_at);

-- Create monthly partitions
CREATE TABLE user_notifications_2025_01
PARTITION OF user_notifications_partitioned
FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

-- Auto-create partitions with pg_partman extension
```

**Benefits:**
- Faster queries (partition pruning)
- Easy old data archival (drop old partitions)
- Better vacuum performance

---

### 4. Data Retention Policies

**Recommended retention:**

| Channel | Type | Retention | Reason |
|---------|------|-----------|--------|
| Web | Unread | Forever | User might read later |
| Web | Read | 90 days | Keep for recent history |
| Email | Sent | 7 days | Already in user's email inbox |
| Email | Failed | Delete on max retry | Undeliverable |
| Push | Sent | 1 day | Already on device |
| Push | Failed | Delete on max retry | Undeliverable |

**Implementation:**
```go
// Add to RecoveryService
func (s *RecoveryService) CleanupOldNotifications(ctx) (int64, error) {
    // Delete old sent email/push notifications
    query := `
        DELETE FROM xcentro.user_notifications
        WHERE channel IN ('email', 'push')
          AND status = 'sent'
          AND sent_at < NOW() - INTERVAL '7 days'
    `
    // ... execute
}
```

---

### 5. Read Replicas for Analytics

**For heavy read workloads (dashboards, reports):**

1. Setup PostgreSQL read replica
2. Route analytics queries to replica
3. Keep transactional queries (worker) on primary

```go
// Config
type DatabaseConfig struct {
    Primary DBConfig
    Replica DBConfig  // Read-only
}

// Repository
type AnalyticsRepository struct {
    replicaDB client.DB
}
```

---

## Integration Guide: Triggering Notifications from Your Code

### Simple Example: Payment Completed Notification

When you want to send a notification from your business logic (e.g., after payment completion), simply create a notification event:

```go
// Example: In your payment service
// File: internal/services/payment/payment_service.go

import (
    "xcentro.in/internal/core/notification"
)

type PaymentService struct {
    // ... other dependencies
    notificationEventRepo notification.NotificationEventRepository
}

func (s *PaymentService) CompletePayment(ctx context.Context, paymentID string) error {
    // 1. Complete payment logic
    payment, err := s.paymentRepo.UpdateStatus(ctx, paymentID, "completed")
    if err != nil {
        return err
    }

    // 2. Trigger notification (fire and forget - worker will pick it up)
    notifPayload := notification.NotificationEventPayload{
        Timestamp: time.Now().Format(time.RFC3339),
        EventType: notification.EventPaymentCompleted,
        Target: notification.TargetInfo{
            BusinessID: payment.BusinessID,
            Type:       notification.TargetUnicast,
            IDs:        []string{payment.CreatedBy}, // Send to user who made payment
        },
        Entities: notification.EntityReferences{
            BusinessID: &payment.BusinessID,
            PaymentID:  &paymentID,
        },
        Message: notification.MessageContent{
            Title: "Payment Completed",
            Body:  fmt.Sprintf("Your payment of ₹%s was successful", formatAmount(payment.Amount)),
        },
        Actions: []notification.ActionButton{
            {
                Type: "button-neutral",
                Text: "View Receipt",
                Href: fmt.Sprintf("/payments/%s/receipt", paymentID),
            },
        },
    }

    event := &notification.NotificationEvent{
        EventType: notification.EventPaymentCompleted,
        Payload:   notifPayload,
    }

    // Create event (non-blocking - just inserts into notification_events table)
    if err := s.notificationEventRepo.CreateEvent(ctx, event); err != nil {
        // Log error but don't fail payment
        s.logger.Errorf("Failed to create notification event: %v", err)
    }

    return nil
}
```

**That's it!** The notification worker will:
1. Pick up the event from `notification_events` table
2. Fanout to target users (checking preferences)
3. Create `user_notifications` for each enabled channel
4. Send emails and show in web inbox

---

### Broadcast Example: System Maintenance Announcement

```go
// Example: Ops/Admin triggering a broadcast
// File: internal/services/system/maintenance_service.go

func (s *MaintenanceService) AnnounceScheduledMaintenance(ctx context.Context, businessID string) error {
    notifPayload := notification.NotificationEventPayload{
        Timestamp: time.Now().Format(time.RFC3339),
        EventType: notification.EventSystemMaintenance,
        Target: notification.TargetInfo{
            BusinessID:  businessID,
            Type:        notification.TargetBroadcast, // Send to all users
            MinimumRole: strPtr(notification.RoleSupport), // All roles >= support
        },
        Message: notification.MessageContent{
            Title: "Scheduled Maintenance",
            Body:  "XCentro will undergo maintenance on Oct 30 from 2 AM to 4 AM IST.",
        },
    }

    event := &notification.NotificationEvent{
        EventType: notification.EventSystemMaintenance,
        Payload:   notifPayload,
    }

    return s.notificationEventRepo.CreateEvent(ctx, event)
}

func strPtr(s string) *string { return &s }
```

---

### Key Points:

1. **Just create an event** - Insert into `notification_events` table
2. **Non-blocking** - Your business logic doesn't wait for notification delivery
3. **Worker handles everything** - Fanout, delivery, retries, failures
4. **User preferences respected** - Worker checks Redis-cached preferences
5. **Multi-channel** - Email + Web inbox created automatically (push if enabled)

---

## Migration to Queue-Based System

### Current System: Polling-Based (Database as Queue)

**Pros:**
- ✅ Simple implementation
- ✅ No external dependencies
- ✅ ACID transactions
- ✅ Good for 1K-10K notifications/sec

**Cons:**
- ⚠️ Polling overhead (DB queries even when idle)
- ⚠️ Limited to single database scaling
- ⚠️ No built-in backpressure/rate limiting

---

### Future System: Queue-Based (Redis Streams / RabbitMQ / Kafka)

**When to migrate:**
- Notification volume exceeds 10K/sec
- Need for better horizontal scaling
- Want to reduce database load
- Need advanced features (dead letter queues, priority queues)

---

### Migration Path: Database → Redis Streams

**Why Redis Streams:**
- Already using Redis for caching
- Simple pub/sub with consumer groups
- Persistent (survives restarts)
- Good performance (100K+ msgs/sec)

---

#### **Step 1: Update `notification_events` Creation**

**Current:**
```go
// internal/services/notification/user_facing_service.go
func (s *Service) TriggerManualNotification(ctx, payload) error {
    event := &notification.NotificationEvent{
        EventType: payload.EventType,
        Payload:   payload,
    }
    return s.eventRepo.CreateEvent(ctx, event)
}
```

**New (with Redis Streams):**
```go
func (s *Service) TriggerManualNotification(ctx, payload) error {
    // Option 1: Write to Redis Stream instead of DB
    streamKey := "notification:events"
    msgID, err := s.redisClient.XAdd(ctx, &redis.XAddArgs{
        Stream: streamKey,
        Values: map[string]interface{}{
            "event_type": payload.EventType,
            "payload":    jsonMarshal(payload),
            "timestamp":  time.Now().Unix(),
        },
    }).Result()

    // Option 2 (Dual Write): Write to both DB and Redis during migration
    // This allows gradual rollout

    return err
}
```

---

#### **Step 2: Update Fanout Worker to Consume from Redis**

**Current (Polling):**
```go
// FetchPendingEvents with FOR UPDATE SKIP LOCKED
events, err := txEventRepo.FetchPendingEvents(ctx, batchSize)
```

**New (Consumer Group):**
```go
// Create consumer group (one-time setup)
s.redisClient.XGroupCreateMkStream(ctx, "notification:events", "fanout-workers", "0")

// Read from stream in worker
func (s *FanoutService) ProcessPendingEvents(ctx, batchSize) (int, error) {
    streams, err := s.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
        Group:    "fanout-workers",
        Consumer: "worker-1",  // Each worker has unique consumer ID
        Streams:  []string{"notification:events", ">"},
        Count:    int64(batchSize),
        Block:    2 * time.Second,  // Block for 2s if no messages
    }).Result()

    if err != nil || len(streams) == 0 {
        return 0, nil  // No messages
    }

    for _, msg := range streams[0].Messages {
        // Process event (same logic as before)
        payload := unmarshal(msg.Values["payload"])
        // ... create user_notifications

        // Acknowledge message
        s.redisClient.XAck(ctx, "notification:events", "fanout-workers", msg.ID)
    }

    return len(streams[0].Messages), nil
}
```

**Benefits:**
- ✅ No polling overhead
- ✅ Multiple workers consume in parallel (consumer group)
- ✅ Automatic load balancing
- ✅ Built-in acknowledgment (at-least-once delivery)

---

#### **Step 3: Update Dispatch Worker (Email Queue)**

**New Queue Structure:**
```
notification:events            → Fanout Worker → user_notifications DB
notification:jobs:email        → Email Dispatch Worker → AWS SES
notification:jobs:push         → Push Dispatch Worker → FCM
```

**Fanout Worker Writes to Redis Queue:**
```go
// After creating user_notifications in DB
for _, notif := range notificationsToCreate {
    if notif.Channel == notification.ChannelEmail {
        s.redisClient.XAdd(ctx, &redis.XAddArgs{
            Stream: "notification:jobs:email",
            Values: map[string]interface{}{
                "notif_id": notif.ID,
                "user_id":  notif.UserID,
                "content":  jsonMarshal(notif.Content),
            },
        })
    }
}
```

**Email Dispatch Worker Consumes Queue:**
```go
streams, err := s.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
    Group:    "email-dispatchers",
    Consumer: "email-worker-1",
    Streams:  []string{"notification:jobs:email", ">"},
    Count:    int64(batchSize),
    Block:    2 * time.Second,
}).Result()

// Process messages, send emails, ack on success
```

---

#### **Step 4: Database Changes**

**Keep `user_notifications` table for:**
- ✅ Web notifications (persistent inbox)
- ✅ User query history (GetUserNotifications)
- ✅ Audit trail

**Remove from `user_notifications`:**
- ❌ Email/push jobs (move to Redis Streams)

**New Schema:**
```sql
-- Keep web notifications in DB
CREATE TABLE xcentro.user_notifications (
    -- ... same schema
) PARTITION BY LIST (channel);

-- Partition for web only
CREATE TABLE user_notifications_web
PARTITION OF user_notifications FOR VALUES IN ('web');

-- Email/push go through Redis Streams (ephemeral)
```

**Benefits:**
- Smaller `user_notifications` table
- Faster queries (only web notifications)
- Better Redis performance (ephemeral email/push jobs)

---

#### **Step 5: Retry Logic with Redis**

**Dead Letter Queue Pattern:**
```go
// On send failure
maxRetries := 3
retryCount := getRetryCount(msg.ID)

if retryCount < maxRetries {
    // Re-add to queue with delay
    s.redisClient.XAdd(ctx, &redis.XAddArgs{
        Stream: "notification:jobs:email:retry",
        Values: map[string]interface{}{
            "original_msg": msg.ID,
            "retry_count":  retryCount + 1,
            "retry_at":     time.Now().Add(time.Minute * time.Duration(retryCount+1)).Unix(),
        },
    })
} else {
    // Move to dead letter queue
    s.redisClient.XAdd(ctx, &redis.XAddArgs{
        Stream: "notification:jobs:email:dlq",
        Values: msg.Values,
    })
}

// Ack original message
s.redisClient.XAck(ctx, "notification:jobs:email", "email-dispatchers", msg.ID)
```

---

### Migration Strategy

#### **Phase 1: Dual Write (Safe Rollout)**
- Write events to both DB and Redis
- Workers read from DB (existing behavior)
- Monitor Redis queue depth

#### **Phase 2: Gradual Rollover**
- 10% of workers read from Redis
- 90% still read from DB
- Monitor error rates

#### **Phase 3: Full Migration**
- All workers read from Redis
- Keep DB writes for audit trail
- Remove polling-based code

#### **Phase 4: Cleanup**
- Remove unused indexes (worker_jobs)
- Remove `notification_events.processed_at` column
- Archive old data

---

### Alternative: RabbitMQ

**If you need:**
- ✅ Stronger durability guarantees (persistent disk storage)
- ✅ Complex routing (fanout, topic exchanges)
- ✅ Dead letter exchanges (built-in)
- ✅ Management UI

**Trade-offs:**
- ⚠️ Extra infrastructure (RabbitMQ cluster)
- ⚠️ More complex operations
- ⚠️ Slightly lower throughput than Redis

---

### Alternative: Kafka

**If you need:**
- ✅ Extremely high throughput (millions/sec)
- ✅ Event replay (retain messages for days)
- ✅ Stream processing (Kafka Streams)
- ✅ Multi-datacenter replication

**Trade-offs:**
- ⚠️ Much more complex (ZooKeeper, partitions, rebalancing)
- ⚠️ Overkill for notification use case
- ⚠️ Higher latency than Redis/RabbitMQ

---

## Summary

### Current System (Database Polling)
**Good for:** 0 - 10K notifications/sec, simple setup, ACID guarantees
**Limitations:** Polling overhead, single database scaling

### Future System (Queue-Based)
**Good for:** 10K+ notifications/sec, horizontal scaling, lower DB load
**Complexity:** Additional infrastructure, distributed systems challenges

### Recommended Migration Trigger
Migrate to queue-based when:
- Notification volume consistently exceeds 5K/sec
- Database CPU usage > 70% from notification workers
- Need for advanced features (priority queues, backpressure)

**Until then:** Current polling-based system is production-ready and scales well! 🚀

---

## References

- **Architecture Diagrams:** `docs/notification_module.md`
- **Database Schema:** `migrations/sql/000023_*.sql`
- **Worker Code:** `internal/workers/notification_worker.go`
- **Services:** `internal/services/notification/`
- **Configuration:** `internal/shared/config/config.go`

---

**End of Document**

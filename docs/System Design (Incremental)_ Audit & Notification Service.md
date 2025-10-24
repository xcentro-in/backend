Here is the incremental design document, starting with a simple, robust PostgreSQL-only solution and then detailing the upgrade path to include Redis for scaling.

-----

# System Design (Incremental): Audit & Notification Service

This document outlines the architecture for a durable audit and notification system. The design is presented in two phases:

1.  **Phase 1:** A simple, robust solution using **only PostgreSQL**. This is the starting point.
2.  **Phase 2:** An enhanced, high-throughput solution adding **Redis** for scaling.

## 1\. Core Principles (Apply to Both Phases)

1.  **Decouple Systems:** Audit trails (record of fact) and notifications (user action) are separate systems. They have their own queue tables (`audit_events`, `notification_events`) to prevent "noisy neighbor" problems.
2.  **DB as Source of Truth:** PostgreSQL is the single source of truth. All events are written here durably before any processing.
3.  **Append-Only Destination:** Audit trails (`audit_trail`) and user inboxes (`user_notifications`) are built with `INSERT`s only. We **never** `UPDATE` a `jsonb` array, as this causes severe lock contention.
4.  **Idempotent Workers:** Workers are designed to run a job multiple times with the same result as running it once. This is achieved via `UNIQUE` constraints.

-----

## 2\. Phase 1: PostgreSQL-Only Architecture (Start Here)

This design is simple, highly reliable, and uses **only your existing database**. It leverages PostgreSQL's built-in transactional features to act as a reliable queue.

### Design

The producer (your business logic) writes jobs to a queue table inside a database transaction. Workers (separate Go processes) poll this table, safely grab batches of jobs, and process them.

### Producer Logic (In Business Transaction)

When a payment is processed, you do this in a *single transaction*:

```sql
BEGIN;

-- 1. Do the real work
UPDATE payments SET status = 'completed' WHERE id = 123;

-- 2. Write to the AUDIT queue (a record of fact)
INSERT INTO audit_events (event_type, payload)
VALUES ('payment.completed', '{"payment_id": 123, "amount": 10000}');

-- 3. Write to the NOTIFICATION queue (a trigger for action)
INSERT INTO notification_events (business_id, event_type, payload)
VALUES (456, 'payment.completed', '{"payment_id": 123, "customer_name": "Acme"}');

COMMIT;
```

If this succeeds, the jobs are durably saved and ready for the workers.

### Worker Logic (Polling)

Your workers run in a loop, polling the DB for jobs. They use `FOR UPDATE SKIP LOCKED` to grab a unique batch, preventing two workers from ever getting the same job.

```sql
-- 1. Begin transaction
BEGIN;

-- 2. Select and lock a unique batch
-- This is the core of the queue. It's atomic and safe.
SELECT id, payload FROM audit_events
WHERE processed_at IS NULL
ORDER BY id
LIMIT 100
FOR UPDATE SKIP LOCKED;

-- If 0 rows are returned, COMMIT and sleep for 1-2 seconds.

-- 3. (In App Code) Transform payloads and batch INSERT/COPY
INSERT INTO audit_trail (payment_id, ..., source_event_id)
VALUES (...), (...), ...
ON CONFLICT (source_event_id) DO NOTHING; -- Idempotency

-- 4. Mark jobs as complete
UPDATE audit_events
SET processed_at = NOW()
WHERE id IN (/* list of 100 IDs from step 2 */);

-- 5. Commit
COMMIT;
```

### Phase 1 Pros & Cons

  * **Pros:**
      * **Extremely Simple:** No new infrastructure.
      * **100% Reliable:** Uses DB transactions. A worker crash automatically rolls back and unlocks the jobs, which are then picked up by another worker. It's self-healing.
  * **Cons:**
      * **Polling Overhead:** Workers constantly hit the DB (`SELECT ...`). This is fine at low/medium scale but can create unnecessary load at high scale.
      * **Slight Delay:** Jobs aren't processed "instantly." They are picked up on the next poll (e.g., 1-2 seconds later).

-----

## 3\. Phase 2: Scaling with Redis (Future Enhancement)

**When to upgrade:** When your traffic grows and the constant worker polling (`SELECT ... FOR UPDATE`) starts to cause noticeable CPU/IO load on your primary database.

### Design

We change the model. The DB is still the source of truth, but **Redis becomes a high-speed "new job" signal**. Workers will listen to Redis *first*, which eliminates polling.

### New Producer Logic

The DB transaction is almost the same, but you add one step **after** it commits.

```go
// 1. Run the DB transaction (same as Phase 1)
tx, err := db.Begin()
// ... INSERT into audit_events, etc.
err = tx.Commit()

// 2. AFTER the commit, signal Redis
if err == nil {
    redis.LPush("queue:audit", event.ID)
    redis.LPush("queue:notification", event.ID)
}
```

### New Worker Logic (Signal-Based)

The worker's logic is now "push" instead of "pull."

1.  Run a blocking pop on Redis: `id := redis.BLPop("queue:audit", 0)`. The worker now sleeps *for free* until a job ID arrives.
2.  Once an ID is received, it runs the **same atomic DB transaction** as in Phase 1, but with one change: the `SELECT` is now by `id`.

<!-- end list -->

```sql
BEGIN;

-- 2. Select and lock the SPECIFIC job from Redis
-- We still use FOR UPDATE in case the sweeper re-queued it
SELECT id, payload FROM audit_events
WHERE id = <id_from_redis> AND processed_at IS NULL
FOR UPDATE SKIP LOCKED;

-- If 0 rows (already processed), COMMIT and go back to step 1.

-- 3. (In App Code) Transform payload
INSERT INTO audit_trail (...) VALUES (...) ...;

-- 4. Mark job as complete
UPDATE audit_events SET processed_at = NOW() WHERE id = <id_from_redis>;

-- 5. Commit
COMMIT;
```

### New Resiliency Component: The "Sweeper"

This new model has a failure case: what if the DB `COMMIT` succeeds but the `LPUSH` to Redis fails? The job is in the DB but not in the queue. It's an "orphan."

We fix this with a **Sweeper process** that runs every 5 minutes.

  * **Logic:** It does the *exact same* query as the Phase 1 worker, but *without* `FOR UPDATE`.
    ```sql
    SELECT id FROM audit_events
    WHERE processed_at IS NULL
      AND created_at < NOW() - '5 minutes'::interval;
    ```
  * **Action:** For every "lost" `id` it finds, it `LPUSH`es it back into the Redis queue for the workers to find. This makes the system self-healing again.

-----

## 4\. Shared Table Definitions (For Both Phases)

### Queue Tables (The "Work")

**Table:** `audit_events`
| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `bigserial` | Primary Key |
| `event_type` | `text` | e.g., 'payment.completed' |
| `payload` | `jsonb` | Raw event data |
| `processed_at` | `timestamptz` | `NULL` if pending |
| `created_at` | `timestamptz` | `default NOW()` |

**Key Index (Critical for both Phases):**

```sql
CREATE INDEX idx_audit_events_pending
ON audit_events (created_at)
WHERE processed_at IS NULL;
```

*(This partial index is tiny and fast, containing only pending jobs. It's vital for Phase 1's polling performance.)*

-----

**Table:** `notification_events`
| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `bigserial` | Primary Key |
| `business_id` | `bigint` | Indexed |
| `event_type` | `text` | e.g., 'payment.failed' |
| `payload` | `jsonb` | |
| `processed_at` | `timestamptz` | `NULL` if pending |
| `created_at` | `timestamptz` | `default NOW()` |

**Key Index:**

```sql
CREATE INDEX idx_notification_events_pending
ON notification_events (created_at)
WHERE processed_at IS NULL;
```

### Destination Tables (The "Result")

**Table:** `audit_trail` (Append-Only)
| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `bigserial` | Primary Key |
| `business_id` | `bigint` | Indexed |
| `payment_id` | `bigint` | **Indexed** |
| `event_type` | `text` | Transformed type |
| `details` | `jsonb` | Transformed data |
| `source_event_id` | `bigint` | `UNIQUE`. FK to `audit_events.id` |
| `event_timestamp` | `timestamptz` | `default NOW()` |

**Key Indexes:**

```sql
CREATE INDEX idx_audit_trail_payment_id ON audit_trail (payment_id);
ALTER TABLE audit_trail ADD CONSTRAINT uq_audit_trail_source_event_id UNIQUE (source_event_id);
```

-----

**Table:** `user_notifications` (The User Inbox)
| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | `bigserial` | Primary Key |
| `user_id` | `bigint` | **Indexed** |
| `business_id` | `bigint` | |
| `event_id` | `bigint` | FK to `notification_events.id` |
| `channel` | `text` | 'email', 'push', 'web' |
| `status` | `text` | 'pending', 'sent', 'failed' |
| `content` | `jsonb` | `{"title": "...", "body": "..."}` |
| `created_at` | `timestamptz` | `default NOW()` |

**Key Indexes:**

```sql
CREATE INDEX idx_user_notifications_user_id ON user_notifications (user_id);
ALTER TABLE user_notifications ADD CONSTRAINT uq_user_notifications_event UNIQUE (event_id, user_id, channel);
```
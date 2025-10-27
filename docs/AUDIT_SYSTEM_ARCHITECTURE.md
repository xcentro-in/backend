# Audit System Architecture

## Table of Contents
1. [Overview](#overview)
2. [System Design Principles](#system-design-principles)
3. [Database Schema](#database-schema)
4. [Data Flow](#data-flow)
5. [Code Architecture](#code-architecture)
6. [Entity Types & Relationships](#entity-types--relationships)
7. [Query Patterns](#query-patterns)
8. [Complete Code Flow Examples](#complete-code-flow-examples)
9. [API Endpoints](#api-endpoints)
10. [Error Handling & Idempotency](#error-handling--idempotency)

---

## Overview

The audit system is a **normalized, queue-based architecture** that records all significant business events in the XCentro platform. It uses PostgreSQL as both the queue and permanent storage, designed for high throughput and flexible querying.

### Key Features
- ✅ **Normalized Design**: Events stored once, linked to multiple entities
- ✅ **Queue-Based Processing**: Async worker processes events without blocking API
- ✅ **Idempotent**: Safe to retry, prevents duplicate entries
- ✅ **Flexible Queries**: Query audit trail by any entity type (payment, voucher, payee, etc.)
- ✅ **Transactional**: All operations within DB transactions

---

## System Design Principles

### 1. **Decoupled Write and Read**
```
API Request → Write to Queue (Fast) → Return Response
                    ↓
            Worker Processes Queue (Async)
                    ↓
            Write to Audit Trail (Permanent)
```

### 2. **Normalized Storage**
Instead of duplicating audit events for each entity:
```
❌ BAD: Single payment with 3 vouchers = 4 duplicate audit rows
✅ GOOD: 1 audit_trail row + 4 audit_trail_entities rows (1 payment + 3 vouchers)
```

### 3. **Single Source of Truth**
PostgreSQL is the only data store (Phase 1). Redis will be added in Phase 2 for push-based notifications.

---

## Database Schema

### Table 1: `audit_events` (The Queue)

**Purpose**: Temporary inbox for events. API writes here FAST, worker processes later.

```sql
CREATE TABLE xcentro.audit_events (
    id BIGSERIAL PRIMARY KEY,
    processed_at TIMESTAMPTZ DEFAULT NULL,    -- NULL = pending, timestamp = processed
    event_type TEXT NOT NULL,                 -- e.g., "payment.initiated"
    payload JSONB NOT NULL,                   -- Full event details
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_events_pending
ON xcentro.audit_events (created_at)
WHERE processed_at IS NULL;  -- Fast lookup of pending events
```

**Payload Schema**:
```json
{
  "timestamp": "2025-10-27T10:30:00Z",
  "event_type": "payment.initiated",
  "actor": {
    "type": "user",
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "Sarah Johnson (Finance Manager)",
    "ip_address": "192.168.1.100"
  },
  "entities": {
    "business_id": "bus_123",      // REQUIRED - every event belongs to a business
    "payment_id": "pay_456",       // Optional - if event involves a payment
    "voucher_id": "v_789",         // Optional - if event involves a voucher
    "payee_id": "payee_012"        // Optional - if event involves a payee
  },
  "note": "Payment initiated for vendor invoice",
  "change_log": [
    {
      "field": "status",
      "old_value": "draft",
      "new_value": "initiated"
    }
  ]
}
```

**Lifecycle**:
1. API inserts event → `processed_at = NULL`
2. Worker picks up event → `FOR UPDATE SKIP LOCKED` (atomic)
3. Worker processes → Inserts into `audit_trail` + `audit_trail_entities`
4. Worker marks done → `processed_at = NOW()`

---

### Table 2: `audit_trail` (Permanent Event Storage)

**Purpose**: Normalized storage. Event stored ONCE, no matter how many entities it affects.

```sql
CREATE TABLE xcentro.audit_trail (
    id BIGSERIAL PRIMARY KEY,
    business_id UUID NOT NULL,

    -- The Event Data
    event_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type TEXT NOT NULL,              -- "Payment Initiated"
    actor_type TEXT NOT NULL,              -- "user" or "system"
    actor_id TEXT NOT NULL,                -- user UUID or "system"
    actor_name TEXT NOT NULL,              -- "Sarah Johnson (Finance Manager)"
    note TEXT DEFAULT NULL,
    details JSONB DEFAULT NULL,            -- {ip_address, change_log}

    -- Idempotency
    source_event_id BIGINT NOT NULL UNIQUE REFERENCES xcentro.audit_events(id)
);

CREATE INDEX idx_audit_trail_business_id
ON xcentro.audit_trail (business_id, event_timestamp DESC);
```

**Key Points**:
- ❌ **No payment_id column** (removed in normalized design)
- ✅ **Links to entities via audit_trail_entities table**
- ✅ **source_event_id UNIQUE** = prevents duplicate processing

---

### Table 3: `audit_trail_entities` (Entity Links)

**Purpose**: Many-to-many relationship between events and entities.

```sql
CREATE TABLE xcentro.audit_trail_entities (
    id BIGSERIAL PRIMARY KEY,
    business_id UUID NOT NULL,
    audit_trail_id BIGINT NOT NULL REFERENCES xcentro.audit_trail(id),

    entity_type TEXT NOT NULL,  -- 'payment', 'voucher', 'payee', 'user', 'wallet', 'payout'
    entity_id UUID NOT NULL     -- UUID of the entity
);

-- Most important index - used for entity activity feeds
CREATE INDEX idx_audit_trail_entities_lookup
ON xcentro.audit_trail_entities (entity_type, entity_id);
```

**Example Data**:
```
Audit Trail Entry (id=501): "Payment Initiated"
  ↓
audit_trail_entities rows:
  | id  | audit_trail_id | entity_type | entity_id |
  |-----|----------------|-------------|-----------|
  | 1   | 501            | payment     | pay_456   |
  | 2   | 501            | voucher     | v_789     |
  | 3   | 501            | payee       | payee_012 |
```

---

## Data Flow

### Write Path: API → Queue

```
┌─────────────┐
│   API Call  │ User initiates payment
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────────────┐
│  Business Logic (Handler/Service)       │
│  - Update payment status                │
│  - Update voucher status                │
│  - Perform business operations          │
└──────┬──────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────┐
│  Insert into audit_events (Queue)       │
│  INSERT INTO audit_events               │
│  (event_type, payload)                  │
│  VALUES ('payment.initiated', {...})    │
└──────┬──────────────────────────────────┘
       │
       ▼
┌─────────────────┐
│  COMMIT         │ Transaction completes
└─────────────────┘
       │
       ▼
┌─────────────────┐
│  Return 200 OK  │ API responds immediately
└─────────────────┘
```

**Code Location**: Business logic inserts audit events directly in transactions.

---

### Process Path: Worker → Permanent Storage

```
┌──────────────────────────────────────────────┐
│  Audit Worker (polls every N seconds)       │
│  - Runs in separate process                 │
│  - cmd/workers/audit_worker/main.go         │
└──────┬───────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────┐
│  BEGIN TRANSACTION                           │
└──────┬───────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────┐
│  SELECT from audit_events                    │
│  WHERE processed_at IS NULL                  │
│  LIMIT 100                                   │
│  FOR UPDATE SKIP LOCKED  ← Atomic lock       │
└──────┬───────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────┐
│  Transform Events → Audit Trail Entries      │
│  - Extract business_id (required)            │
│  - Parse timestamp                           │
│  - Build details JSONB                       │
└──────┬───────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────┐
│  INSERT INTO audit_trail                     │
│  RETURNING id  ← Get the new trail entry ID  │
└──────┬───────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────┐
│  Build Entity Links                          │
│  - Extract payment_id, voucher_id, etc.      │
│  - Create audit_trail_entities rows          │
└──────┬───────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────┐
│  INSERT INTO audit_trail_entities (batch)    │
│  - One row per entity reference              │
└──────┬───────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────┐
│  UPDATE audit_events                         │
│  SET processed_at = NOW()                    │
│  WHERE id IN (...)                           │
└──────┬───────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────┐
│  COMMIT TRANSACTION                          │
└──────────────────────────────────────────────┘
```

**Key Worker Features**:
- ✅ **Concurrent Safe**: Multiple workers can run in parallel (SKIP LOCKED)
- ✅ **Atomic**: All-or-nothing (transaction rollback on error)
- ✅ **Idempotent**: `source_event_id UNIQUE` prevents duplicates

---

## Code Architecture

### Layer Structure

```
┌─────────────────────────────────────────────────────────────┐
│  cmd/workers/audit_worker/main.go                           │
│  - Entry point                                              │
│  - Initializes dependencies                                 │
│  - Runs worker loop                                         │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  internal/workers/audit_worker.go                           │
│  - Minimal worker logic                                     │
│  - Calls service.ProcessPendingAuditEvents()                │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  internal/services/audit/audit_service.go                   │
│  - Business logic orchestration                             │
│  - Manages transactions                                     │
│  - Transforms events → trail entries                        │
│  - Builds entity links                                      │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  internal/core/audit/                                       │
│  - audit_entity.go: Domain entities                         │
│  - audit_repository.go: Repository interfaces               │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  internal/adapters/database/postgres/audit/                 │
│  - audit_event_repository.go: Queue operations              │
│  - audit_trail_repository.go: Trail CRUD + queries          │
│  - audit_trail_entity_repository.go: Entity links CRUD      │
└─────────────────────────────────────────────────────────────┘
```

---

## Entity Types & Relationships

### Supported Entity Types

```go
const (
    EntityTypePayment = "payment"
    EntityTypeVoucher = "voucher"
    EntityTypePayee   = "payee"
    EntityTypeUser    = "user"
    EntityTypeWallet  = "wallet"
    EntityTypePayout  = "payout"
)
```

### Entity Relationships

**Important**: Each audit event can reference MULTIPLE entity types, but only ONE instance of each type.

```
Example Event: "Payment Initiated"
  ├─ business_id: "bus_123"  ← REQUIRED
  ├─ payment_id: "pay_456"   ← Optional
  ├─ voucher_id: "v_789"     ← Optional (single voucher)
  └─ payee_id: "payee_012"   ← Optional
```

**Voucher-Payment Relationship**:
- ✅ Voucher table has `payment_id` FK → Links voucher to payment
- ✅ Audit system records individual events for each entity
- ❌ Audit payload does NOT support multiple vouchers per event (single voucher only)

**Example Scenario**:
```
Payment "pay_123" with Voucher "v_001":

Event 1: payment.initiated
  entities: { payment_id: "pay_123", voucher_id: "v_001" }
  → Creates links: payment_id=pay_123, voucher_id=v_001

Event 2: voucher.finalized
  entities: { payment_id: "pay_123", voucher_id: "v_001" }
  → Creates links: payment_id=pay_123, voucher_id=v_001

Query Results:
  GET /audit/payment/pay_123  → Returns both events
  GET /audit/voucher/v_001    → Returns both events
```

---

## Query Patterns

### 1. **Business Activity Feed**

```sql
-- Get all audit trail for a business
SELECT id, event_timestamp, event_type, actor_name, note
FROM xcentro.audit_trail
WHERE business_id = 'bus_123'
ORDER BY event_timestamp DESC
LIMIT 50 OFFSET 0;
```

**Go Code**:
```go
entries, err := auditService.GetTrailByBusinessID(ctx, "bus_123", 50, 0)
```

**API**:
```
GET /v1/audit/business/bus_123?limit=50&offset=0
```

---

### 2. **Entity Activity Feed** (Most Common)

**Payment Activity**:
```sql
-- Get all events linked to a payment
SELECT t.id, t.event_timestamp, t.event_type, t.actor_name, t.note
FROM xcentro.audit_trail AS t
INNER JOIN xcentro.audit_trail_entities AS e ON t.id = e.audit_trail_id
WHERE e.entity_type = 'payment' AND e.entity_id = 'pay_456'
ORDER BY t.event_timestamp DESC;
```

**Voucher Activity**:
```sql
-- Get all events linked to a voucher
SELECT t.id, t.event_timestamp, t.event_type, t.actor_name, t.note
FROM xcentro.audit_trail AS t
INNER JOIN xcentro.audit_trail_entities AS e ON t.id = e.audit_trail_id
WHERE e.entity_type = 'voucher' AND e.entity_id = 'v_789'
ORDER BY t.event_timestamp DESC;
```

**Go Code**:
```go
// Generic method for any entity type
entries, err := auditService.GetTrailByEntityID(ctx, "payment", "pay_456", 50, 0)
entries, err := auditService.GetTrailByEntityID(ctx, "voucher", "v_789", 50, 0)
entries, err := auditService.GetTrailByEntityID(ctx, "payee", "payee_012", 50, 0)
```

**API**:
```
GET /v1/audit/payment/pay_456
GET /v1/audit/voucher/v_789
GET /v1/audit/payee/payee_012
```

---

### 3. **Combined Activity Feed** (Voucher + Payment)

If you need to show BOTH voucher events AND payment events in a single feed:

```go
// Step 1: Get voucher's payment_id from voucher table
var paymentID string
db.QueryRow("SELECT payment_id FROM xcentro.vouchers WHERE id = $1", voucherID).Scan(&paymentID)

// Step 2: Query both voucher and payment audit trails
query := `
    (
        -- Voucher events
        SELECT t.* FROM xcentro.audit_trail AS t
        JOIN xcentro.audit_trail_entities AS e ON t.id = e.audit_trail_id
        WHERE e.entity_type = 'voucher' AND e.entity_id = $1
    )
    UNION
    (
        -- Payment events
        SELECT t.* FROM xcentro.audit_trail AS t
        JOIN xcentro.audit_trail_entities AS e ON t.id = e.audit_trail_id
        WHERE e.entity_type = 'payment' AND e.entity_id = $2
    )
    ORDER BY event_timestamp DESC
    LIMIT $3 OFFSET $4
`
```

**Note**: This UNION pattern is needed because the voucher table has the payment_id FK, not the audit system.

---

## Complete Code Flow Examples

### Example 1: Payment Initiation Flow

#### Step 1: Business Logic (Handler/Service)

```go
// File: internal/services/payments/payment_service.go

func (s *PaymentService) InitiatePayment(ctx context.Context, req *PaymentRequest) error {
    return s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
        // 1. Create payment record
        payment := &domain.Payment{
            ID:         uuid.New(),
            BusinessID: req.BusinessID,
            Amount:     req.Amount,
            Status:     "initiated",
        }
        err := s.paymentRepo.Create(ctx, payment)
        if err != nil {
            return err
        }

        // 2. Insert audit event (same transaction)
        auditEvent := &audit.AuditEvent{
            EventType: audit.EventPaymentInitiated,
            Payload: audit.AuditEventPayload{
                Timestamp: time.Now().Format(time.RFC3339),
                EventType: "payment.initiated",
                Actor: audit.ActorInfo{
                    Type:      "user",
                    ID:        req.UserID,
                    Name:      req.UserName,
                    IPAddress: &req.IPAddress,
                },
                Entities: audit.EntityReferences{
                    BusinessID: req.BusinessID,
                    PaymentID:  &payment.ID.String(),
                },
                Note: strPtr("Payment initiated via web portal"),
            },
        }

        err = s.auditEventRepo.CreateEvent(ctx, auditEvent)
        if err != nil {
            return err // Transaction rolls back
        }

        // 3. Commit transaction
        return nil
    })
}
```

---

#### Step 2: Worker Processes Event

```go
// File: internal/services/audit/audit_service.go

func (s *AuditService) ProcessPendingAuditEvents(ctx context.Context, batchSize int) (int, error) {
    var processedCount int

    err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
        // 1. Get transactional repos
        txEventRepo := s.auditEventRepo.WithTx(tx)
        txTrailRepo := s.auditTrailRepo.WithTx(tx)
        txEntityRepo := s.auditTrailEntityRepo.WithTx(tx)

        // 2. Fetch pending events (FOR UPDATE SKIP LOCKED)
        events, err := txEventRepo.FetchPendingEvents(ctx, batchSize)
        if err != nil {
            return fmt.Errorf("failed to fetch pending events: %w", err)
        }

        if len(events) == 0 {
            return nil // No events to process
        }

        // 3. Process each event
        var eventIDs []int64
        for _, event := range events {
            // Transform to trail entry
            entry := transformEventToTrailEntry(event)
            if entry == nil {
                continue // Skip invalid events
            }

            // Insert audit trail entry (gets ID back)
            err := txTrailRepo.CreateTrailEntry(ctx, entry)
            if err != nil {
                return fmt.Errorf("failed to create trail entry: %w", err)
            }

            // Build entity links
            entityLinks := buildEntityLinks(event, entry.ID, entry.BusinessID)

            // Insert entity links
            if len(entityLinks) > 0 {
                err := txEntityRepo.CreateEntityLinks(ctx, entityLinks)
                if err != nil {
                    return fmt.Errorf("failed to create entity links: %w", err)
                }
            }

            eventIDs = append(eventIDs, event.ID)
        }

        // 4. Mark events as processed
        if len(eventIDs) > 0 {
            err := txEventRepo.MarkEventsProcessed(ctx, eventIDs)
            if err != nil {
                return fmt.Errorf("failed to mark events as processed: %w", err)
            }
        }

        processedCount = len(eventIDs)
        return nil
    })

    return processedCount, err
}
```

---

#### Step 3: Transform Event → Trail Entry

```go
// File: internal/services/audit/audit_service.go

func transformEventToTrailEntry(event *audit.AuditEvent) *audit.AuditTrailEntry {
    payload := event.Payload

    // Validate required business_id
    if payload.Entities.BusinessID == "" {
        return nil // Skip invalid events
    }

    // Parse event timestamp
    eventTime, err := time.Parse(time.RFC3339, payload.Timestamp)
    if err != nil {
        eventTime = event.CreatedAt // Fallback to queue insertion time
    }

    // Build details JSONB
    details := make(map[string]any)
    if payload.Actor.IPAddress != nil {
        details["ip_address"] = *payload.Actor.IPAddress
    }
    if len(payload.ChangeLog) > 0 {
        details["change_log"] = payload.ChangeLog
    }

    return &audit.AuditTrailEntry{
        BusinessID:     payload.Entities.BusinessID,
        EventTimestamp: eventTime,
        EventType:      payload.EventType,
        ActorType:      payload.Actor.Type,
        ActorID:        payload.Actor.ID,
        ActorName:      payload.Actor.Name,
        Note:           payload.Note,
        Details:        details,
        SourceEventID:  event.ID,
    }
}
```

---

#### Step 4: Build Entity Links

```go
// File: internal/services/audit/audit_service.go

func buildEntityLinks(event *audit.AuditEvent, auditTrailID int64, businessID string) []*audit.AuditTrailEntity {
    var links []*audit.AuditTrailEntity
    entities := event.Payload.Entities

    // Link to payment (if present)
    if entities.PaymentID != nil {
        links = append(links, &audit.AuditTrailEntity{
            BusinessID:   businessID,
            AuditTrailID: auditTrailID,
            EntityType:   audit.EntityTypePayment,
            EntityID:     *entities.PaymentID,
        })
    }

    // Link to voucher (if present)
    if entities.VoucherID != nil {
        links = append(links, &audit.AuditTrailEntity{
            BusinessID:   businessID,
            AuditTrailID: auditTrailID,
            EntityType:   audit.EntityTypeVoucher,
            EntityID:     *entities.VoucherID,
        })
    }

    // Link to payee (if present)
    if entities.PayeeID != nil {
        links = append(links, &audit.AuditTrailEntity{
            BusinessID:   businessID,
            AuditTrailID: auditTrailID,
            EntityType:   audit.EntityTypePayee,
            EntityID:     *entities.PayeeID,
        })
    }

    // Link to user (if present)
    if entities.UserID != nil {
        links = append(links, &audit.AuditTrailEntity{
            BusinessID:   businessID,
            AuditTrailID: auditTrailID,
            EntityType:   audit.EntityTypeUser,
            EntityID:     *entities.UserID,
        })
    }

    // Link to wallet (if present)
    if entities.WalletID != nil {
        links = append(links, &audit.AuditTrailEntity{
            BusinessID:   businessID,
            AuditTrailID: auditTrailID,
            EntityType:   audit.EntityTypeWallet,
            EntityID:     *entities.WalletID,
        })
    }

    return links
}
```

---

### Example 2: Querying Audit Trail

#### API Handler

```go
// File: internal/handlers/http/audit/audit_handler.go

// GetEntityAuditTrail handles GET /v1/audit/:entity_type/:entity_id
func (h *AuditHandler) GetEntityAuditTrail(c echo.Context) error {
    entityType := c.Param("entity_type")
    entityID := c.Param("entity_id")

    if entityType == "" {
        return c.JSON(http.StatusBadRequest, map[string]any{
            "error":   "MISSING_ENTITY_TYPE",
            "message": "Entity type is required (payment, voucher, payee, user, wallet, payout)",
        })
    }

    if entityID == "" {
        return c.JSON(http.StatusBadRequest, map[string]any{
            "error":   "MISSING_ENTITY_ID",
            "message": "Entity ID is required",
        })
    }

    // Parse pagination
    limit, _ := strconv.Atoi(c.QueryParam("limit"))
    if limit <= 0 || limit > 100 {
        limit = 50
    }

    offset, _ := strconv.Atoi(c.QueryParam("offset"))
    if offset < 0 {
        offset = 0
    }

    // Get audit trail
    trail, err := h.auditService.GetTrailByEntityID(
        c.Request().Context(),
        entityType,
        entityID,
        limit,
        offset,
    )
    if err != nil {
        return c.JSON(http.StatusInternalServerError, map[string]any{
            "error":   "FETCH_FAILED",
            "message": "Failed to fetch audit trail",
        })
    }

    return c.JSON(http.StatusOK, map[string]any{
        "message": "Audit trail retrieved successfully",
        "data": map[string]any{
            "entity_type": entityType,
            "entity_id":   entityID,
            "entries":     trail,
            "limit":       limit,
            "offset":      offset,
        },
    })
}
```

#### Repository Implementation

```go
// File: internal/adapters/database/postgres/audit/audit_trail_repository.go

func (r *PgAuditTrailRepository) GetTrailByEntityID(
    ctx context.Context,
    entityType, entityID string,
    limit, offset int,
) ([]*audit.AuditTrailEntry, error) {
    query := `
        SELECT t.id, t.business_id, t.event_timestamp, t.event_type,
               t.actor_type, t.actor_id, t.actor_name, t.note, t.details, t.source_event_id
        FROM xcentro.audit_trail AS t
        INNER JOIN xcentro.audit_trail_entities AS e ON t.id = e.audit_trail_id
        WHERE e.entity_type = $1 AND e.entity_id = $2
        ORDER BY t.event_timestamp DESC
        LIMIT $3 OFFSET $4
    `

    rows, err := r.querier.Query(ctx, query, entityType, entityID, limit, offset)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch audit trail for entity: %w", err)
    }
    defer rows.Close()

    return r.scanTrailEntries(rows)
}
```

---

## API Endpoints

### 1. Get Business Audit Trail

```
GET /v1/audit/business/:business_id?limit=50&offset=0
```

**Response**:
```json
{
  "message": "Audit trail retrieved successfully",
  "data": {
    "business_id": "bus_123",
    "entries": [
      {
        "id": 501,
        "business_id": "bus_123",
        "event_timestamp": "2025-10-27T10:30:00Z",
        "event_type": "payment.initiated",
        "actor_type": "user",
        "actor_id": "550e8400-e29b-41d4-a716-446655440000",
        "actor_name": "Sarah Johnson (Finance Manager)",
        "note": "Payment initiated via web portal",
        "details": {
          "ip_address": "192.168.1.100",
          "change_log": [
            {
              "field": "status",
              "old_value": "draft",
              "new_value": "initiated"
            }
          ]
        },
        "source_event_id": 1234
      }
    ],
    "limit": 50,
    "offset": 0
  }
}
```

---

### 2. Get Entity Audit Trail

```
GET /v1/audit/:entity_type/:entity_id?limit=50&offset=0
```

**Supported Entity Types**:
- `payment` - Payment activity
- `voucher` - Voucher activity
- `payee` - Payee activity
- `user` - User activity
- `wallet` - Wallet activity
- `payout` - Payout activity

**Examples**:
```bash
# Payment activity
GET /v1/audit/payment/550e8400-e29b-41d4-a716-446655440000

# Voucher activity
GET /v1/audit/voucher/660f9511-f39c-52e5-b827-557766551111

# Payee activity
GET /v1/audit/payee/770fa622-g40d-63f6-c938-668877662222
```

**Response**: Same as business audit trail response

---

## Error Handling & Idempotency

### Idempotency Guarantees

#### 1. **Unique Source Event ID**

```sql
ALTER TABLE xcentro.audit_trail
ADD CONSTRAINT uq_audit_trail_source_event_id
UNIQUE (source_event_id);
```

**What this prevents**:
- ❌ Processing same event twice
- ❌ Duplicate audit trail entries
- ✅ Safe to retry worker processing

#### 2. **Transaction Rollback**

If any step fails during processing:
```
BEGIN;
  1. Insert audit_trail entry ✅
  2. Insert entity links ❌ FAILS
  3. Mark event as processed ⏭️ SKIPPED
ROLLBACK; ← Everything undone
```

Next worker run will retry the same event.

---

### Error Scenarios

#### Scenario 1: Worker Crashes Mid-Process

```
Worker picks up Event ID=1234 (FOR UPDATE SKIP LOCKED)
  → Inserts audit_trail entry
  → Process crashes before marking processed_at

Result:
  ✅ Transaction rolled back (processed_at still NULL)
  ✅ Another worker will pick it up
  ✅ source_event_id UNIQUE prevents duplicate trail entry
```

---

#### Scenario 2: Invalid Event Data

```go
func transformEventToTrailEntry(event *audit.AuditEvent) *audit.AuditTrailEntry {
    // Validate required business_id
    if payload.Entities.BusinessID == "" {
        return nil // Skip invalid events
    }
    // ...
}
```

**Result**:
- ⚠️ Event skipped (not marked as processed)
- ⚠️ Will be retried on next run
- 💡 Need monitoring/alerting for repeatedly failing events

---

#### Scenario 3: Database Constraint Violation

```sql
-- Foreign key violation example
INSERT INTO xcentro.audit_trail_entities (entity_id)
VALUES ('non-existent-payment-id');  -- FAILS if payment doesn't exist
```

**Result**:
- ❌ Transaction rolled back
- ✅ Event remains in queue (processed_at = NULL)
- ✅ Will be retried

---

### Monitoring Points

**Key Metrics to Track**:
1. Queue depth: `SELECT COUNT(*) FROM audit_events WHERE processed_at IS NULL`
2. Processing rate: Events processed per minute
3. Error rate: Failed transaction count
4. Lag: `MAX(created_at) - NOW()` for pending events

**Alerts**:
- 🚨 Queue depth > 10,000 (backlog building)
- 🚨 Lag > 5 minutes (worker not keeping up)
- 🚨 Same event ID failing repeatedly (data issue)

---

## Summary

### Key Takeaways

1. **Normalized Design**:
   - Events stored once in `audit_trail`
   - Linked to entities via `audit_trail_entities`
   - Efficient storage and flexible querying

2. **Queue-Based Processing**:
   - Fast API writes to `audit_events`
   - Async worker processes events
   - No blocking on audit trail writes

3. **Single Entity References**:
   - ❌ NOT bulk operations (one voucher per event)
   - ✅ Multiple entity types per event (payment + voucher + payee)
   - ✅ Voucher-payment relationship handled in voucher table

4. **Idempotent & Safe**:
   - UNIQUE constraint on source_event_id
   - Transactional processing
   - Safe to retry

5. **Flexible Queries**:
   - Query by business
   - Query by any entity type
   - Efficient indexes

### Migration Path

**Phase 1 (Current)**: PostgreSQL-only queue
**Phase 2 (Future)**: Add Redis for push-based notifications
- API writes to Postgres, then LPUSH to Redis
- Worker does BLPOP on Redis (instant processing)
- Sweeper finds orphaned events (Redis miss due to crash)

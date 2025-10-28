# Audit System Architecture - COMPLETE IMPLEMENTATION ARCHIVE

> **STATUS**: This audit system has been REMOVED from the codebase.
> **PURPOSE**: This document preserves the complete implementation for future reference.
> **RESTORATION**: Use this document to re-implement the audit system with minimal prompting.

## ⚠️ REMOVAL NOTICE

**Date Removed**: 2025-10-28
**Reason**: Feature removed from product scope
**Preserved Files**: All code, database schemas, migrations, and documentation archived below

---

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
11. [Complete Source Code Archive](#complete-source-code-archive)

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

---

## Complete Source Code Archive

This section preserves all source code files for the audit system. Use these to restore the system if needed in the future.

### File Structure

```
internal/
├── core/audit/
│   ├── audit_entity.go          # Domain entities and constants
│   └── audit_repository.go      # Repository interfaces
├── services/audit/
│   └── audit_service.go         # Business logic orchestration
├── adapters/database/postgres/audit/
│   ├── audit_event_repository.go         # Queue operations
│   ├── audit_trail_repository.go         # Trail CRUD + queries
│   └── audit_trail_entity_repository.go  # Entity links
├── handlers/http/audit/
│   └── audit_handler.go         # HTTP API handlers
└── workers/
    └── audit_worker.go          # Worker implementation

cmd/workers/audit_worker/
└── main.go                      # Worker entry point

internal/adapters/database/postgres/migrations/sql/
├── 000023_create_audit_notification_tables.up.sql
└── 000023_create_audit_notification_tables.down.sql
```

---

### 1. Core Domain - `internal/core/audit/audit_entity.go`

```go
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
```

---

### 2. Core Interfaces - `internal/core/audit/audit_repository.go`

```go
package audit

import (
	"context"

	"xcentro.in/internal/adapters/database/postgres/client"
)

// AuditEventRepository handles audit event queue operations
type AuditEventRepository interface {
	// CreateEvent adds a new audit event to the queue
	CreateEvent(ctx context.Context, event *AuditEvent) error

	// FetchPendingEvents retrieves pending events for processing (with FOR UPDATE SKIP LOCKED)
	FetchPendingEvents(ctx context.Context, limit int) ([]*AuditEvent, error)

	// MarkEventsProcessed marks events as processed
	MarkEventsProcessed(ctx context.Context, eventIDs []int64) error

	// WithTx creates a new repository instance that uses a transaction
	WithTx(tx client.Tx) AuditEventRepository
}

// AuditTrailRepository handles audit trail operations
type AuditTrailRepository interface {
	// CreateTrailEntry adds a new entry to the permanent audit trail
	CreateTrailEntry(ctx context.Context, entry *AuditTrailEntry) error

	// CreateTrailEntries batch inserts multiple entries
	CreateTrailEntries(ctx context.Context, entries []*AuditTrailEntry) error

	// GetTrailByBusinessID retrieves audit trail for a business (sorted by timestamp DESC)
	GetTrailByBusinessID(ctx context.Context, businessID string, limit, offset int) ([]*AuditTrailEntry, error)

	// GetTrailByEntityID retrieves audit trail for a specific entity (payment, voucher, payee, etc.)
	GetTrailByEntityID(ctx context.Context, entityType, entityID string, limit, offset int) ([]*AuditTrailEntry, error)

	// WithTx creates a new repository instance that uses a transaction
	WithTx(tx client.Tx) AuditTrailRepository
}

// AuditTrailEntityRepository handles audit trail entity link operations
type AuditTrailEntityRepository interface {
	// CreateEntityLinks creates links between an audit trail entry and entities
	CreateEntityLinks(ctx context.Context, entities []*AuditTrailEntity) error

	// GetEntitiesByAuditTrailID retrieves all entity links for an audit trail entry
	GetEntitiesByAuditTrailID(ctx context.Context, auditTrailID int64) ([]*AuditTrailEntity, error)

	// WithTx creates a new repository instance that uses a transaction
	WithTx(tx client.Tx) AuditTrailEntityRepository
}
```

---

### 3. Service Layer - `internal/services/audit/audit_service.go`

```go
package audit

import (
	"context"
	"fmt"
	"time"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/audit"
	"xcentro.in/internal/shared/logger"
)

// AuditService handles audit trail operations
type AuditService struct {
	auditEventRepo       audit.AuditEventRepository
	auditTrailRepo       audit.AuditTrailRepository
	auditTrailEntityRepo audit.AuditTrailEntityRepository
	txManager            client.TxManager
	logger               *logger.Logger
}

// NewAuditService creates a new audit service
func NewAuditService(
	auditEventRepo audit.AuditEventRepository,
	auditTrailRepo audit.AuditTrailRepository,
	auditTrailEntityRepo audit.AuditTrailEntityRepository,
	txManager client.TxManager,
	logger *logger.Logger,
) *AuditService {
	return &AuditService{
		auditEventRepo:       auditEventRepo,
		auditTrailRepo:       auditTrailRepo,
		auditTrailEntityRepo: auditTrailEntityRepo,
		txManager:            txManager,
		logger:               logger,
	}
}

// ProcessPendingAuditEvents fetches, transforms, and processes audit events in a transaction
// This is called by the audit worker
func (s *AuditService) ProcessPendingAuditEvents(ctx context.Context, batchSize int) (int, error) {
	var processedCount int

	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Create transactional repository instances
		txEventRepo := s.auditEventRepo.WithTx(tx)
		txTrailRepo := s.auditTrailRepo.WithTx(tx)
		txEntityRepo := s.auditTrailEntityRepo.WithTx(tx)

		// Fetch pending events with FOR UPDATE SKIP LOCKED (within transaction)
		events, err := txEventRepo.FetchPendingEvents(ctx, batchSize)
		if err != nil {
			return fmt.Errorf("failed to fetch pending events: %w", err)
		}

		if len(events) == 0 {
			return nil // No events to process
		}

		// Process each event: insert trail entry + entity links
		var eventIDs []int64

		for _, event := range events {
			// Transform event to audit trail entry
			entry := transformEventToTrailEntry(event)
			if entry == nil {
				continue // Skip invalid events
			}

			// Insert the audit trail entry and get its ID
			if err := txTrailRepo.CreateTrailEntry(ctx, entry); err != nil {
				return fmt.Errorf("failed to create trail entry for event %d: %w", event.ID, err)
			}

			// Build entity links from the event payload
			entityLinks := buildEntityLinks(event, entry.ID, entry.BusinessID)

			// Insert entity links
			if len(entityLinks) > 0 {
				if err := txEntityRepo.CreateEntityLinks(ctx, entityLinks); err != nil {
					return fmt.Errorf("failed to create entity links for event %d: %w", event.ID, err)
				}
			}

			eventIDs = append(eventIDs, event.ID)
		}

		// Mark events as processed
		if len(eventIDs) > 0 {
			if err := txEventRepo.MarkEventsProcessed(ctx, eventIDs); err != nil {
				return fmt.Errorf("failed to mark events as processed: %w", err)
			}
		}

		processedCount = len(eventIDs)
		return nil
	})

	if err != nil {
		return 0, err
	}

	return processedCount, nil
}

// GetTrailByBusinessID retrieves audit trail for a business
func (s *AuditService) GetTrailByBusinessID(ctx context.Context, businessID string, limit, offset int) ([]*audit.AuditTrailEntry, error) {
	return s.auditTrailRepo.GetTrailByBusinessID(ctx, businessID, limit, offset)
}

// GetTrailByEntityID retrieves audit trail for a specific entity (payment, voucher, payee, etc.)
func (s *AuditService) GetTrailByEntityID(ctx context.Context, entityType, entityID string, limit, offset int) ([]*audit.AuditTrailEntry, error) {
	return s.auditTrailRepo.GetTrailByEntityID(ctx, entityType, entityID, limit, offset)
}

// transformEventToTrailEntry transforms an audit event to a trail entry
// Note: payment_id removed from audit_trail (normalized design)
func transformEventToTrailEntry(event *audit.AuditEvent) *audit.AuditTrailEntry {
	payload := event.Payload

	// Validate required business_id
	if payload.Entities.BusinessID == "" {
		return nil
	}

	// Parse event timestamp
	eventTime, err := time.Parse(time.RFC3339, payload.Timestamp)
	if err != nil {
		eventTime = event.CreatedAt
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

// buildEntityLinks builds entity links from the event payload
// This extracts all entity references and creates audit_trail_entities rows
func buildEntityLinks(event *audit.AuditEvent, auditTrailID int64, businessID string) []*audit.AuditTrailEntity {
	var links []*audit.AuditTrailEntity
	entities := event.Payload.Entities

	// Link to payment
	if entities.PaymentID != nil {
		links = append(links, &audit.AuditTrailEntity{
			BusinessID:   businessID,
			AuditTrailID: auditTrailID,
			EntityType:   audit.EntityTypePayment,
			EntityID:     *entities.PaymentID,
		})
	}

	// Link to voucher
	if entities.VoucherID != nil {
		links = append(links, &audit.AuditTrailEntity{
			BusinessID:   businessID,
			AuditTrailID: auditTrailID,
			EntityType:   audit.EntityTypeVoucher,
			EntityID:     *entities.VoucherID,
		})
	}

	// Link to payee
	if entities.PayeeID != nil {
		links = append(links, &audit.AuditTrailEntity{
			BusinessID:   businessID,
			AuditTrailID: auditTrailID,
			EntityType:   audit.EntityTypePayee,
			EntityID:     *entities.PayeeID,
		})
	}

	// Link to user
	if entities.UserID != nil {
		links = append(links, &audit.AuditTrailEntity{
			BusinessID:   businessID,
			AuditTrailID: auditTrailID,
			EntityType:   audit.EntityTypeUser,
			EntityID:     *entities.UserID,
		})
	}

	// Link to wallet
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

### 4. Repository - Queue Operations - `internal/adapters/database/postgres/audit/audit_event_repository.go`

```go
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/audit"
	"xcentro.in/internal/shared/logger"
)

// PgAuditEventRepository implements audit.AuditEventRepository using PostgreSQL
type PgAuditEventRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgAuditEventRepository creates a new PostgreSQL audit event repository
func NewPgAuditEventRepository(db client.DB) *PgAuditEventRepository {
	return &PgAuditEventRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// CreateEvent adds a new audit event to the queue
func (r *PgAuditEventRepository) CreateEvent(ctx context.Context, event *audit.AuditEvent) error {
	query := `
		INSERT INTO xcentro.audit_events (event_type, payload)
		VALUES ($1, $2)
		RETURNING id, created_at
	`

	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	err = r.querier.QueryRow(ctx, query, event.EventType, payloadJSON).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create audit event: %w", err)
	}

	return nil
}

// FetchPendingEvents retrieves pending events for processing with FOR UPDATE SKIP LOCKED
func (r *PgAuditEventRepository) FetchPendingEvents(ctx context.Context, limit int) ([]*audit.AuditEvent, error) {
	query := `
		SELECT id, event_type, payload, created_at
		FROM xcentro.audit_events
		WHERE processed_at IS NULL
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := r.querier.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending events: %w", err)
	}
	defer rows.Close()

	var events []*audit.AuditEvent
	for rows.Next() {
		var event audit.AuditEvent
		var payloadJSON []byte

		err := rows.Scan(&event.ID, &event.EventType, &payloadJSON, &event.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit event: %w", err)
		}

		if err := json.Unmarshal(payloadJSON, &event.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		events = append(events, &event)
	}

	return events, nil
}

// MarkEventsProcessed marks events as processed
func (r *PgAuditEventRepository) MarkEventsProcessed(ctx context.Context, eventIDs []int64) error {
	query := `
		UPDATE xcentro.audit_events
		SET processed_at = NOW()
		WHERE id = ANY($1)
	`

	_, err := r.querier.Exec(ctx, query, eventIDs)
	if err != nil {
		return fmt.Errorf("failed to mark events as processed: %w", err)
	}

	return nil
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgAuditEventRepository) WithTx(tx client.Tx) audit.AuditEventRepository {
	return &PgAuditEventRepository{
		querier: tx,
		logger:  r.logger,
	}
}
```

---

### 5. Repository - Trail Storage - `internal/adapters/database/postgres/audit/audit_trail_repository.go`

```go
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/audit"
	"xcentro.in/internal/shared/logger"
)

// PgAuditTrailRepository implements audit.AuditTrailRepository using PostgreSQL
type PgAuditTrailRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgAuditTrailRepository creates a new PostgreSQL audit trail repository
func NewPgAuditTrailRepository(db client.DB) *PgAuditTrailRepository {
	return &PgAuditTrailRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// CreateTrailEntry adds a new entry to the permanent audit trail
func (r *PgAuditTrailRepository) CreateTrailEntry(ctx context.Context, entry *audit.AuditTrailEntry) error {
	query := `
		INSERT INTO xcentro.audit_trail (
			business_id, event_timestamp, event_type,
			actor_type, actor_id, actor_name, note, details, source_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (source_event_id) DO NOTHING
		RETURNING id
	`

	var detailsJSON []byte
	if entry.Details != nil {
		var err error
		detailsJSON, err = json.Marshal(entry.Details)
		if err != nil {
			return fmt.Errorf("failed to marshal details: %w", err)
		}
	}

	err := r.querier.QueryRow(ctx, query,
		entry.BusinessID,
		entry.EventTimestamp,
		entry.EventType,
		entry.ActorType,
		entry.ActorID,
		entry.ActorName,
		entry.Note,
		detailsJSON,
		entry.SourceEventID,
	).Scan(&entry.ID)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Conflict occurred, entry already exists (idempotency)
			return nil
		}
		return fmt.Errorf("failed to create audit trail entry: %w", err)
	}

	return nil
}

// CreateTrailEntries batch inserts multiple entries
func (r *PgAuditTrailRepository) CreateTrailEntries(ctx context.Context, entries []*audit.AuditTrailEntry) error {
	if len(entries) == 0 {
		return nil
	}

	query := `
		INSERT INTO xcentro.audit_trail (
			business_id, event_timestamp, event_type,
			actor_type, actor_id, actor_name, note, details, source_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (source_event_id) DO NOTHING
	`

	// Insert each entry individually (idempotent due to ON CONFLICT)
	for _, entry := range entries {
		var detailsJSON []byte
		if entry.Details != nil {
			var err error
			detailsJSON, err = json.Marshal(entry.Details)
			if err != nil {
				return fmt.Errorf("failed to marshal details: %w", err)
			}
		}

		_, err := r.querier.Exec(ctx, query,
			entry.BusinessID,
			entry.EventTimestamp,
			entry.EventType,
			entry.ActorType,
			entry.ActorID,
			entry.ActorName,
			entry.Note,
			detailsJSON,
			entry.SourceEventID,
		)
		if err != nil {
			return fmt.Errorf("failed to insert audit trail entry: %w", err)
		}
	}

	return nil
}

// GetTrailByBusinessID retrieves audit trail for a business (sorted by timestamp DESC)
func (r *PgAuditTrailRepository) GetTrailByBusinessID(ctx context.Context, businessID string, limit, offset int) ([]*audit.AuditTrailEntry, error) {
	query := `
		SELECT id, business_id, event_timestamp, event_type,
		       actor_type, actor_id, actor_name, note, details, source_event_id
		FROM xcentro.audit_trail
		WHERE business_id = $1
		ORDER BY event_timestamp DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.querier.Query(ctx, query, businessID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch audit trail: %w", err)
	}
	defer rows.Close()

	return r.scanTrailEntries(rows)
}

// GetTrailByEntityID retrieves audit trail for a specific entity (payment, voucher, payee, etc.)
// Uses the normalized design with JOIN to audit_trail_entities
func (r *PgAuditTrailRepository) GetTrailByEntityID(ctx context.Context, entityType, entityID string, limit, offset int) ([]*audit.AuditTrailEntry, error) {
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

// scanTrailEntries is a helper to scan rows into AuditTrailEntry slice
func (r *PgAuditTrailRepository) scanTrailEntries(rows pgx.Rows) ([]*audit.AuditTrailEntry, error) {
	var entries []*audit.AuditTrailEntry

	for rows.Next() {
		var entry audit.AuditTrailEntry
		var note *string
		var detailsJSON []byte

		err := rows.Scan(
			&entry.ID,
			&entry.BusinessID,
			&entry.EventTimestamp,
			&entry.EventType,
			&entry.ActorType,
			&entry.ActorID,
			&entry.ActorName,
			&note,
			&detailsJSON,
			&entry.SourceEventID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit trail entry: %w", err)
		}

		entry.Note = note

		if detailsJSON != nil {
			if err := json.Unmarshal(detailsJSON, &entry.Details); err != nil {
				return nil, fmt.Errorf("failed to unmarshal details: %w", err)
			}
		}

		entries = append(entries, &entry)
	}

	return entries, nil
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgAuditTrailRepository) WithTx(tx client.Tx) audit.AuditTrailRepository {
	return &PgAuditTrailRepository{
		querier: tx,
		logger:  r.logger,
	}
}
```

---

### 6. Repository - Entity Links - `internal/adapters/database/postgres/audit/audit_trail_entity_repository.go`

```go
package audit

import (
	"context"
	"fmt"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/audit"
	"xcentro.in/internal/shared/logger"
)

// PgAuditTrailEntityRepository implements audit.AuditTrailEntityRepository using PostgreSQL
type PgAuditTrailEntityRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgAuditTrailEntityRepository creates a new PostgreSQL audit trail entity repository
func NewPgAuditTrailEntityRepository(db client.DB) *PgAuditTrailEntityRepository {
	return &PgAuditTrailEntityRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// CreateEntityLinks creates links between an audit trail entry and entities
func (r *PgAuditTrailEntityRepository) CreateEntityLinks(ctx context.Context, entities []*audit.AuditTrailEntity) error {
	if len(entities) == 0 {
		return nil
	}

	query := `
		INSERT INTO xcentro.audit_trail_entities (
			business_id, audit_trail_id, entity_type, entity_id
		) VALUES ($1, $2, $3, $4)
	`

	// Insert entity links (within same transaction as audit trail entry)
	for _, entity := range entities {
		_, err := r.querier.Exec(ctx, query,
			entity.BusinessID,
			entity.AuditTrailID,
			entity.EntityType,
			entity.EntityID,
		)
		if err != nil {
			return fmt.Errorf("failed to insert entity link (type=%s, id=%s): %w",
				entity.EntityType, entity.EntityID, err)
		}
	}

	return nil
}

// GetEntitiesByAuditTrailID retrieves all entity links for an audit trail entry
func (r *PgAuditTrailEntityRepository) GetEntitiesByAuditTrailID(ctx context.Context, auditTrailID int64) ([]*audit.AuditTrailEntity, error) {
	query := `
		SELECT id, business_id, audit_trail_id, entity_type, entity_id
		FROM xcentro.audit_trail_entities
		WHERE audit_trail_id = $1
		ORDER BY entity_type, entity_id
	`

	rows, err := r.querier.Query(ctx, query, auditTrailID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch entity links: %w", err)
	}
	defer rows.Close()

	var entities []*audit.AuditTrailEntity
	for rows.Next() {
		var entity audit.AuditTrailEntity
		err := rows.Scan(
			&entity.ID,
			&entity.BusinessID,
			&entity.AuditTrailID,
			&entity.EntityType,
			&entity.EntityID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entity link: %w", err)
		}
		entities = append(entities, &entity)
	}

	return entities, nil
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgAuditTrailEntityRepository) WithTx(tx client.Tx) audit.AuditTrailEntityRepository {
	return &PgAuditTrailEntityRepository{
		querier: tx,
		logger:  r.logger,
	}
}
```

---

### 7. HTTP Handlers - `internal/handlers/http/audit/audit_handler.go`

```go
package audit

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/services/audit"
)

// AuditHandler handles audit trail HTTP requests
type AuditHandler struct {
	auditService *audit.AuditService
}

// NewAuditHandler creates a new audit handler
func NewAuditHandler(auditService *audit.AuditService) *AuditHandler {
	return &AuditHandler{
		auditService: auditService,
	}
}

// GetBusinessAuditTrail handles GET /v1/audit/business/:business_id
func (h *AuditHandler) GetBusinessAuditTrail(c echo.Context) error {
	businessID := c.Param("business_id")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "Business ID is required",
		})
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50 // Default limit
	}

	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if offset < 0 {
		offset = 0
	}

	// Get audit trail
	trail, err := h.auditService.GetTrailByBusinessID(c.Request().Context(), businessID, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "FETCH_FAILED",
			"message": "Failed to fetch audit trail",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Audit trail retrieved successfully",
		"data": map[string]any{
			"business_id": businessID,
			"entries":     trail,
			"limit":       limit,
			"offset":      offset,
		},
	})
}

// GetEntityAuditTrail handles GET /v1/audit/:entity_type/:entity_id
// Supports: payment, voucher, payee, user, wallet, payout
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

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50 // Default limit
	}

	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if offset < 0 {
		offset = 0
	}

	// Get audit trail for the entity
	trail, err := h.auditService.GetTrailByEntityID(c.Request().Context(), entityType, entityID, limit, offset)
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

---

### 8. Worker Implementation - `internal/workers/audit_worker.go`

```go
package workers

import (
	"context"
	"fmt"

	auditService "xcentro.in/internal/services/audit"
)

// AuditWorker processes audit events from the queue
// PHASE2_TODO: Add redisClient field for push-based processing (BLPOP instead of polling)
type AuditWorker struct {
	auditService *auditService.AuditService
	batchSize    int
	// PHASE2_TODO: Add field: redisClient *redis.Client
}

// NewAuditWorker creates a new audit worker
func NewAuditWorker(
	auditService *auditService.AuditService,
	batchSize int,
) *AuditWorker {
	return &AuditWorker{
		auditService: auditService,
		batchSize:    batchSize,
	}
}

// Run executes the worker cycle
// Worker is kept minimal - all business logic is in the service
// PHASE2_TODO: This will be called when an event ID arrives via Redis BLPOP
func (w *AuditWorker) Run(ctx context.Context) error {
	fmt.Println("=== AUDIT WORKER: Starting run ===")

	// Call service to process pending events (service handles transaction, transformation, etc.)
	processedCount, err := w.auditService.ProcessPendingAuditEvents(ctx, w.batchSize)
	if err != nil {
		fmt.Printf("❌ Error processing audit events: %v\n", err)
		return fmt.Errorf("failed to process audit events: %w", err)
	}

	if processedCount == 0 {
		fmt.Println("ℹ️  No pending audit events")
	} else {
		fmt.Printf("✅ Successfully processed %d audit events\n", processedCount)
	}

	fmt.Println("=== AUDIT WORKER: Run complete ===")
	return nil
}
```

---

### 9. Worker Entry Point - `cmd/workers/audit_worker/main.go`

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xcentro.in/internal/adapters/database/postgres/audit"
	"xcentro.in/internal/adapters/database/postgres/client"
	auditService "xcentro.in/internal/services/audit"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/workers"
)

func main() {
	fmt.Println("🚀 Starting Audit Worker...")

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Printf("📝 Loaded configuration\n")

	// Get worker configuration
	batchSize := cfg.Workers.Audit.BatchSize
	pollInterval := cfg.Workers.Audit.PollInterval

	fmt.Printf("⚙️  Worker config: batch_size=%d, poll_interval=%ds\n", batchSize, pollInterval)

	// Initialize database connection
	db, err := client.NewPgxDB(context.Background(), cfg.Database.ToDBConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	fmt.Println("✅ Database connection established")

	// Create repositories
	eventRepo := audit.NewPgAuditEventRepository(db)
	trailRepo := audit.NewPgAuditTrailRepository(db)
	entityRepo := audit.NewPgAuditTrailEntityRepository(db)

	fmt.Println("✅ Repositories initialized")

	// Create transaction manager
	txManager := client.NewTxManager(db)

	// Create audit service (orchestrates business logic and transactions)
	svc := auditService.NewAuditService(eventRepo, trailRepo, entityRepo, txManager, logger.GetDefault())

	fmt.Println("✅ Audit service initialized")

	// Initialize audit worker (minimal - just calls service)
	worker := workers.NewAuditWorker(svc, batchSize)

	fmt.Println("✅ Audit worker initialized")

	fmt.Println("\n📦 Worker structure:")
	fmt.Println("  ✅ Worker logic: internal/workers/audit_worker.go (minimal)")
	fmt.Println("  ✅ Service logic: internal/services/audit/audit_service.go (orchestrates)")
	fmt.Println("  ✅ Entry point: cmd/workers/audit_worker/main.go")
	fmt.Println("  ✅ Repositories: audit_events, audit_trail")

	fmt.Println("\n💡 Service responsibilities:")
	fmt.Println("  1. Begin transaction")
	fmt.Println("  2. Fetch pending audit events (FOR UPDATE SKIP LOCKED)")
	fmt.Println("  3. Transform events to audit trail entries")
	fmt.Println("  4. Batch insert into audit_trail")
	fmt.Println("  5. Mark events as processed")
	fmt.Println("  6. Commit transaction (or rollback on error)")

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start worker scheduler
	ticker := time.NewTicker(time.Duration(pollInterval) * time.Second)
	defer ticker.Stop()

	fmt.Printf("\n🔄 Worker scheduler started (polls every %ds)\n", pollInterval)
	fmt.Println("⏸️  Press Ctrl+C to stop gracefully")

	// Run worker immediately on startup
	fmt.Println("🏃 Running initial worker cycle...")
	if err := worker.Run(ctx); err != nil {
		log.Printf("⚠️  Initial worker run failed: %v", err)
	}

	// Main event loop
	for {
		select {
		case <-ticker.C:
			if err := worker.Run(ctx); err != nil {
				log.Printf("⚠️  Worker run failed: %v", err)
			}

		case sig := <-sigChan:
			fmt.Printf("\n\n🛑 Received signal: %v\n", sig)
			fmt.Println("🔄 Shutting down worker gracefully...")
			cancel()
			fmt.Println("✅ Worker stopped")
			return
		}
	}
}
```

---

### 10. Database Migration (Up) - `000023_create_audit_notification_tables.up.sql`

```sql
-- #############################################################################
-- # AUDIT & NOTIFICATION SYSTEM TABLES
-- #############################################################################
-- This migration creates the PostgreSQL-only (Phase 1) implementation
-- of the audit and notification queuing system.
--
-- Architecture:
-- - audit_events (queue) -> audit_trail (permanent log)
-- - notification_events (queue) -> user_notifications (user inbox)
--
-- All tables are in the xcentro schema to match existing project structure.

-- #############################################################################
-- # 1. AUDIT SYSTEM
-- #############################################################################

-- TABLE 1: audit_events (The Queue)
-- Temporary, append-only log of work to be done.
-- Business logic writes here FAST, then async processor moves to audit_trail.
CREATE TABLE xcentro.audit_events (
    id BIGSERIAL PRIMARY KEY,
    processed_at TIMESTAMPTZ DEFAULT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,     -- Full event details (actor, entities, change_log)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for fast lookup of pending events
CREATE INDEX idx_audit_events_pending
ON xcentro.audit_events (created_at)
WHERE processed_at IS NULL;

COMMENT ON TABLE xcentro.audit_events IS 'Temporary queue of audit events to be processed';
COMMENT ON COLUMN xcentro.audit_events.payload IS 'JSON schema: {timestamp (RFC3339), event_type, actor, entities, note, change_log}';


-- TABLE 2: audit_trail (The Destination - Normalized Event Storage)
-- Permanent, queryable, append-only table optimized for SELECT by UI.
-- Stores the event ONCE, links to entities via audit_trail_entities table.
CREATE TABLE xcentro.audit_trail (
    id BIGSERIAL PRIMARY KEY,
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,

    -- The Event Data
    event_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type TEXT NOT NULL,        -- Human-readable title (e.g., "Transaction Approved")
    actor_type TEXT NOT NULL,        -- 'user' or 'system'
    actor_id TEXT NOT NULL,          -- user_id UUID as text or 'system'
    actor_name TEXT NOT NULL,        -- "Michael Chen (Financial Controller)"
    note TEXT DEFAULT NULL,          -- Optional note (e.g., "Funds transferred")
    details JSONB DEFAULT NULL,      -- Extra data (IP, change_log, etc.)

    -- Link back to queue for idempotency
    source_event_id BIGINT NOT NULL UNIQUE REFERENCES xcentro.audit_events(id) ON DELETE CASCADE
);

-- Indexes for fast lookups
CREATE INDEX idx_audit_trail_business_id
ON xcentro.audit_trail (business_id, event_timestamp DESC);

CREATE INDEX idx_audit_trail_event_timestamp
ON xcentro.audit_trail (event_timestamp DESC);

COMMENT ON TABLE xcentro.audit_trail IS 'Permanent normalized audit log - stores event once, links via audit_trail_entities';
COMMENT ON COLUMN xcentro.audit_trail.event_type IS 'Human-readable event title for display';
COMMENT ON COLUMN xcentro.audit_trail.details IS 'Extra data: {ip_address, change_log, etc.}';
COMMENT ON COLUMN xcentro.audit_trail.source_event_id IS 'FK to audit_events.id (ensures no duplicate processing)';


-- TABLE 2B: audit_trail_entities (The Links)
-- Links a single audit trail event to multiple entities (normalized many-to-many).
-- This solves the bulk payment/voucher problem efficiently.
CREATE TABLE xcentro.audit_trail_entities (
    id BIGSERIAL PRIMARY KEY,
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,

    -- Link to the event
    audit_trail_id BIGINT NOT NULL REFERENCES xcentro.audit_trail(id) ON DELETE CASCADE,

    -- Link to the entity
    entity_type TEXT NOT NULL,  -- 'payment', 'voucher', 'payee', 'user', 'wallet', 'payout'
    entity_id UUID NOT NULL     -- The ID of the payment, voucher, payee, etc.
);

-- Most important index for reading data (entity activity feeds)
CREATE INDEX idx_audit_trail_entities_lookup
ON xcentro.audit_trail_entities (entity_type, entity_id);

-- Index for reverse lookup (all entities for an event)
CREATE INDEX idx_audit_trail_entities_audit_id
ON xcentro.audit_trail_entities (audit_trail_id);

-- Composite index for business-scoped entity lookups
CREATE INDEX idx_audit_trail_entities_business_entity
ON xcentro.audit_trail_entities (business_id, entity_type, entity_id);

COMMENT ON TABLE xcentro.audit_trail_entities IS 'Many-to-many links between audit events and entities (normalized design)';
COMMENT ON COLUMN xcentro.audit_trail_entities.entity_type IS 'Type of entity: payment, voucher, payee, user, wallet, payout';
COMMENT ON COLUMN xcentro.audit_trail_entities.entity_id IS 'UUID of the entity (payment_id, voucher_id, etc.)';
```

---

### 11. Database Migration (Down) - `000023_create_audit_notification_tables.down.sql`

```sql
-- #############################################################################
-- # ROLLBACK: AUDIT & NOTIFICATION SYSTEM TABLES
-- #############################################################################
-- This migration rolls back the audit and notification system tables.
-- Drop order: child tables first (due to foreign keys)

-- Drop notification preferences table
DROP TABLE IF EXISTS xcentro.notification_preferences;

-- Drop notification system tables
DROP TABLE IF EXISTS xcentro.user_notifications;
DROP TABLE IF EXISTS xcentro.notification_events;

-- Drop audit system tables (child tables first due to foreign keys)
DROP TABLE IF EXISTS xcentro.audit_trail_entities;  -- Child table (references audit_trail)
DROP TABLE IF EXISTS xcentro.audit_trail;           -- Parent table (referenced by audit_trail_entities)
DROP TABLE IF EXISTS xcentro.audit_events;
```

---

### 12. Configuration Struct - `internal/shared/config/config.go` (Audit-related section)

**NOTE**: These structs need to be REMOVED from config.go:

```go
// WorkersConfig holds worker configuration.
type WorkersConfig struct {
	Audit        AuditWorkerConfig        `json:"audit"`  // ← REMOVE THIS LINE
	Notification NotificationWorkerConfig `json:"notification"`
}

// AuditWorkerConfig holds audit worker configuration.
// ← REMOVE THIS ENTIRE STRUCT
type AuditWorkerConfig struct {
	BatchSize    int `json:"batch_size" env:"AUDIT_WORKER_BATCH_SIZE" env-default:"100"`
	PollInterval int `json:"poll_interval_seconds" env:"AUDIT_WORKER_POLL_INTERVAL_SECONDS" env-default:"2"`
}
```

**Environment Variables to Remove**:
- `AUDIT_WORKER_BATCH_SIZE`
- `AUDIT_WORKER_POLL_INTERVAL_SECONDS`

---

## Restoration Checklist

When re-implementing the audit system, follow this sequence:

1. **Copy source files** from this archive:
   - Core entities: `internal/core/audit/audit_entity.go`
   - Core interfaces: `internal/core/audit/audit_repository.go`
   - Service layer: `internal/services/audit/audit_service.go`
   - Repository implementations: All 3 files in `internal/adapters/database/postgres/audit/`
   - HTTP handlers: `internal/handlers/http/audit/audit_handler.go`
   - Worker: `internal/workers/audit_worker.go`
   - Worker entry point: `cmd/workers/audit_worker/main.go`

2. **Restore configuration**:
   - Add `AuditWorkerConfig` struct to `config.go`
   - Add `Audit` field to `WorkersConfig`
   - Add environment variables to `.env.example`

3. **Run database migration**:
   ```bash
   ./bin/migrate -action=up
   ```
   This will create the audit tables (migration 000023 includes audit tables)

4. **Register API routes** (if needed):
   ```go
   // In cmd/api/routes.go
   func registerV1AuditRoutes(e *echo.Echo, auditHandler *auditHTTP.AuditHandler) {
       audit := e.Group("/v1/audit")
       audit.GET("/business/:business_id", auditHandler.GetBusinessAuditTrail)
       audit.GET("/:entity_type/:entity_id", auditHandler.GetEntityAuditTrail)
   }
   ```

5. **Build and test**:
   ```bash
   go build -o bin/audit_worker ./cmd/workers/audit_worker
   ./bin/audit_worker
   ```

---

## Integration Examples

### Writing Audit Events

```go
// In any service that needs to log an audit event:
auditEvent := &audit.AuditEvent{
    EventType: audit.EventPaymentInitiated,
    Payload: audit.AuditEventPayload{
        Timestamp: time.Now().Format(time.RFC3339),
        EventType: "payment.initiated",
        Actor: audit.ActorInfo{
            Type:      "user",
            ID:        userID,
            Name:      userName,
            IPAddress: &ipAddress,
        },
        Entities: audit.EntityReferences{
            BusinessID: businessID,
            PaymentID:  &paymentID,
        },
        Note: strPtr("Payment initiated via web portal"),
    },
}

// Write to queue (within transaction)
err := auditEventRepo.CreateEvent(ctx, auditEvent)
```

---

## Notes for Future Implementation

1. **Phase 2 Enhancements** (mentioned in code comments):
   - Add Redis for push-based notification (LPUSH/BLPOP)
   - Implement sweeper for orphaned events
   - Add real-time WebSocket support

2. **Monitoring Integration**:
   - CloudWatch metrics for queue depth
   - Alerts for processing lag
   - Error tracking for failed events

3. **Performance Optimizations**:
   - Batch inserts for entity links
   - Connection pooling for workers
   - Partitioning for historical data

---

**END OF ARCHIVE**

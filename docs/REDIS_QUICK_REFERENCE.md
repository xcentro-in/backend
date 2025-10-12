# Redis Caching - Quick Reference

## Key Generation

```go
import "xcentro.in/internal/adapters/cache/redis/keys"

// User keys
keys.User.Profile(userID)           // User profile cache
keys.User.Token(userID)             // JWT tokens
keys.User.EmailLookup(email)        // Email → userID
keys.User.Verification(userID)      // Verification state

// Business keys
keys.Business.Details(businessID)   // Business details
keys.Business.UserRoles(userID)     // User's business relations
keys.Business.GSTIN(gstin)          // GSTIN → businessID

// TTL constants
keys.TTLUserProfile      // 5 minutes
keys.TTLBusinessDetails  // 10 minutes
keys.TTLUserToken        // 24 hours
```

## RedisClient Methods

```go
// Set JSON value with TTL
err := rclient.SetJSON(ctx, key, value, ttl)

// Get JSON value
var user User
err := rclient.GetJSON(ctx, key, &user)
if errors.Is(err, redis.Nil) {
    // Cache miss
}

// Delete keys
err := rclient.Delete(ctx, keys...)

// Check if key exists
exists, err := rclient.Exists(ctx, key)
```

## Cache-Aside Pattern (Copy-Paste Template)

```go
func (s *ServiceImpl) GetResource(ctx context.Context, id string) (*Resource, error) {
    // 1. Try cache first
    cacheKey := keys.Resource.Details(id)
    var resource Resource
    err := s.rclient.GetJSON(ctx, cacheKey, &resource)

    if err == nil {
        // Cache hit
        return &resource, nil
    }

    // 2. Handle cache miss
    if !errors.Is(err, redis.Nil) && err != nil {
        s.logger.Warnf("Redis error (continuing with DB): %v", err)
    }

    // 3. Fetch from database
    resourceFromDB, err := s.repo.Get(ctx, id)
    if err != nil {
        return nil, err
    }

    // 4. Update cache async (fire-and-forget)
    go func() {
        bgCtx := context.Background()
        if err := s.rclient.SetJSON(bgCtx, cacheKey, resourceFromDB, keys.TTLResource); err != nil {
            s.logger.Warnf("Failed to cache resource: %v", err)
        }
    }()

    return resourceFromDB, nil
}
```

## Cache Invalidation Pattern

```go
func (s *ServiceImpl) UpdateResource(ctx context.Context, id string, updates Updates) error {
    // 1. Update database
    err := s.repo.Update(ctx, id, updates)
    if err != nil {
        return err
    }

    // 2. Invalidate cache async
    go func() {
        bgCtx := context.Background()
        invalidationKeys := keys.InvalidateResource(id)
        if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
            s.logger.Warnf("Failed to invalidate cache: %v", err)
        }
    }()

    return nil
}
```

## Debugging Commands

```bash
# Connect to Redis
./deployments/docker/connect-redis.sh

# View all XCentro keys
KEYS xcentro:*

# View specific user cache
KEYS xcentro:user:{user-id}:*

# Get cached value
GET xcentro:user:{user-id}:profile

# Check TTL (seconds remaining)
TTL xcentro:user:{user-id}:profile

# Delete specific key
DEL xcentro:user:{user-id}:profile

# Clear all XCentro cache (USE WITH CAUTION!)
# EVAL "return redis.call('del', unpack(redis.call('keys', 'xcentro:*')))" 0
```

## Common Gotchas

### ❌ DON'T: Block on cache writes
```go
// BAD - blocks request
err := rclient.SetJSON(ctx, key, value, ttl)
if err != nil {
    return err
}
```

### ✅ DO: Fire-and-forget cache writes
```go
// GOOD - async cache update
go func() {
    rclient.SetJSON(context.Background(), key, value, ttl)
}()
```

### ❌ DON'T: Fail request on cache error
```go
// BAD - request fails if Redis is down
if err := rclient.GetJSON(ctx, key, &value); err != nil {
    return nil, err
}
```

### ✅ DO: Fallback to database
```go
// GOOD - continue if cache fails
if err := rclient.GetJSON(ctx, key, &value); err == nil {
    return &value, nil
}
// Continue with DB query
```

### ❌ DON'T: Hardcode cache keys
```go
// BAD - typo-prone
key := "xcentro:user:" + userID + ":profle"  // typo!
```

### ✅ DO: Use key builder
```go
// GOOD - type-safe
key := keys.User.Profile(userID)
```

## Performance Expectations

| Operation | Cache Hit | Cache Miss |
|-----------|-----------|------------|
| GetUserByID | <1ms | ~10-15ms |
| GetUserByEmail | <2ms | ~10-15ms |
| GetUserRoles | <1ms | ~15-20ms |

**Cache Hit Rate Target:** >70%

## When to Cache

✅ **DO cache:**
- Frequently read, infrequently updated data
- Data accessed on every request (user profile, roles)
- Data that's expensive to compute or fetch
- Data that can tolerate slight staleness (5-10 min)

❌ **DON'T cache:**
- Security-sensitive data (verification codes, passwords)
- Data that must be real-time accurate (payments, balances)
- Data accessed infrequently (<10 req/min)
- Large blobs (>1MB per key)

## Adding Cache to New Service

**Checklist:**

1. ☑ Inject `*redisClient.RedisClient` in service constructor
2. ☑ Add key pattern to `internal/adapters/cache/redis/keys/keys.go`
3. ☑ Add TTL constant if needed
4. ☑ Implement cache-aside in read methods
5. ☑ Implement cache invalidation in write methods
6. ☑ Update DI in `cmd/api/routes.go`
7. ☑ Test with manual Redis inspection
8. ☑ Document in service comments

## Examples in Codebase

- **User caching**: `internal/services/user/user_service.go:159-198`
- **Role caching**: `internal/services/business/business_service.go:58-100`
- **Token management**: `internal/services/auth/auth_service.go:130-143`
- **Key patterns**: `internal/adapters/cache/redis/keys/keys.go`
- **Redis helpers**: `internal/adapters/cache/redis/client/redis_client.go:51-105`

package ratelimiter

// TokenBucketLuaScript is the atomic token-bucket implementation in Lua.
// It ensures all operations (read, compute, update) happen atomically in Redis.
//
// KEYS[1] = token key (e.g., "rl:user:123|login")
// ARGV[1] = capacity (int64)
// ARGV[2] = refill_per_second (float64 as string)
// ARGV[3] = now_ms (int64, current timestamp in milliseconds)
// ARGV[4] = requested_tokens (int64)
//
// Returns: {allowed (1|0), remaining_tokens (string), retry_after_seconds (string)}
const TokenBucketLuaScript = `
-- Parse arguments
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local req = tonumber(ARGV[4])

-- Get current token state from Redis
local data = redis.call("HMGET", key, "tokens", "last")
local tokens = tonumber(data[1])
local last = tonumber(data[2])

-- Initialize if not exists
if tokens == nil or last == nil then
  tokens = capacity
  last = now
end

-- Calculate elapsed time and refill tokens
local elapsed_ms = now - last
if elapsed_ms < 0 then
  elapsed_ms = 0
end
local elapsed_s = elapsed_ms / 1000.0
local added = elapsed_s * refill

-- Cap tokens at capacity (bucket overflow)
tokens = math.min(capacity, tokens + added)

-- Check if enough tokens available
if tokens >= req then
  -- Allow request: consume tokens
  tokens = tokens - req
  redis.call("HSET", key, "tokens", tostring(tokens), "last", tostring(now))
  redis.call("EXPIRE", key, 1800) -- 30 minutes TTL
  return {1, tostring(tokens), "0"}
else
  -- Deny request: calculate retry time
  local needed = req - tokens
  local wait = 0
  if refill > 0 then
     wait = needed / refill
  else
     wait = 9999999 -- Effectively infinite if no refill
  end
  return {0, tostring(tokens), tostring(wait)}
end
`

// FixedWindowLuaScript is the atomic fixed-window implementation in Lua.
// Uses DECRBY to decrement remaining quota - simpler than counting up.
//
// KEYS[1] = window key (e.g., "rl:user:123|login:fw:1672531200")
// ARGV[1] = limit (int64, maximum requests allowed in the window)
// ARGV[2] = ttl (int64, window duration in seconds)
// ARGV[3] = tokens (int64, number of tokens to consume)
//
// Returns: {allowed (1|0), remaining_tokens (string), retry_after_seconds (string)}
const FixedWindowLuaScript = `
-- Parse arguments
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local tokens = tonumber(ARGV[3])

-- Get current remaining quota
local remaining = redis.call('GET', key)

-- If key doesn't exist, initialize with full limit
if remaining == false then
  redis.call('SET', key, limit)
  redis.call('EXPIRE', key, ttl)
  remaining = limit
else
  remaining = tonumber(remaining)
end

-- Check if we have enough quota
if remaining >= tokens then
  -- Allow: decrement quota
  local new_remaining = redis.call('DECRBY', key, tokens)
  return {1, tostring(new_remaining), "0"}
else
  -- Deny: not enough quota, get TTL for retry_after
  local remaining_ttl = redis.call('TTL', key)
  if remaining_ttl < 0 then
    remaining_ttl = 0
  end
  return {0, tostring(remaining), tostring(remaining_ttl)}
end
`

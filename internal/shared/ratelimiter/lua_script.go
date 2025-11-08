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
  redis.call("HMSET", key, "tokens", tostring(tokens), "last", tostring(now))
  redis.call("EXPIRE", key, 7200) -- 2 hours TTL
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

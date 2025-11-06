package ratelimiter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/shared/logger"
)

// DeviceMetadata stores metadata about a device mapping.
type DeviceMetadata struct {
	CreatedAt string `json:"created_at"`
	IP        string `json:"ip"`
	UserAgent string `json:"ua"`
}

// DeviceHandler manages device cookie creation and validation.
type DeviceHandler struct {
	redis                  *redis.Client
	cookieName             string
	cookieTTL              time.Duration
	deviceMappingTTL       time.Duration
	creationThrottlePerIP  int
	creationThrottleWindow time.Duration
	cookieSecure           bool
	cookieDomain           string
	log                    *logger.Logger
}

// NewDeviceHandler creates a new device handler.
func NewDeviceHandler(
	redisClient *redis.Client,
	cookieName string,
	cookieTTL time.Duration,
	deviceMappingTTL time.Duration,
	creationThrottlePerIP int,
	creationThrottleWindow time.Duration,
	cookieSecure bool,
	cookieDomain string,
) *DeviceHandler {
	return &DeviceHandler{
		redis:                  redisClient,
		cookieName:             cookieName,
		cookieTTL:              cookieTTL,
		deviceMappingTTL:       deviceMappingTTL,
		creationThrottlePerIP:  creationThrottlePerIP,
		creationThrottleWindow: creationThrottleWindow,
		cookieSecure:           cookieSecure,
		cookieDomain:           cookieDomain,
		log:                    logger.GetDefault(),
	}
}

// EnsureDeviceID ensures a valid device ID exists, creating one if needed.
// Returns deviceID and whether it was newly created.
func (dh *DeviceHandler) EnsureDeviceID(c echo.Context) (string, bool, error) {
	ctx := c.Request().Context()

	// Check if device_id cookie exists
	cookie, err := c.Cookie(dh.cookieName)
	if err == nil && cookie.Value != "" {
		deviceID := cookie.Value

		// Validate device mapping exists in Redis
		exists, err := dh.deviceMappingExists(ctx, deviceID)
		if err != nil {
			dh.log.Errorf("Failed to check device mapping existence: %v", err)
			// On Redis error, accept cookie value (fail-open for device validation)
			return deviceID, false, nil
		}

		if exists {
			// Refresh TTL on existing mapping
			if err := dh.refreshDeviceMappingTTL(ctx, deviceID); err != nil {
				dh.log.Errorf("Failed to refresh device mapping TTL: %v", err)
			}
			return deviceID, false, nil
		}

		// Cookie exists but mapping is gone - treat as stale, create new
		dh.log.Infof("Stale device cookie detected: %s, creating new device", deviceID)
	}

	// No valid cookie - attempt to create device
	deviceID, created, err := dh.createDevice(c)
	if err != nil {
		return "", false, err
	}

	if created {
		// Set cookie
		dh.setDeviceCookie(c, deviceID)
	}

	return deviceID, created, nil
}

// createDevice attempts to create a new device mapping with throttle check.
func (dh *DeviceHandler) createDevice(c echo.Context) (string, bool, error) {
	ctx := c.Request().Context()
	ip := c.RealIP()

	// Check creation throttle per IP
	throttled, err := dh.isCreationThrottled(ctx, ip)
	if err != nil {
		dh.log.Errorf("Failed to check creation throttle: %v", err)
		return "", false, fmt.Errorf("throttle check failed")
	}

	if throttled {
		dh.log.Warnf("Device creation throttled for IP: %s", ip)
		return "", false, fmt.Errorf("device creation throttled")
	}

	// Generate device ID
	deviceID, err := generateDeviceID()
	if err != nil {
		dh.log.Errorf("Failed to generate device ID: %v", err)
		return "", false, fmt.Errorf("failed to generate device ID")
	}

	// Store device mapping
	metadata := DeviceMetadata{
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		IP:        ip,
		UserAgent: c.Request().UserAgent(),
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		dh.log.Errorf("Failed to marshal device metadata: %v", err)
		return "", false, fmt.Errorf("failed to marshal metadata")
	}

	key := fmt.Sprintf("device:%s", deviceID)
	err = dh.redis.Set(ctx, key, metadataJSON, dh.deviceMappingTTL).Err()
	if err != nil {
		dh.log.Errorf("Failed to store device mapping: %v", err)
		return "", false, fmt.Errorf("failed to store device mapping")
	}

	// Increment creation counter for IP
	if err := dh.incrementCreationCounter(ctx, ip); err != nil {
		dh.log.Errorf("Failed to increment creation counter: %v", err)
		// Non-fatal, continue
	}

	dh.log.Infof("Created new device: %s for IP: %s", deviceID, ip)
	return deviceID, true, nil
}

// deviceMappingExists checks if a device mapping exists in Redis.
func (dh *DeviceHandler) deviceMappingExists(ctx context.Context, deviceID string) (bool, error) {
	key := fmt.Sprintf("device:%s", deviceID)
	exists, err := dh.redis.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// refreshDeviceMappingTTL refreshes the TTL on an existing device mapping.
func (dh *DeviceHandler) refreshDeviceMappingTTL(ctx context.Context, deviceID string) error {
	key := fmt.Sprintf("device:%s", deviceID)
	return dh.redis.Expire(ctx, key, dh.deviceMappingTTL).Err()
}

// isCreationThrottled checks if device creation is throttled for the given IP.
func (dh *DeviceHandler) isCreationThrottled(ctx context.Context, ip string) (bool, error) {
	key := fmt.Sprintf("device:create:ip:%s", ip)
	count, err := dh.redis.Get(ctx, key).Int()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return count >= dh.creationThrottlePerIP, nil
}

// incrementCreationCounter increments the creation counter for an IP.
func (dh *DeviceHandler) incrementCreationCounter(ctx context.Context, ip string) error {
	key := fmt.Sprintf("device:create:ip:%s", ip)
	pipe := dh.redis.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, dh.creationThrottleWindow)
	_, err := pipe.Exec(ctx)
	return err
}

// setDeviceCookie sets the device_id cookie.
func (dh *DeviceHandler) setDeviceCookie(c echo.Context, deviceID string) {
	cookie := &http.Cookie{
		Name:     dh.cookieName,
		Value:    deviceID,
		Path:     "/",
		Expires:  time.Now().Add(dh.cookieTTL),
		HttpOnly: true,
		Secure:   dh.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	if dh.cookieDomain != "" {
		cookie.Domain = dh.cookieDomain
	}
	c.SetCookie(cookie)
}

// generateDeviceID generates a cryptographically secure random device ID.
func generateDeviceID() (string, error) {
	bytes := make([]byte, 16) // 128 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

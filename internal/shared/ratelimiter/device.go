package ratelimiter

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/shared/logger"
)

// ErrInvalidDeviceSignature is returned when device cookie signature verification fails.
// This error indicates suspected tampering or DDoS attack.
var ErrInvalidDeviceSignature = errors.New("invalid device signature")

// DeviceHandler manages device cookie creation and HMAC-based validation.
// Uses HMAC signing instead of Redis device mapping for better performance.
type DeviceHandler struct {
	redis                  *redis.Client
	hmacSecret             []byte
	cookieName             string
	cookieTTL              time.Duration
	creationThrottlePerIP  int
	creationThrottleWindow time.Duration
	cookieSecure           bool
	cookieSameSite         http.SameSite
	cookieDomain           string
	skipValidation         bool // Skip signature validation (dev mode only)
	log                    *logger.Logger
}

// getSameSiteMode converts config string to http.SameSite enum.
func getSameSiteMode(sameSite string) http.SameSite {
	switch sameSite {
	case "Strict":
		return http.SameSiteStrictMode
	case "Lax":
		return http.SameSiteLaxMode
	case "None":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

// NewDeviceHandler creates a new device handler with HMAC signing.
func NewDeviceHandler(
	redisClient *redis.Client,
	hmacSecret string,
	cookieName string,
	cookieTTL time.Duration,
	creationThrottlePerIP int,
	creationThrottleWindow time.Duration,
	cookieSecure bool,
	cookieSameSite string,
	cookieDomain string,
	skipValidation bool,
) *DeviceHandler {
	return &DeviceHandler{
		redis:                  redisClient,
		hmacSecret:             []byte(hmacSecret),
		cookieName:             cookieName,
		cookieTTL:              cookieTTL,
		creationThrottlePerIP:  creationThrottlePerIP,
		creationThrottleWindow: creationThrottleWindow,
		cookieSecure:           cookieSecure,
		cookieSameSite:         getSameSiteMode(cookieSameSite),
		cookieDomain:           cookieDomain,
		skipValidation:         skipValidation,
		log:                    logger.GetDefault(),
	}
}

// EnsureDeviceID ensures a valid device ID exists, creating one if needed.
// Cookie format: deviceID|hmac_signature
// Returns deviceID and whether it was newly created.
// Returns error if signature is invalid (suspected tampering/DDoS).
func (dh *DeviceHandler) EnsureDeviceID(c echo.Context) (string, bool, error) {
	// Check if device_id cookie exists
	cookie, err := c.Cookie(dh.cookieName)
	if err == nil && cookie.Value != "" {
		// Parse cookie value: deviceID|signature
		parts := strings.Split(cookie.Value, "|")
		if len(parts) != 2 {
			// In dev mode (skip validation), try to extract deviceID anyway
			if dh.skipValidation && len(parts) == 1 {
				dh.log.Warnf("Invalid device cookie format from IP %s (dev mode: using raw value as deviceID)", c.RealIP())
				return parts[0], false, nil
			}
			dh.log.Warnf("Invalid device cookie format from IP %s: expected 'deviceID|signature'", c.RealIP())
			return "", false, ErrInvalidDeviceSignature
		}

		deviceID := parts[0]
		providedSignature := parts[1]

		// Skip signature verification in dev mode
		if dh.skipValidation {
			dh.log.Debugf("Device signature validation skipped (dev mode) for IP %s", c.RealIP())
			return deviceID, false, nil
		}

		// Verify HMAC signature
		if !dh.verifyDeviceSignature(deviceID, providedSignature) {
			dh.log.Warnf("Invalid device signature from IP %s, suspected tampering/DDoS", c.RealIP())
			return "", false, ErrInvalidDeviceSignature
		}

		// Signature valid - use this device ID
		return deviceID, false, nil
	}

	// No valid cookie - attempt to create device
	deviceID, created, err := dh.createDevice(c)
	if err != nil {
		return "", false, err
	}

	if created {
		// Sign device ID and set cookie
		dh.setSignedDeviceCookie(c, deviceID)
	}

	return deviceID, created, nil
}

// createDevice attempts to create a new device ID with throttle check.
// No longer stores device mapping in Redis - uses HMAC signing instead.
func (dh *DeviceHandler) createDevice(c echo.Context) (string, bool, error) {
	ctx := c.Request().Context()
	ip := c.RealIP()

	// Check creation throttle per IP (fixed window in Redis)
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

	// Increment creation counter for IP
	if err := dh.incrementCreationCounter(ctx, ip); err != nil {
		dh.log.Errorf("Failed to increment creation counter: %v", err)
		// Non-fatal, continue
	}

	dh.log.Infof("Created new device: %s for IP: %s", deviceID, ip)
	return deviceID, true, nil
}

// signDeviceID creates an HMAC-SHA256 signature for a device ID.
func (dh *DeviceHandler) signDeviceID(deviceID string) string {
	h := hmac.New(sha256.New, dh.hmacSecret)
	h.Write([]byte(deviceID))
	return hex.EncodeToString(h.Sum(nil))
}

// verifyDeviceSignature verifies the HMAC signature of a device ID.
func (dh *DeviceHandler) verifyDeviceSignature(deviceID, providedSignature string) bool {
	expectedSignature := dh.signDeviceID(deviceID)
	return hmac.Equal([]byte(expectedSignature), []byte(providedSignature))
}

// isCreationThrottled checks if device creation is throttled for the given IP.
func (dh *DeviceHandler) isCreationThrottled(ctx context.Context, ip string) (bool, error) {
	key := fmt.Sprintf("xcentro:device:create:ip:%s", ip)
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
	key := fmt.Sprintf("xcentro:device:create:ip:%s", ip)
	pipe := dh.redis.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, dh.creationThrottleWindow)
	_, err := pipe.Exec(ctx)
	return err
}

// setSignedDeviceCookie sets the device_id cookie with HMAC signature.
// Cookie format: deviceID|hmac_signature
func (dh *DeviceHandler) setSignedDeviceCookie(c echo.Context, deviceID string) {
	signature := dh.signDeviceID(deviceID)
	signedValue := fmt.Sprintf("%s|%s", deviceID, signature)

	cookie := &http.Cookie{
		Name:     dh.cookieName,
		Value:    signedValue,
		Path:     "/",
		Expires:  time.Now().Add(dh.cookieTTL),
		HttpOnly: true,
		Secure:   dh.cookieSecure,
		SameSite: dh.cookieSameSite,
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

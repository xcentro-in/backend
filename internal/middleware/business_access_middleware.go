package middleware

import (
	"context"
	"errors"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	redisClient "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/shared/logger"
)

// BusinessAccessMiddleware validates that the user has access to the business specified in X-Business-ID header
// This middleware MUST be called after JWTMiddleware (requires userId in context)
// It checks business_user_relation table to ensure user belongs to the business
// Uses Redis cache for performance with DB fallback
func BusinessAccessMiddleware(
	businessUserRelRepo business.BusinessUserRelationRepository,
	rclient *redisClient.RedisClient,
) echo.MiddlewareFunc {
	log := logger.GetDefault()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract X-Business-ID header
			businessID := c.Request().Header.Get("X-Business-ID")
			if businessID == "" {
				return c.JSON(400, map[string]any{
					"error":   "MISSING_BUSINESS_ID",
					"message": "X-Business-ID header is required",
				})
			}

			// Get userId from context (set by JWTMiddleware)
			userID, ok := c.Get("userId").(string)
			if !ok || userID == "" {
				// This should not happen if JWTMiddleware is properly configured
				log.Error("BusinessAccessMiddleware called without userId in context")
				return c.JSON(401, map[string]any{
					"error":   "UNAUTHORIZED",
					"message": "Authentication required",
				})
			}

			ctx := c.Request().Context()

			// Check if user has access to this business
			hasAccess, err := checkUserBusinessAccess(ctx, userID, businessID, businessUserRelRepo, rclient, log)
			if err != nil {
				log.Errorf("Error checking business access: %v", err)
				return c.JSON(500, map[string]any{
					"error":   "INTERNAL_ERROR",
					"message": "Failed to verify business access",
				})
			}

			if !hasAccess {
				log.Warnf("User %s attempted to access business %s without permission", userID, businessID)
				return c.JSON(403, map[string]any{
					"error":   "ACCESS_DENIED",
					"message": "You do not have access to this business",
				})
			}

			// User has access - store businessID in context for downstream handlers
			c.Set("businessId", businessID)

			// Continue to next handler
			return next(c)
		}
	}
}

// checkUserBusinessAccess checks if a user belongs to a business
// Uses Redis cache with DB fallback for performance
func checkUserBusinessAccess(
	ctx context.Context,
	userID string,
	businessID string,
	businessUserRelRepo business.BusinessUserRelationRepository,
	rclient *redisClient.RedisClient,
	log *logger.Logger,
) (bool, error) {
	// Try cache first
	cacheKey := keys.Business.UserRoles(userID)
	var relations []*business.BusinessUserRelation
	err := rclient.GetJSON(ctx, cacheKey, &relations)

	if err == nil && len(relations) > 0 {
		// Cache hit - check if businessID is in the list
		for _, rel := range relations {
			if rel.BID == businessID {
				return true, nil
			}
		}
		// User has cached relations but not for this business
		return false, nil
	}

	// Cache miss or error - fetch from database
	if !errors.Is(err, redis.Nil) && err != nil {
		log.Warnf("Redis cache error (continuing with DB): %v", err)
	}

	// Fetch all relations for this user from database
	relations, err = businessUserRelRepo.FindByUser(ctx, userID)
	if err != nil {
		return false, err
	}

	// Check if user belongs to the requested business
	hasAccess := false
	for _, rel := range relations {
		if rel.BID == businessID {
			hasAccess = true
			break
		}
	}

	// Update cache asynchronously (fire-and-forget)
	// Cache all relations for future requests
	if len(relations) > 0 {
		go func() {
			bgCtx := context.Background()
			if err := rclient.SetJSON(bgCtx, cacheKey, relations, keys.WithJitter(keys.TTLUserRoles)); err != nil {
				log.Warnf("Failed to cache user relations: %v", err)
			}
		}()
	}

	return hasAccess, nil
}

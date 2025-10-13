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

// BusinessRolesMiddleware creates a middleware that fetches user's business roles
// and injects them into the context. It uses Redis cache with DB fallback.
func BusinessRolesMiddleware(
	businessUserRelRepo business.BusinessUserRelationRepository,
	rclient *redisClient.RedisClient,
) echo.MiddlewareFunc {
	log := logger.GetDefault()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID := c.Get("userId").(string)
			ctx := c.Request().Context()

			// Try cache first
			cacheKey := keys.Business.UserRoles(userID)
			var rel []*business.BusinessUserRelation
			err := rclient.GetJSON(ctx, cacheKey, &rel)

			if err == nil && len(rel) > 0 {
				// Cache hit
				c.Set("userRoles", rel)
				return next(c)
			}

			// Cache miss or error - fetch from database
			if !errors.Is(err, redis.Nil) && err != nil {
				log.Warnf("Redis cache error (continuing with DB): %v", err)
			}

			rel, err = businessUserRelRepo.FindByUser(ctx, userID)
			if err != nil {
				log.Errorf("%v", err)
			}

			if len(rel) > 0 {
				c.Set("userRoles", rel)

				// Update cache asynchronously (fire-and-forget)
				go func() {
					bgCtx := context.Background()
					if err := rclient.SetJSON(bgCtx, cacheKey, rel, keys.TTLUserRoles); err != nil {
						log.Warnf("Failed to cache user roles: %v", err)
					}
				}()
			}

			return next(c)
		}
	}
}

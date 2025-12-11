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
// All roles (including empty/null ones) are stored in cache and context.
// Filtering should be done at the handler level based on specific needs.
func BusinessRolesMiddleware(
	businessUserRelRepo business.BusinessUserRelationRepository,
	rclient *redisClient.RedisClient,
) echo.MiddlewareFunc {
	log := logger.GetDefault()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Check if roles are already set by JWT middleware (optimization)
			if existingRoles := c.Get("userRoles"); existingRoles != nil {
				// Roles already fetched by JWT middleware, skip redundant fetch
				return next(c)
			}

			userID := c.Get("userId").(string)
			ctx := c.Request().Context()

			// Try cache first
			cacheKey := keys.Business.UserRoles(userID)
			var rel []*business.BusinessUserRelation
			err := rclient.GetJSON(ctx, cacheKey, &rel)

			if err == nil && len(rel) > 0 {
				// Cache hit - store all roles in context, filtering will be done at handler level
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

			// Store all roles (including empty ones) in context and cache
			// Filtering will be done at handler level based on specific needs
			if len(rel) > 0 {
				c.Set("userRoles", rel)

				// Update cache asynchronously with all roles (fire-and-forget)
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

package middleware

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/metrics"
)

// MetricsMiddleware creates middleware for tracking HTTP request metrics
// Tracks: request count, duration, status codes, errors
func MetricsMiddleware(metricsProvider metrics.MetricsProvider) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip metrics for health check endpoints
			if c.Path() == "/health" || c.Path() == "/ping" {
				return next(c)
			}

			start := time.Now()
			req := c.Request()
			method := req.Method
			path := c.Path()

			// Process request
			err := next(c)

			// Calculate duration
			duration := time.Since(start)
			status := c.Response().Status

			// Extract common labels
			labels := metrics.MetricLabels{
				"method": method,
				"path":   path,
				"status": strconv.Itoa(status),
			}

			// Track request count
			_ = metricsProvider.Counter(c.Request().Context(), "http.request", 1, labels)

			// Track request duration
			_ = metricsProvider.Timer(c.Request().Context(), "http.request.duration", duration, labels)

			// Track errors (4xx and 5xx)
			if status >= 400 {
				errorLabels := metrics.MetricLabels{
					"method":      method,
					"path":        path,
					"status_code": strconv.Itoa(status),
				}

				if status >= 500 {
					_ = metricsProvider.Counter(c.Request().Context(), "http.errors.5xx", 1, errorLabels)
				} else if status >= 400 {
					_ = metricsProvider.Counter(c.Request().Context(), "http.errors.4xx", 1, errorLabels)
				}
			}

			// Track request and response sizes
			if req.ContentLength > 0 {
				sizeLabels := metrics.MetricLabels{
					"method": method,
					"path":   path,
				}
				_ = metricsProvider.Counter(c.Request().Context(), "http.request.size", float64(req.ContentLength), sizeLabels)
			}

			responseSize := c.Response().Size
			if responseSize > 0 {
				sizeLabels := metrics.MetricLabels{
					"method": method,
					"path":   path,
				}
				_ = metricsProvider.Counter(c.Request().Context(), "http.response.size", float64(responseSize), sizeLabels)
			}

			return err
		}
	}
}

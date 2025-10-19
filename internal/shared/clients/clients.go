package clients

import (
	redisClient "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/providers/mailers"
)

// ExternalClients interface provides access to all external clients
// This interface allows services to extract only the clients they need
// without changing service constructors when new clients are added
type ExternalClients interface {
	// Database clients
	GetDB() client.DB
	GetTxManager() client.TxManager

	// Cache clients
	GetRedisClient() *redisClient.RedisClient

	// External service clients
	GetEmailService() mailers.EmailService

	// Metrics client
	// Returns interface{} to avoid circular dependency
	// Services can type-assert to metrics.MetricsProvider when needed
	GetMetrics() interface{}
}

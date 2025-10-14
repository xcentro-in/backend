# GitHub Copilot Instructions for XCentro Backend

## Project Architecture

XCentro is a **payment processing platform** using **Clean Architecture** with Go 1.25+, Echo v4, PostgreSQL, and Redis. Module path: `xcentro.in`

### Core Architecture Pattern
```
HTTP → handlers → services → core → repository interface → adapters
```

**Clean Architecture Layers:**
- `internal/core/`: Pure business logic and domain entities (NO external dependencies)
- `internal/services/`: Orchestration layer coordinating multiple domains 
- `internal/handlers/`: HTTP request/response handling
- `internal/adapters/`: External system integrations (DB, cache, APIs)
- `internal/middleware/`: Reusable middleware (JWT + Redis-cached business roles)
- `cmd/`: Multiple entry points for scaling (api, migrate, payout_worker)

### Key Domains
- **User**: Authentication, KYC verification (`/v1/auth`, `/v1/user`)
- **Business**: Business entity management, KYB (`/v1/business`) 
- **Payment**: Multi-gateway orchestration (JioPay, Easebuzz) (`/api/v1/process_payment`)
- **Payee**: Payee onboarding with GST/bank verification (`/v1/payees`)
- **Payout**: Voucher creation and payout processing (`/v1/vouchers`)

## Development Workflow

### Quick Start Commands
```bash
# Environment setup
cd deployments/docker && ./start.sh && ./setup-postgres-users.sh && ./setup-redis-users.sh

# Build & run
go build -o bin/api ./cmd/api && ./bin/api
go build -o bin/migrate ./cmd/migrate && ./bin/migrate -action=up

# Testing
go test ./... -cover
```

### Service & Repository Pattern
**Always follow this dependency injection pattern:**
```go
// In cmd/api/routes.go - Wire services with repositories
userRepo := userRepo.NewPostgresUserRepository(meta.clients.db)
userService := userSvc.NewUserService(userRepo, authService, ...)
userHandler := userHandler.NewUserHandler(userService, authService)
```

### Transaction Management
Use `client.TxManager` for multi-table operations:
```go
return s.txManager.WithTransaction(ctx, func(tx client.Tx) error {
    // Multiple repository calls within transaction
    err := s.userRepo.WithTx(tx).UpdateProfile(ctx, userID, profile)
    // Auto-rollback on error, commit on success
})
```

## Critical Conventions

### Error Handling
Use typed errors from `internal/shared/types/error.go`:
```go
if userErr, ok := err.(*types.UserError); ok {
    switch userErr.Code {
    case types.ErrCodeUserNotFound:
        return c.JSON(404, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
    }
}
```

### Environment Variables
**Required for development:**
- Database: `DB_PASSWORD`, `DB_MIGRATION_PASSWORD`, `JWT_SECRET_KEY`
- Payment gateways: `JIOPAY_*`, `EASEBUZZ_*` (see `internal/shared/config/config.go`)
- AWS: `AWS_*`, `S3_BUCKET_NAME`, `SES_DEFAULT_FROM_EMAIL`

### Middleware Chain
**Authentication pattern:**
```go
// JWT validation + business role caching
protectedUser.GET("/roles", handler.GetUserRoles, 
    middleware.JWTMiddleware(authService),
    middleware.BusinessRolesMiddleware(businessUserRelRepo, rclient))
```

### Repository Interface Pattern
**Always define interfaces in core, implement in adapters:**
```go
// internal/core/user/user_repository.go
type UserRepository interface {
    WithTx(tx interface{}) UserRepository  // Transaction support
    CreateUser(ctx context.Context, user *User) error
}

// internal/adapters/database/postgres/user/user_repository.go  
func (r *PgUserRepository) WithTx(tx interface{}) user.UserRepository {
    return &PgUserRepository{querier: tx.(client.Tx)}
}
```

## Payment Gateway Integration

### Gateway Selection Logic
Automatic routing based on card type:
- **JioPay**: Net banking, Master/Visa/RuPay credit, corporate cards
- **Easebuzz**: Amex/Diners, Master/Visa debit cards

### Payment Flow
1. **Tokenization**: Store card securely via gateway vault APIs
2. **Cryptogram**: Generate payment cryptogram for transaction
3. **Payment**: Process with gateway-specific encryption/hashing
4. **Webhook**: Handle async payment status updates

## File Locations Reference

- **API Routes**: `cmd/api/routes.go`
- **Domain Logic**: `internal/core/{domain}/`
- **Services**: `internal/services/{domain}/`
- **Database**: `internal/adapters/database/postgres/{domain}/`
- **Migrations**: `internal/adapters/database/postgres/migrations/sql/`
- **Configuration**: `internal/shared/config/config.go`
- **Workers**: `internal/workers/` + `cmd/{worker_name}/`

Always read existing patterns in these directories before implementing new features.
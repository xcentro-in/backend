# Business Module Implementation

This document describes the implementation of the business module following the clean architecture pattern established in the project.

## Overview

The business module has been implemented with the following components:

### 1. Domain Layer (`internal/core/business/`)
- **business_entity.go**: Contains the business domain entities
- **business_repository.go**: Defines repository interfaces
- **business_service.go**: Defines service interfaces

### 2. Application Layer (`internal/services/business/`)
- **business_service.go**: Implements business logic and coordinates between repositories

### 3. Infrastructure Layer (`internal/adapters/database/postgres/business/`)
- **business_repository.go**: PostgreSQL implementation of business repository
- **business_user_relation_repository.go**: PostgreSQL implementation of business-user relationship repository

### 4. Presentation Layer (`internal/handlers/http/business/`)
- **business_handler.go**: HTTP handlers for business operations with REST API endpoints

## API Endpoints

### Business Operations
- `POST /business` - Create a new business
- `GET /business/:id` - Get business by ID
- `PUT /business/:id` - Update business
- `GET /business/gstin/:gstin` - Get business by GSTIN
- `PUT /business/:id/kyb` - Update KYB data
- `PUT /business/:id/subscription` - Update subscription status

### Business-User Relationship Operations
- `POST /business/:id/users` - Add user to business
- `GET /business/:id/users` - Get all users for a business
- `PUT /business/:id/users/:userId/role` - Update user role in business
- `DELETE /business/:id/users/:userId` - Remove user from business
- `GET /business/user/:userId` - Get all businesses for a user

## Key Features

### 1. Context Propagation
All service and repository methods accept `context.Context` as the first parameter, enabling proper request tracing, timeouts, and cancellation. Handlers pass `c.Request().Context()` through the entire call chain.

### 2. Transaction Support
All operations use PostgreSQL transactions for data consistency, following the established pattern with `TxManager`.

### 3. Error Handling
Comprehensive error handling with proper logging and user-friendly error messages.

### 4. Type Safety
Uses strongly typed enums from the shared package for:
- KYB status
- User roles
- Business types
- Subscription types

### 5. Validation
Basic validation on required fields with appropriate error responses.

## Database Schema Alignment

The implementation aligns with the database schema defined in the migrations:
- `xcentro.business` table for business entities
- `xcentro.kyb_data` table for KYB information
- `xcentro.business_user_relation` table for user-business relationships
- `xcentro.subscription_status` table for subscription management

## Request/Response Examples

### Create Business
```json
POST /business
{
    "name": "Tech Corp",
    "gstin": "29ABCDE1234F1Z5",
    "businessType": 3,
    "countryCode": "IN",
    "currencyCode": "INR"
}
```

### Add User to Business
```json
POST /business/:id/users
{
    "userId": "user-uuid",
    "role": 0,
    "roleClaim": 0,
    "kybInitiated": false
}
```

### Update KYB Data
```json
PUT /business/:id/kyb
{
    "itid": "INCOME123456",
    "itidData": "{\"verified\": true}",
    "cid": "COMPANY789",
    "cidData": "{\"status\": \"active\"}",
    "docUrl": "https://docs.example.com/kyb"
}
```

## Integration

To integrate this module:

1. Register the business routes in your main router
2. Inject the required dependencies (repository, service, tx manager)
3. Ensure database migrations are applied
4. Add authentication middleware as needed

Example integration:
```go
// In your main router setup
businessRepo := businessRepo.NewPgBusinessRepository(db)
businessUserRelRepo := businessRepo.NewPgBusinessUserRelationRepository(db)
businessService := businessService.NewBusinessService(businessRepo, businessUserRelRepo, txManager)

// Register routes
business.RegisterBusinessRoutes(api, businessService)
```

## Notes

- The implementation is kept minimal as requested, focusing on core functionality
- Edge cases and advanced validations can be added later
- All operations include proper logging for debugging and monitoring
- The code follows the established patterns in the existing user and auth modules
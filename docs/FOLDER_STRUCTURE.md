# XCentro Backend - Clean Architecture Structure Guide

**Domain**: `xcentro.in`  
**Package Path**: All Go modules use `xcentro.in/backend/...`

> 💡 **For New Developers**: This README explains our folder structure and the purpose of each directory. Start by reading the "Key Concepts" section below, then explore each folder following the guided tour.

## 🏗️ Folder Structure Overview

```
xcentro-backend/
├── cmd/                           # 🚀 ENTRY POINTS - Multiple executable programs
│   ├── api/                       # 📡 REST API server entry point
│   │   └── main.go               # Start API server: handles HTTP requests, authentication
│   ├── scheduler/                 # ⏰ Background task scheduler entry point
│   │   └── main.go               # Start scheduler: runs cron jobs, periodic tasks
│   ├── notification/              # 📬 Notification worker entry point
│   │   └── main.go               # Start notification service: emails, SMS, push notifications
│   ├── etl/                       # 🔄 ETL (Extract-Transform-Load) worker entry point
│   │   └── main.go               # Start ETL jobs: data processing, migrations, sync
│   ├── audit/                     # 🔍 Audit log processing entry point
│   │   └── main.go               # Start audit worker: compliance, logging, monitoring
│   └── migrate/                   # 🗄️  Database migration tool entry point
│       └── main.go               # Run database migrations: schema updates, data fixes
│
├── internal/                      # 🔒 PRIVATE CODE - Cannot be imported by external projects
│   ├── core/                      # 🧠 BUSINESS LOGIC - Domain models and rules (most important!)
│   │   ├── user/                 # 👤 User domain: authentication, profiles, permissions
│   │   │   ├── entity.go          # User struct, business rules, validation
│   │   │   ├── service.go         # User operations: create, update, authenticate
│   │   │   └── repository.go      # User data access interface (implemented in adapters/)
│   │   ├── notification/          # 📢 Notification domain: emails, SMS, push alerts
│   │   │   ├── entity.go          # Notification models, delivery rules
│   │   │   ├── service.go         # Send notifications, template processing
│   │   │   └── repository.go      # Notification storage interface
│   │   ├── audit/                 # 📊 Audit domain: logging, compliance, tracking
│   │   │   ├── entity.go          # Audit log models, compliance rules
│   │   │   ├── service.go         # Create audit trails, compliance checks
│   │   │   └── repository.go      # Audit log storage interface
│   │   └── task/                  # 📋 Task domain: background jobs, workflows
│   │       ├── entity.go          # Task models, job states, priorities
│   │       ├── service.go         # Task execution, scheduling, retries
│   │       └── repository.go      # Task storage interface
│   │
│   ├── adapters/                  # 🔌 EXTERNAL INTEGRATIONS - Connect to outside world
│   │   ├── database/              # 🗃️  Database implementations
│   │   │   ├── postgres/         # PostgreSQL implementations of repository interfaces
│   │   │   │   ├── user_repo.go   # User repository: SQL queries for user operations
│   │   │   │   ├── notification_repo.go # Notification storage: SQL for notifications
│   │   │   │   ├── audit_repo.go  # Audit repository: SQL for compliance logs
│   │   │   │   └── migrations/    # SQL migration files: schema changes
│   │   │   └── redis/             # Redis cache implementations
│   │   │       ├── user_cache.go  # User caching: sessions, temporary data
│   │   │       └── session_cache.go # Session management: login states
│   │   ├── queue/                 # 📨 Message queue implementations
│   │   │   ├── rabbitmq/         # RabbitMQ for reliable messaging
│   │   │   │   ├── publisher.go   # Publish messages: send jobs to workers
│   │   │   │   └── consumer.go    # Consume messages: process background jobs
│   │   │   └── redis/            # Redis for simple queuing
│   │   │       ├── publisher.go   # Redis pub/sub: lightweight messaging
│   │   │       └── consumer.go    # Redis consumer: process simple jobs
│   │   ├── notification/          # 📤 External notification services
│   │   │   ├── email/            # Email service implementations
│   │   │   │   └── smtp.go        # SMTP client: send emails via mail server
│   │   │   ├── sms/              # SMS service implementations
│   │   │   │   └── twilio.go      # Twilio integration: send SMS messages
│   │   │   └── push/             # Push notification implementations
│   │   │       └── firebase.go    # Firebase FCM: mobile push notifications
│   │   └── external/              # 🌐 Third-party API clients
│   │       ├── payment/          # Payment gateway integrations
│   │       │   └── stripe.go      # Stripe API: process payments, subscriptions
│   │       └── analytics/        # Analytics service integrations
│   │           └── mixpanel.go    # Mixpanel: track user events, analytics
│   │
│   ├── handlers/                  # 🎯 REQUEST HANDLERS - HTTP/gRPC endpoints
│   │   ├── http/                 # 🌐 HTTP REST API handlers
│   │   │   ├── middleware/       # 🛡️  HTTP middleware: cross-cutting concerns
│   │   │   │   ├── auth.go        # Authentication: JWT validation, user context
│   │   │   │   ├── cors.go        # CORS: cross-origin request handling
│   │   │   │   ├── logging.go     # Request logging: track API usage
│   │   │   │   └── rate_limit.go  # Rate limiting: prevent API abuse
│   │   │   ├── user_handler.go    # User endpoints: /api/users/* routes
│   │   │   ├── notification_handler.go # Notification endpoints: /api/notifications/*
│   │   │   └── health_handler.go  # Health check: /health endpoint for monitoring
│   │   └── grpc/                  # ⚡ gRPC handlers (if using microservices)
│   │       └── user_handler.go    # gRPC user service: high-performance API
│   │
│   ├── workers/                   # ⚙️  BACKGROUND WORKERS - Process async jobs
│   │   ├── scheduler/            # ⏱️  Task scheduling workers
│   │   │   ├── task_scheduler.go  # Schedule tasks: cron jobs, delayed execution
│   │   │   └── job_processor.go   # Process scheduled jobs: execute background work
│   │   ├── notification/         # 📮 Notification processing workers
│   │   │   ├── email_worker.go    # Process email queue: send emails asynchronously
│   │   │   ├── sms_worker.go      # Process SMS queue: send text messages
│   │   │   └── push_worker.go     # Process push notifications: mobile alerts
│   │   ├── etl/                  # 🔄 Data processing workers
│   │   │   ├── data_extractor.go  # Extract data: pull from external sources
│   │   │   ├── data_transformer.go # Transform data: clean, validate, process
│   │   │   └── data_loader.go     # Load data: save processed data to database
│   │   └── audit/                # 📈 Audit processing workers
│   │       ├── log_processor.go   # Process audit logs: aggregate, analyze
│   │       └── compliance_checker.go # Compliance monitoring: check regulations
│   │
│   ├── services/                  # 🎼 APPLICATION SERVICES - Orchestrate business logic
│   │   ├── user_service.go       # User operations: coordinate user workflows
│   │   ├── notification_service.go # Notification workflows: send multi-channel alerts
│   │   ├── task_service.go       # Task coordination: manage complex job flows
│   │   └── audit_service.go      # Audit workflows: compliance reporting
│   │
│   └── shared/                    # 🛠️  SHARED UTILITIES - Common internal code
│       ├── config/               # ⚙️  Configuration management
│       │   ├── config.go          # App configuration: load settings from env/files
│       │   └── env.go             # Environment handling: dev/staging/prod settings
│       ├── logger/               # 📝 Logging utilities
│       │   └── logger.go          # Structured logging: consistent log format
│       ├── crypto/               # 🔐 Cryptographic utilities
│       │   └── hash.go            # Password hashing: secure user credentials
│       ├── validator/            # ✅ Data validation utilities
│       │   └── validator.go       # Input validation: check user input safety
│       ├── errors/               # ❌ Error handling utilities
│       │   └── errors.go          # Custom errors: consistent error responses
│       └── types/                # 📄 Common type definitions
│           └── common.go          # Shared types: common structs, constants
│
├── pkg/                           # 📦 PUBLIC LIBRARIES - Can be imported by other projects
│   ├── client/                    # 🖥️  API clients for external use
│   │   └── api_client.go         # XCentro API client: SDK for other applications
│   └── utils/                     # 🧰 Utility functions
│       ├── time.go               # Time utilities: formatting, parsing, zones
│       ├── strings.go            # String utilities: validation, formatting
│       └── http.go               # HTTP utilities: common request/response helpers
│
├── api/                           # 📋 API SPECIFICATIONS - Documentation and contracts
│   ├── openapi/                  # 📖 OpenAPI/Swagger specifications
│   │   └── api.yaml              # REST API documentation: endpoints, schemas
│   ├── grpc/                     # ⚡ gRPC API specifications
│   │   └── proto/                # Protocol buffer definitions
│   │       ├── user.proto         # User service proto: gRPC user operations
│   │       └── notification.proto # Notification service proto: gRPC notifications
│   └── docs/                     # 📚 API documentation
│       └── api_guide.md          # API usage guide: examples, best practices
│
├── web/                           # 🎨 FRONTEND ASSETS - Static files and templates
│   ├── static/                   # Static assets served by the API
│   │   ├── css/                  # Stylesheets: admin panel, docs styling
│   │   ├── js/                   # JavaScript: admin interface, widgets
│   │   └── images/               # Images: logos, icons, assets
│   └── templates/                # HTML templates
│       └── index.html            # Admin dashboard: management interface
│
├── configs/                       # ⚙️  CONFIGURATION FILES - Environment settings
│   ├── local.yaml               # Local development: developer machine settings
│   ├── development.yaml         # Development environment: team dev server
│   ├── staging.yaml             # Staging environment: pre-production testing
│   └── production.yaml          # Production environment: live server settings
│
├── deployments/                   # 🚀 DEPLOYMENT CONFIGURATIONS - Container orchestration
│   ├── docker/                   # 🐳 Docker configurations
│   │   ├── Dockerfile.api        # API server container: web service deployment
│   │   ├── Dockerfile.worker     # Worker container: background job processing
│   │   └── docker-compose.yml    # Local development: complete stack setup
│   ├── kubernetes/               # ☸️  Kubernetes configurations
│   │   ├── api-deployment.yaml   # API deployment: production web service
│   │   ├── worker-deployment.yaml # Worker deployment: background processors
│   │   ├── configmap.yaml        # Configuration: environment variables
│   │   └── secrets.yaml          # Secrets: passwords, API keys
│   └── helm/                     # ⛵ Helm charts for Kubernetes
│       └── startup-chart/        # Helm chart: complete application deployment
│
├── scripts/                       # 🔧 BUILD & DEPLOYMENT SCRIPTS - Automation tools
│   ├── build.sh                 # Build script: compile all services
│   ├── deploy.sh                # Deployment: push to staging/production
│   ├── test.sh                  # Testing: run all tests, coverage reports
│   └── migrate.sh               # Migration: run database schema updates
│
├── migrations/                    # 🗄️  DATABASE MIGRATIONS - Schema evolution
│   ├── 001_create_users.up.sql  # Migration: create users table
│   ├── 001_create_users.down.sql # Rollback: drop users table
│   └── 002_create_notifications.up.sql # Migration: create notifications table
│
├── test/                         # 🧪 TESTING - Integration and E2E tests
│   ├── integration/              # 🔗 Integration tests: test component interactions
│   │   ├── user_test.go          # User integration: API + database + auth
│   │   └── notification_test.go  # Notification integration: full workflow testing
│   ├── fixtures/                 # 📊 Test data: sample data for testing
│   │   └── test_data.json        # Test fixtures: consistent test scenarios
│   └── utils/                    # 🛠️  Test utilities: common testing helpers
│       └── test_helpers.go       # Test helpers: database setup, mocks
│
├── docs/                         # 📖 DOCUMENTATION - Architecture and guides
│   ├── architecture.md          # System architecture: design decisions, patterns
│   ├── deployment.md            # Deployment guide: how to deploy services
│   ├── development.md           # Development setup: getting started guide
│   └── api/                     # API documentation: detailed endpoint docs
│
├── tools/                        # 🔨 DEVELOPMENT TOOLS - Code generation and linting
│   ├── codegen/                 # Code generation: auto-generate boilerplate
│   └── linter/                  # Custom linting: project-specific rules
│
├── .github/                      # 🤖 GITHUB WORKFLOWS - CI/CD automation
│   └── workflows/               # GitHub Actions: automated testing and deployment
│       ├── ci.yml               # Continuous Integration: run tests on PR
│       └── deploy.yml           # Continuous Deployment: auto-deploy on merge
│
├── Makefile                      # 🏗️  Build automation: common commands
├── Dockerfile.multi              # 🐳 Multi-stage Docker: all services in one image
├── go.mod                       # 📦 Go dependencies: xcentro.in/backend module
├── go.sum                       # 🔒 Dependency lock: reproducible builds
├── .gitignore                   # 🙈 Git ignore: exclude build artifacts, secrets
├── .dockerignore                # 🐳 Docker ignore: optimize container build
└── README.md                    # 📋 Project README: getting started guide
```

## 🎓 Key Concepts for New Developers

### 🏛️ Clean Architecture Layers (Understanding Our Structure)

**1. 🧠 Core Domain (`internal/core/`)** - **START HERE**
- **What**: Pure business logic, entities, and rules
- **Why Important**: This is the heart of XCentro - contains all business rules
- **No Dependencies**: Core never imports from adapters, handlers, or external libraries
- **Example**: User validation rules, notification templates, audit policies

**2. 🎯 Handlers (`internal/handlers/`)** - **HTTP/API Layer**
- **What**: Receives external requests (HTTP, gRPC) 
- **Responsibility**: Parse requests, call services, return responses
- **Depends On**: Services and core domains
- **Example**: `POST /api/users` → `user_handler.go` → `user_service.go`

**3. 🎼 Services (`internal/services/`)** - **Orchestration Layer**
- **What**: Coordinates between core domains and external systems
- **Responsibility**: Complex workflows, transaction management, external API calls
- **Example**: User registration (create user + send welcome email + create audit log)

**4. 🔌 Adapters (`internal/adapters/`)** - **External Integration Layer**
- **What**: Implements core interfaces to connect with external systems
- **Responsibility**: Database queries, API calls, message queues, email services
- **Example**: PostgreSQL implementation of user repository interface

**5. ⚙️ Workers (`internal/workers/`)** - **Background Processing**
- **What**: Async job processing, scheduled tasks
- **Responsibility**: Handle queued work, cron jobs, data processing
- **Example**: Send email notifications, process payment webhooks

### 🗃️ Go Module Structure

All imports use our domain: `xcentro.in/backend/`

```go
// Example imports in your Go files
import (
    "xcentro.in/backend/internal/core/user"           // Core business logic
    "xcentro.in/backend/internal/services"            // Application services  
    "xcentro.in/backend/internal/adapters/database/postgres" // Database layer
    "xcentro.in/backend/pkg/utils"                    // Public utilities
)
```

### 📊 Data Flow Example (User Registration)

```
1. 🌐 HTTP Request → internal/handlers/http/user_handler.go
2. 🎼 Handler calls → internal/services/user_service.go  
3. 🧠 Service uses → internal/core/user/service.go (business logic)
4. 🔌 Core calls → internal/core/user/repository.go (interface)
5. 🗃️ Implemented by → internal/adapters/database/postgres/user_repo.go
6. 📬 Service also calls → internal/adapters/notification/email/smtp.go
7. 📊 Service creates → audit log via internal/core/audit/service.go
```

### 🚀 Deployment Architecture 

**Single Binary, Multiple Entry Points**:
- Same codebase builds different executables
- `cmd/api/main.go` → Web server
- `cmd/scheduler/main.go` → Background scheduler  
- `cmd/notification/main.go` → Notification processor
- **Benefit**: Shared business logic, separate scaling

## 🛣️ Getting Started Guide for New Developers

### 📚 Step 1: Understand the Business (Read These First)
1. `internal/core/user/entity.go` - What is a user in XCentro?
2. `internal/core/notification/entity.go` - How do notifications work?
3. `internal/core/audit/entity.go` - How do we track user actions?
4. `internal/core/task/entity.go` - What background jobs do we run?

### 🔍 Step 2: See How Data Flows (Trace a Request)
1. Start at `internal/handlers/http/user_handler.go` - HTTP endpoints
2. Follow to `internal/services/user_service.go` - Business workflows
3. Look at `internal/core/user/service.go` - Core business rules
4. Check `internal/adapters/database/postgres/user_repo.go` - Database queries

### 🛠️ Step 3: Understand External Dependencies
1. `internal/adapters/database/` - How we store data
2. `internal/adapters/notification/` - How we send emails/SMS  
3. `internal/adapters/queue/` - How we handle background jobs
4. `internal/adapters/external/` - Third-party integrations

### 🧪 Step 4: See How We Test
1. `test/integration/` - Full workflow tests
2. Look for `*_test.go` files in each package
3. `test/fixtures/` - Sample data for testing

### ⚙️ Step 5: Development Workflow
1. `scripts/build.sh` - How to build the project
2. `scripts/test.sh` - How to run tests
3. `configs/local.yaml` - Local development settings
4. `deployments/docker/docker-compose.yml` - Local development stack

## 🎯 Common Development Tasks

### 🆕 Adding a New Feature
1. **Define the domain** in `internal/core/yourfeature/`
2. **Create the service** in `internal/services/yourfeature_service.go`  
3. **Add HTTP endpoints** in `internal/handlers/http/yourfeature_handler.go`
4. **Implement data storage** in `internal/adapters/database/postgres/yourfeature_repo.go`
5. **Add tests** in `test/integration/yourfeature_test.go`

### 🔧 Adding External Integration
1. **Define interface** in `internal/core/yourfeature/repository.go`
2. **Implement adapter** in `internal/adapters/external/yourservice/client.go`
3. **Configure** in `configs/*.yaml`
4. **Wire up** in service layer

### 📬 Adding Background Job
1. **Define job** in `internal/core/task/entity.go`
2. **Create worker** in `internal/workers/yourworker/processor.go`
3. **Add to scheduler** in `cmd/scheduler/main.go`
4. **Queue job** from services using queue adapter

## 🚨 Important Rules & Conventions

### ✅ DO
- **Core first**: Always start with business logic in `internal/core/`
- **Interface segregation**: Small, focused interfaces
- **Dependency injection**: Pass dependencies, don't create them
- **Error handling**: Always handle errors explicitly
- **Testing**: Write integration tests for workflows
- **Logging**: Use structured logging from `internal/shared/logger/`

### ❌ DON'T  
- **Core imports**: Never import adapters/handlers in core
- **Direct DB access**: Always go through repository interfaces
- **Hardcoded config**: Always use configuration files
- **Blocking operations**: Use async workers for long tasks
- **Missing validation**: Always validate user input

## 🔍 Finding Things Quickly

### 🔎 "Where do I find...?"
- **HTTP endpoints**: `internal/handlers/http/`
- **Business rules**: `internal/core/*/service.go`
- **Database queries**: `internal/adapters/database/postgres/`
- **Configuration**: `configs/` and `internal/shared/config/`
- **Background jobs**: `internal/workers/`
- **API documentation**: `api/docs/` and `api/openapi/`

### 🚀 "How do I...?"
- **Start local dev**: `docker-compose up` in `deployments/docker/`
- **Run tests**: `./scripts/test.sh`  
- **Build project**: `./scripts/build.sh`
- **Run migrations**: `./scripts/migrate.sh`
- **Deploy**: Check `deployments/kubernetes/` or `scripts/deploy.sh`

### 🆘 "I'm stuck! Help!"
1. **Check logs**: `internal/shared/logger/` for log configuration
2. **Read tests**: Integration tests show expected behavior
3. **Check configs**: `configs/local.yaml` for development settings
4. **Look at examples**: Other similar features in the same layer
5. **API docs**: `api/docs/` for endpoint documentation

---

*🎯 Remember: Start with the business logic (`internal/core/`), understand the data flow, then dive into implementation details. The folder structure guides you from abstract business concepts to concrete technical implementation.*

## 🏗️ XCentro Architecture Principles

### 1. 🎯 Single Binary, Multiple Entry Points
Each `cmd/` directory creates the same binary but runs different services:

```go
// cmd/api/main.go - Web server entry point
func main() {
    app := app.NewApp()
    app.RunAPIServer()  // Start HTTP server
}

// cmd/scheduler/main.go - Background scheduler entry point  
func main() {
    app := app.NewApp()
    app.RunScheduler()  // Start cron jobs
}

// cmd/notification/main.go - Notification worker entry point
func main() {
    app := app.NewApp()
    app.RunNotificationWorker()  // Process notification queue
}
```

**🎁 Benefits**: 
- ✅ Shared business logic across all services
- ✅ Single codebase to maintain  
- ✅ Scale services independently
- ✅ Consistent configuration and logging

### 2. 🧠 Domain-Driven Design
- **Domains live in `internal/core/`**: User, Notification, Audit, Task
- **Business rules are centralized**: All logic in domain services
- **Clean interfaces**: Each domain defines its own repository interface
- **No external dependencies**: Core never imports database/HTTP/queue libraries

### 3. 🔌 Adapter Pattern for External Systems
- **Database adapters**: `internal/adapters/database/` - Easy to swap Postgres → MySQL
- **Queue adapters**: `internal/adapters/queue/` - Switch RabbitMQ → Redis → AWS SQS  
- **Notification adapters**: `internal/adapters/notification/` - Change email providers
- **Payment adapters**: `internal/adapters/external/payment/` - Support multiple gateways

### 4. 🎼 Service Orchestration Layer
**Purpose**: Coordinate complex workflows across multiple domains

```go
// Example: User registration workflow
func (s *UserService) RegisterUser(ctx context.Context, req RegisterRequest) error {
    // 1. Create user (user domain)
    user, err := s.userCore.CreateUser(ctx, req.Email, req.Password)
    if err != nil {
        return err
    }
    
    // 2. Send welcome email (notification domain)  
    err = s.notificationCore.SendWelcomeEmail(ctx, user.Email, user.Name)
    if err != nil {
        s.logger.Error("Failed to send welcome email", "error", err)
        // Don't fail registration for email failure
    }
    
    // 3. Create audit log (audit domain)
    s.auditCore.LogUserAction(ctx, user.ID, "user_registered", req)
    
    return nil
}
```

### 5. ⚙️ Background Job Processing
- **Queue-based**: All long-running tasks go to queues
- **Worker processes**: Dedicated workers process different job types
- **Retry logic**: Built-in retry with exponential backoff
- **Dead letter queues**: Failed jobs go to DLQ for manual review

### 6. 🔒 Transaction Management Patterns

#### **Service-Level Transactions**
Services manage complex operations across multiple repositories:

```go
func (s *UserService) UpdateUserProfile(ctx context.Context, userID int64, profile ProfileUpdate) error {
    return s.db.WithTransaction(ctx, func(tx *sql.Tx) error {
        // Update user profile
        err := s.userRepo.UpdateProfile(ctx, tx, userID, profile)
        if err != nil {
            return err
        }
        
        // Create audit log
        err = s.auditRepo.LogAction(ctx, tx, userID, "profile_updated", profile)
        if err != nil {
            return err  
        }
        
        // Send notification (async, so not in transaction)
        s.queueAdapter.EnqueueJob(ctx, "notification", NotificationJob{
            UserID: userID,
            Type:   "profile_updated",
        })
        
        return nil
    })
}
```

#### **Repository-Level Transactions**
Individual repositories handle multi-table operations:

```go 
func (r *PostgresUserRepo) CreateUserWithProfile(ctx context.Context, tx *sql.Tx, user User, profile Profile) error {
    // Insert user
    userID, err := r.insertUser(ctx, tx, user)
    if err != nil {
        return err
    }
    
    // Insert profile
    profile.UserID = userID
    return r.insertProfile(ctx, tx, profile)
}
```

*🎯 This structure grows with your startup - start simple, add complexity only when needed. Focus on business value first, technical perfection second.*
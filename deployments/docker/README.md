# XCentro Database Stack

A clean, simple Docker setup for PostgreSQL and Redis databases used by the XCentro backend application.

## Quick Start

1. **Start the database services:**
   ```bash
   ./start.sh
   ```

2. **Set up PostgreSQL users:**
   ```bash
   ./setup-postgres-users.sh
   ```

3. **Set up Redis users:**
   ```bash
   ./setup-redis-users.sh
   ```

4. **Connect to databases:**
   ```bash
   # PostgreSQL
   ./connect-postgres.sh [user]
   
   # Redis
   ./connect-redis.sh [user]
   ```

## Architecture

### PostgreSQL Setup
- **Container**: `xcentro-postgres` (PostgreSQL 18 Alpine)
- **Database**: `xcentro` 
- **Superuser**: `postgres` (for administrative tasks)
- **Port**: 5432

**Application Users** (created by `setup-postgres-users.sh`):
- **Owner** (`xcentro_owner`): Full database ownership and permissions
- **Admin** (`xcentro_admin`): Can manage tables, indexes, schemas (for migrations)
- **Read-Write** (`xcentro_rw`): Standard CRUD operations (for application)

### Redis Setup
- **Container**: `xcentro-redis` (Redis 8 Alpine)
- **Superuser**: Password-protected (defined in `.env`)
- **Port**: 6379

**Application Users** (created by `setup-redis-users.sh`):
- **Superuser**: Full access to all Redis commands and keys
- **Application User** (`xcentro_redis`): Read-write access to `xcentro:*` keys only

## Configuration

All configuration is managed through the `.env` file. Use `.env.example` as a template:

```bash
# Copy the example file
cp .env.example .env
```

**Configuration variables:**

```bash
# PostgreSQL Configuration
POSTGRES_SUPERUSER=postgres
POSTGRES_SUPERUSER_PASSWORD=SuperSecure123!
POSTGRES_DB=xcentro
POSTGRES_PORT=5432

# Application Database Users
DB_OWNER_USER=xcentro_owner
DB_OWNER_PASSWORD=OwnerSecure456!
DB_ADMIN_USER=xcentro_admin
DB_ADMIN_PASSWORD=AdminSecure789!
DB_RW_USER=xcentro_rw
DB_RW_PASSWORD=RwSecure101!

# Redis Configuration
REDIS_PORT=6379
REDIS_SUPERUSER=redis_super
REDIS_SUPERUSER_PASSWORD=RedisSuper123!
REDIS_APP_USER=xcentro_redis
REDIS_APP_PASSWORD=XcentroRedis456!
```

## Scripts

### Core Scripts

- **`start.sh`**: Start PostgreSQL and Redis containers
- **`setup-postgres-users.sh`**: Create PostgreSQL application users
- **`setup-redis-users.sh`**: Create Redis application users
- **`connect-postgres.sh [user]`**: Connect to PostgreSQL
- **`connect-redis.sh [user]`**: Connect to Redis

### Connection Examples

**PostgreSQL:**
```bash
# Interactive connections
./connect-postgres.sh                    # As superuser (default)
./connect-postgres.sh owner              # As owner
./connect-postgres.sh admin              # As admin (for migrations)
./connect-postgres.sh rw                 # As read-write (for app)

# Execute commands
./connect-postgres.sh admin -c '\du'     # List users
./connect-postgres.sh -l                 # List all users

# Execute SQL files
./connect-postgres.sh owner -f schema.sql
```

**Redis:**
```bash
# Interactive connections
./connect-redis.sh                       # As superuser (default)
./connect-redis.sh app                   # As application user

# Execute commands
./connect-redis.sh super -c 'INFO'       # Show Redis info
./connect-redis.sh app -c 'SET xcentro:test hello'

# Tests and diagnostics
./connect-redis.sh -p                    # Ping test
./connect-redis.sh -t                    # Full connection tests
./connect-redis.sh -u                    # List ACL users
```

## Management Commands

```bash
# View service status
docker compose ps

# View logs
docker compose logs [postgres|redis]

# Stop services
docker compose down

# Stop and remove data (reset)
docker compose down -v

# Restart services
docker compose restart

# Rebuild and restart
docker compose up -d --build
```

## Security Features

### PostgreSQL Security
- Separate users for different access levels
- Least-privilege principle (RW user cannot create/drop tables)
- Strong password requirements
- Database ownership properly assigned

### Redis Security
- Default user disabled
- ACL-based user management
- Application user restricted to specific key patterns (`xcentro:*`)
- Dangerous commands blocked for application user
- Password authentication required

## Data Persistence

- **PostgreSQL data**: Stored in Docker volume `pg_data`
- **Redis data**: Stored in Docker volume `redis_data`
- Data persists across container restarts
- Use `docker compose down -v` to reset all data

## Troubleshooting

### Services not starting
```bash
# Check service status
docker compose ps

# Check logs
docker compose logs postgres
docker compose logs redis

# Restart services
docker compose restart
```

### Connection issues
```bash
# Test PostgreSQL connection
./connect-postgres.sh -l

# Test Redis connection
./connect-redis.sh -p

# Run full Redis tests
./connect-redis.sh -t
```

### Reset everything
```bash
# Stop services and remove all data
docker compose down -v

# Remove containers and images (if needed)
docker compose down --rmi all

# Start fresh
./start.sh
./setup-postgres-users.sh
./setup-redis-users.sh
```

## Development Workflow

1. **Initial setup** (once):
   ```bash
   ./start.sh
   ./setup-postgres-users.sh
   ./setup-redis-users.sh
   ```

2. **Daily development**:
   ```bash
   # Start services (if not running)
   docker compose up -d
   
   # Connect for development/testing
   ./connect-postgres.sh rw
   ./connect-redis.sh app
   ```

3. **Migration workflow**:
   ```bash
   # Connect as admin user for schema changes
   ./connect-postgres.sh admin
   ```

4. **Cleanup**:
   ```bash
   # Stop when done
   docker compose down
   ```

## Best Practices

1. **Use appropriate users**: Don't use superuser for application connections
2. **Regular backups**: Consider setting up automated backups for production
3. **Password security**: Change default passwords in `.env` before production
4. **Resource limits**: Consider adding resource limits to compose file for production
5. **Monitoring**: Add health checks and monitoring for production deployments

# Danger!
### To prune everything if needed in dev mode.
1) More than sufficient
```
docker compose down -v
```

2) More thorough cleanup
```
# Remove containers, volumes, and images
docker compose down -v --rmi all

# Remove any orphaned containers
docker compose down -v --remove-orphans
```

3) Nuclear option (clears everything Docker-related)
```
# Remove all stopped containers
docker container prune -f

# Remove all unused volumes
docker volume prune -f

# Remove all unused networks
docker network prune -f

# Remove all unused images
docker image prune -a -f
```
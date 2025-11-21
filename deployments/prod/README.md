# Production Deployment

This folder contains all production deployment configurations and automation scripts.

## Quick Start

### One-Command Deployment

```bash
# 1. Create environment file
cp .env.example .env
nano .env  # Add your credentials

# 2. Test with staging environment first (recommended)
./bootstrap-production.sh \
  --email admin@xcentro.in \
  --staging \
  -v

# 3. Deploy to production after staging works
./bootstrap-production.sh \
  --email admin@xcentro.in \
  -v

# OR with logs in foreground (for debugging)
./bootstrap-production.sh --email admin@xcentro.in --logs -v

# OR skip SSL for testing services without certificates
./bootstrap-production.sh --skip-ssl --logs
```

This automatically:
- ✅ Creates dummy SSL certificates (prevents nginx crash)
- ✅ Starts nginx with dummy certificates
- ✅ Validates domains are accessible via HTTP
- ✅ Removes dummy certificates (while nginx runs)
- ✅ Requests real Let's Encrypt certificates via ACME challenge
- ✅ Reloads nginx with real certificates
- ✅ Starts all services (PostgreSQL, Redis, API, frontend)
- ✅ Creates database users automatically (owner, xcentro_rw)
- ✅ Creates Redis ACL users automatically (redis_super, xcentro_app)
- ✅ Runs migrations
- ✅ Sets up automatic certificate renewal (every 12 hours check)
- ✅ Sets up automatic nginx reload (every 6 hours)

**Options:**
- `-e, --email EMAIL` - Email for SSL notifications (required unless --skip-ssl)
- `-d, --domains DOMAINS` - Comma-separated domains (default: api.xcentro.in,app.xcentro.in)
- `-s, --staging` - Use Let's Encrypt staging environment (for testing, no rate limits)
- `--break-my-certs` - Delete existing certificates before requesting new ones
- `-v, --verbose` - Show verbose certbot output for debugging
- `--skip-ssl` - Skip SSL setup (nginx and certbot won't start)
- `--logs` - Show Docker logs in foreground (default: detached mode)
- `-h, --help` - Show help message

### Manual Deployment

**Step 1: Create environment file**
```bash
cp .env.example .env
nano .env  # Add your production credentials
```

**Step 2: Setup SSL certificates**
```bash
./init-letsencrypt.sh --email admin@xcentro.in
```

**Step 3: Start all services**
```bash
docker compose -f docker-compose.yml up -d
```

Database and Redis users are created automatically via init scripts.

## Files Overview

### Scripts
- **`bootstrap-production.sh`** - One-command automated deployment
- **`init-letsencrypt.sh`** - SSL certificate setup and automation
- **`manage-certs.sh`** - SSL certificate management (list, renew, test)
- **`init-postgres.sql`** - PostgreSQL user auto-setup script
- **`init-redis.sh`** - Redis ACL user auto-setup script
- **`redis-entrypoint.sh`** - Production Redis entrypoint with auto-init

### Configuration
- **`docker-compose.yml`** - Production services orchestration
- **`nginx/`** - nginx reverse proxy and SSL termination configs
  - `nginx.conf` - Main nginx configuration
  - `conf.d/api.xcentro.in.conf` - API backend config
  - `conf.d/app.xcentro.in.conf` - Frontend app config

## Self-Sufficiency

This folder is **completely self-sufficient** for production deployment:
- ✅ All scripts included
- ✅ All configuration files included
- ✅ nginx configs included
- ✅ Init scripts included
- ✅ Environment file template included
- ✅ **ZERO dependencies on other folders**

This folder can be deployed anywhere independently!

## Prerequisites

1. **DNS configured**: Domains point to your server
2. **Ports open**: Firewall allows 80, 443
3. **Docker installed**: Docker and Docker Compose
4. **Environment file**: Create `.env` in this folder

## Environment Setup

**Before deployment, create your `.env` file:**

```bash
cd deployments/prod
cp .env.example .env
nano .env  # Edit with your production credentials
```

Required variables in `.env`:

```env
# PostgreSQL
POSTGRES_SUPERUSER_PASSWORD=...
DB_OWNER_PASSWORD=...
DB_RW_PASSWORD=...

# Redis
REDIS_SUPERUSER_PASSWORD=...
REDIS_APP_PASSWORD=...

# Application
JWT_SECRET_KEY=...
```

## Container Names

All containers have consistent naming for easy management:

| Service | Container Name |
|---------|----------------|
| PostgreSQL | `xcentro-postgres` |
| Redis | `xcentro-redis` |
| Migration | `xcentro-migrate` |
| API | `xcentro-api` |
| Notification Worker | `xcentro-notification-worker` |
| Frontend | `xcentro-frontend` |
| nginx | `xcentro-nginx` |
| Certbot | `xcentro-certbot` |

## Common Commands

```bash
# View service status
docker compose -f docker-compose.yml ps

# View all logs
docker compose -f docker-compose.yml logs -f

# View specific service logs (by service name)
docker compose -f docker-compose.yml logs -f api

# View specific service logs (by container name)
docker logs -f xcentro-api

# Stop all services
docker compose -f docker-compose.yml down

# Stop and remove volumes (DESTRUCTIVE!)
docker compose -f docker-compose.yml down -v

# Restart a service
docker compose -f docker-compose.yml restart api

# Execute command in running container
docker exec -it xcentro-api /bin/sh
docker exec -it xcentro-postgres psql -U postgres -d xcentro
```

## SSL Certificate Management

### Automatic Renewal (Default)

Certificates are **automatically renewed** every 30 days before expiry:
- ✅ Certbot checks every 12 hours
- ✅ Renews when certificate has < 30 days remaining
- ✅ nginx reloads every 6 hours to pick up new certificates
- ✅ **Zero manual intervention required**

### Manual Certificate Management

```bash
# List certificates and expiry dates
./manage-certs.sh list

# Test renewal (dry-run, safe)
./manage-certs.sh test

# Force renewal
./manage-certs.sh renew

# Add new domain
./manage-certs.sh add newdomain.com

# View certbot logs
./manage-certs.sh logs

# Reload nginx
./manage-certs.sh reload-nginx
```

### Certificate Lifecycle

```
Day 0:   Certificate issued (90-day validity)
Day 60:  Certbot starts renewal attempts (30 days before expiry)
Day 60:  ✅ Certificate renewed
Day 60:  Within 6 hours, nginx picks up new certificate
Day 150: Next renewal cycle begins (new cert has 90 days)
```

### Staging vs Production Certificates

**Staging (Testing):**
- Use `--staging` flag
- Issued by "(STAGING) Let's Encrypt"
- Not trusted by browsers (⚠️ shows certificate warning)
- **No rate limits** - test as much as you want!
- Perfect for testing DNS, firewall, ACME challenges

**Production:**
- No `--staging` flag
- Issued by "Let's Encrypt"
- Trusted by all browsers (✅ green padlock)
- **Rate limit:** 50 certificates per domain per week
- Use after staging tests pass

**To switch from staging to production:**
```bash
# Stop services
docker compose -f docker-compose.yml down

# Deploy with production certificates
./bootstrap-production.sh --email admin@xcentro.in --break-my-certs -v
```

## Database & Redis Access

Use the connection scripts from the local development folder:

```bash
# Connect to PostgreSQL
cd ../docker
./connect-postgres.sh owner

# Connect to Redis
./connect-redis.sh app
```

## Automatic User Setup

### PostgreSQL
**File:** `init-postgres.sql`

Runs automatically on first container start (via `/docker-entrypoint-initdb.d/`).

Creates:
- `owner` user - DDL operations, migrations
- `xcentro_rw` user - Application CRUD
- `xcentro` schema with proper permissions

### Redis
**File:** `init-redis.sh`

Runs automatically from redis-entrypoint (every start, but idempotent).

Creates:
- `redis_super` user - Full admin access
- `xcentro_app` user - Restricted to `xcentro:*` keys
- Persisted ACL rules

## Architecture

```
Internet (80, 443)
      │
 ┌────▼────┐
 │  nginx  │ ← SSL termination
 │  :443   │ ← Reverse proxy
 └────┬────┘
      │
 ┌────▼────┐
 │   API   │
 │   :80   │
 └────┬────┘
      │
 ┌────▼────┬────────┐
 │         │        │
┌▼──┐  ┌──▼──┐  ┌──▼──┐
│PG │  │Redis│  │cert │
│:5432  │:6379│  │bot  │
└────┘  └─────┘  └─────┘
  │       │
  │       │
init-    init-
postgres redis
(auto)   (auto)
```

## Troubleshooting

### Services not starting
```bash
# Check logs
docker compose -f docker-compose.yml logs

# Check specific service
docker compose -f docker-compose.yml logs postgres
```

### Database users not created
```bash
# Users only created if volume is empty on first start
# To reset:
docker compose -f docker-compose.yml down -v
docker compose -f docker-compose.yml up -d
```

### Redis users not created
```bash
# Restart Redis (init script runs on start)
docker compose -f docker-compose.yml restart redis
```

### SSL certificate issues
```bash
# Test with staging first
./init-letsencrypt.sh -e admin@xcentro.in --staging

# Check DNS
dig +short api.xcentro.in

# Verify ACME challenge works
curl http://api.xcentro.in/.well-known/acme-challenge/test
```

## Documentation

- [Complete Production Guide](../../docs/production_deployment.md)
- [SSL Certificate Management](../../docs/ssl_certificates.md)
- [Deployment Summary](../../DEPLOYMENT_SUMMARY.md)
- [Main README](../../README.md)

## Support

For issues or questions, see:
- Check service logs: `docker compose logs <service>`
- Review documentation in `docs/` folder
- Verify environment variables in `.env`

# XCentro Deployments

This folder contains deployment configurations for different environments.

## Folder Structure

```
deployments/
├── docker/     # Local development environment
├── local/      # Reserved for local customizations
└── prod/       # Production deployment (automated)
```

## Quick Start

### Local Development

```bash
cd docker
./start.sh                      # Start PostgreSQL and Redis
./setup-postgres-users.sh       # Create database users
./setup-redis-users.sh          # Create Redis ACL users
```

Then from project root:
```bash
go run ./cmd/migrate            # Run migrations
air                             # Start API with hot reload
```

### Production Deployment

```bash
cd prod

# Test with staging first (recommended)
./bootstrap-production.sh --email admin@xcentro.in --staging -v

# Deploy to production after staging works
./bootstrap-production.sh --email admin@xcentro.in -v
```

This automatically sets up everything:
- ✅ SSL certificates (Let's Encrypt via ACME HTTP-01 challenge)
- ✅ Automatic certificate renewal (every 30 days before expiry)
- ✅ Database users (owner, xcentro_rw)
- ✅ Redis ACL users (redis_super, xcentro_app)
- ✅ All services running with health checks
- ✅ nginx reverse proxy with SSL termination
- ✅ Containerized certbot for certificate management

## Folders

### `docker/` - Local Development

Traditional development setup with manual user configuration.

**Tools:**
- `start.sh` - Start PostgreSQL and Redis
- `setup-postgres-users.sh` - Create database users
- `setup-redis-users.sh` - Create Redis ACL users
- `connect-postgres.sh` - Interactive PostgreSQL connection
- `connect-redis.sh` - Interactive Redis connection

**Use for:**
- Local development
- Testing changes
- Manual database operations

### `prod/` - Production

Fully automated production deployment with containerized SSL certificate management.

**Scripts:**
- `bootstrap-production.sh` - One-command automated deployment
- `init-letsencrypt.sh` - SSL certificate setup (called by bootstrap)
- `manage-certs.sh` - Certificate management utilities
- `init-postgres.sql` - PostgreSQL user auto-creation
- `init-redis.sh` - Redis ACL user auto-creation
- `redis-entrypoint.sh` - Production Redis entrypoint

**Configuration:**
- `docker-compose.production.yml` - Production orchestration
- `nginx/` - nginx configs with SSL termination
- `.env.example` - Environment template

**Key Features:**
- ✅ Containerized certbot (no VM dependencies)
- ✅ Automatic SSL certificates via Let's Encrypt ACME HTTP-01 challenge
- ✅ Auto-renewal every 12 hours (renews 30 days before expiry)
- ✅ Automatic nginx reload every 6 hours (picks up renewed certs)
- ✅ Database users created on first start (via init script)
- ✅ Redis ACL users created on every start (idempotent)
- ✅ nginx reverse proxy with SSL termination
- ✅ Health checks and service dependencies
- ✅ Staging support for testing (--staging flag)
- ✅ Completely self-sufficient folder (no external dependencies)

**Container Names:**
- `xcentro-postgres` - PostgreSQL database
- `xcentro-redis` - Redis cache
- `xcentro-migrate` - Database migrations
- `xcentro-api` - API server
- `xcentro-notification-worker` - Background worker
- `xcentro-frontend` - Frontend application
- `xcentro-nginx` - Reverse proxy
- `xcentro-certbot` - Certificate management

**Use for:**
- Production deployments
- Staging environments
- Any internet-facing deployment requiring SSL

### `local/` - Reserved

Reserved for environment-specific customizations that shouldn't be committed.

Add to `.gitignore` if needed for local overrides.

## Comparison

| Feature | Local (docker/) | Production (prod/) |
|---------|----------------|-------------------|
| SSL Certificates | ❌ Not configured | ✅ Automatic Let's Encrypt (ACME HTTP-01) |
| Certificate Renewal | ❌ N/A | ✅ Every 12h check, renews 30d before expiry |
| Certbot | ❌ Not included | ✅ Containerized, always running |
| nginx | ❌ Not included | ✅ Reverse proxy + SSL termination |
| DB Users | 🔧 Manual setup (`setup-postgres-users.sh`) | ✅ Auto-created on first start |
| Redis ACL Users | 🔧 Manual setup (`setup-redis-users.sh`) | ✅ Auto-created on every start |
| Container Names | ❌ Not named | ✅ Named (xcentro-*) |
| Health Checks | ❌ Not configured | ✅ All services |
| Service Dependencies | ❌ Manual ordering | ✅ Automatic via depends_on |
| Setup Time | ~5 minutes | ~2-3 minutes |
| Commands Needed | 5-6 commands | 1 command |
| Self-Sufficient | ❌ Needs parent folder | ✅ Fully independent |

## Migration Path

### From Local to Production

Local development doesn't affect production. They're completely separate.

```bash
# Continue using local for development
cd deployments/docker
./start.sh

# Deploy to production separately
cd ../prod
./bootstrap-production.sh --email admin@xcentro.in
```

### From Old Production Setup

If you were using the old production setup (manual certbot, manual user scripts):

```bash
# Backup existing certs (optional)
sudo cp -r /etc/letsencrypt /root/letsencrypt-backup

# New automated deployment
cd deployments/prod
./bootstrap-production.sh --email admin@xcentro.in
```

The new setup will:
- Create fresh SSL certificates (or copy existing if you want)
- Set up database users automatically
- Set up Redis users automatically
- Configure everything correctly

## Documentation

- **Local Development**: See `docker/README.md`
- **Production Deployment**: See `prod/README.md`
- **Complete Guide**: See `../docs/production_deployment.md`
- **SSL Management**: See `../docs/ssl_certificates.md`

## Which Should I Use?

**Use `docker/` (Local) when:**
- Developing locally
- Testing changes
- Need manual control over database
- Don't need SSL

**Use `prod/` (Production) when:**
- Deploying to internet-facing server
- Need SSL certificates
- Want automated setup
- Production or staging environments

## Support

For issues:
1. Check service logs: `docker compose logs <service>`
2. Review documentation in respective folder's README
3. See troubleshooting in `docs/production_deployment.md`

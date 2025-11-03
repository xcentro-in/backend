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
./bootstrap-production.sh --email admin@xcentro.in
```

This automatically sets up everything:
- SSL certificates
- Database users
- Redis ACL users
- All services running

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

Fully automated production deployment with SSL, auto-renewal, and init scripts.

**Tools:**
- `bootstrap-production.sh` - One-command deployment
- `init-letsencrypt.sh` - SSL certificate automation
- `manage-certs.sh` - Certificate management
- `docker-compose.production.yml` - Production orchestration

**Features:**
- ✅ Automatic SSL certificates (Let's Encrypt)
- ✅ Auto-renewal configured
- ✅ Database users created automatically
- ✅ Redis ACL users created automatically
- ✅ nginx reverse proxy with SSL termination
- ✅ Health checks and dependencies

**Use for:**
- Production deployments
- Staging environments
- Any public-facing deployment

### `local/` - Reserved

Reserved for environment-specific customizations that shouldn't be committed.

Add to `.gitignore` if needed for local overrides.

## Comparison

| Feature | Local (docker/) | Production (prod/) |
|---------|----------------|-------------------|
| SSL | ❌ Not configured | ✅ Automatic Let's Encrypt |
| DB Users | 🔧 Manual setup | ✅ Auto-created |
| Redis Users | 🔧 Manual setup | ✅ Auto-created |
| nginx | ❌ Not included | ✅ Reverse proxy + SSL |
| Auto-renewal | ❌ N/A | ✅ Certbot cron |
| Setup Time | ~5 minutes | ~3 minutes |
| Commands | 5-6 commands | 1 command |

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

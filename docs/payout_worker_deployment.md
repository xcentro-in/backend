# Payout Worker Deployment Guide

## Overview

The payout worker is a scheduled background service that:
1. **Task 1**: Auto-retries stuck transfers (status='initiated')
2. **Task 2**: Flags timeout transfers (no webhook after 60 min)
3. **Task 3**: Retry failed transfers (placeholder for future)
4. **Task 4**: Auto-payout for settled payments (main functionality)

## Docker Image

### Build Locally
```bash
docker build -f Dockerfile.payout_worker -t payout-worker:latest .
```

### Image Details
- **Base Image**: golang:1.25.1-alpine (builder), alpine:latest (runtime)
- **Size**: ~32MB
- **User**: Non-root (appuser:1000)
- **Binary**: `/app/payout_worker`

## GitHub Container Registry

The worker is automatically built and pushed to GHCR on:
- Push to `main` branch
- Pull requests to `main`
- Version tags (`v*`)

### Image Tags
```
ghcr.io/xcentro-in/backend-payout-worker:latest
ghcr.io/xcentro-in/backend-payout-worker:main
ghcr.io/xcentro-in/backend-payout-worker:v1.0.0
ghcr.io/xcentro-in/backend-payout-worker:main-<sha>
```

### Pull Image
```bash
docker pull ghcr.io/xcentro-in/backend-payout-worker:latest
```

## Configuration

The worker requires a `.env` file with the following variables:

### Required Environment Variables
```bash
# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=xcentro
DB_USER=xcentro_rw
DB_PASSWORD=<password>
DB_SCHEMA=xcentro

# Castler Payout Provider
CASTLER_BASE_URL=https://api-uat.ncome.in/api
CASTLER_API_KEY=<api_key>
CASTLER_SECRET_KEY=<secret_key>
CASTLER_ENCRYPTION_KEY=<encryption_key>
CASTLER_ENCRYPTION_IV=<encryption_iv>
CASTLER_ESCROW_ID=<escrow_id>

# Bank Account Encryption
BANK_ACCOUNT_ENCRYPTION_KEY=<32_byte_key>

# Auto-Payout Worker Configuration
AUTO_PAYOUT_WORKER_ENABLED=true
AUTO_PAYOUT_POLL_INTERVAL_MINUTES=60
AUTO_PAYOUT_BATCH_SIZE=50

```

## Running the Worker

### With Docker
```bash
# Using .env file
docker run --env-file .env ghcr.io/xcentro-in/backend-payout-worker:latest

# Using environment variables
docker run \
  -e DB_HOST=postgres \
  -e DB_PORT=5432 \
  -e DB_NAME=xcentro \
  -e DB_USER=xcentro_rw \
  -e DB_PASSWORD=<password> \
  -e AUTO_PAYOUT_WORKER_ENABLED=true \
  -e AUTO_PAYOUT_POLL_INTERVAL_MINUTES=60 \
  ghcr.io/xcentro-in/backend-payout-worker:latest
```

### With Docker Compose
```yaml
version: '3.8'

services:
  payout-worker:
    image: ghcr.io/xcentro-in/backend-payout-worker:latest
    env_file:
      - .env
    depends_on:
      - postgres
    restart: unless-stopped
    networks:
      - backend
```

### Locally (Development)
```bash
# Build binary
go build -o bin/payout_worker ./cmd/workers/payout_worker

# Run
./bin/payout_worker
```

## Monitoring

### Logs
```bash
# Docker logs
docker logs -f <container_id>

# Search for specific tasks
docker logs <container_id> | grep "Task 1"
docker logs <container_id> | grep "Task 4"
docker logs <container_id> | grep "Auto-Payout Summary"
```

### Key Log Messages
- `Worker scheduler started (runs every X minutes)` - Worker initialized
- `Task 1: Monitoring initiated transfers` - Checking for stuck transfers
- `Task 2: Flagging timeout transfers` - Checking for webhook timeouts
- `Task 4: Auto-Payout - Processing settled payments` - Main auto-payout task
- `Auto-Payout Summary` - Batch processing results

### Health Checks
The worker runs continuously based on poll interval. Monitor:
- Database connectivity
- Castler API availability
- Transfer processing success rate
- Error logs for failed transfers

## Troubleshooting

### Worker Not Starting
1. Check `.env` file exists and has correct values
2. Verify database connectivity: `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`
3. Check logs: `docker logs <container_id>`

### Transfers Stuck in 'initiated'
- Task 1 will auto-retry these every poll interval
- Check Castler API credentials
- Verify `CASTLER_BASE_URL` is correct

### No Auto-Payouts Happening
1. Verify `AUTO_PAYOUT_WORKER_ENABLED=true`
2. Check poll interval: `AUTO_PAYOUT_POLL_INTERVAL_MINUTES`
3. Confirm there are settled payments: `settlement_status='settled'`
4. Review logs: `docker logs <container_id> | grep "Task 4"`

### Timeout Transfers
- Task 2 flags these after `PAYOUT_WEBHOOK_TIMEOUT_MINUTES`
- Check Castler webhook configuration
- Verify webhook endpoint is accessible

## CI/CD Workflow

GitHub Actions automatically:
1. Builds Docker image on push to `main`
2. Pushes to GitHub Container Registry
3. Tags with branch name, commit SHA, and version

### Workflow File
`.github/workflows/docker-build.yml`

### Manual Build Trigger
```bash
# Push to main branch
git push origin main

# Or create version tag
git tag v1.0.0
git push origin v1.0.0
```

## Production Deployment

### Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payout-worker
spec:
  replicas: 1  # Only 1 instance to avoid duplicate processing
  selector:
    matchLabels:
      app: payout-worker
  template:
    metadata:
      labels:
        app: payout-worker
    spec:
      containers:
      - name: payout-worker
        image: ghcr.io/xcentro-in/backend-payout-worker:latest
        envFrom:
        - secretRef:
            name: payout-worker-secrets
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
```

### Important Notes
- **Single Instance**: Only run ONE instance to avoid duplicate processing
- **Graceful Shutdown**: Worker handles SIGTERM gracefully
- **Resource Usage**: Lightweight (~100MB RAM, minimal CPU)

## Performance

- **Image Size**: 31.9MB
- **Memory Usage**: ~100-150MB
- **CPU Usage**: Minimal (spikes during batch processing)
- **Poll Interval**: Configurable (default: 60 minutes)
- **Batch Size**: Configurable (default: 50 payments per run)

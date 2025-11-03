#!/bin/bash

# XCentro Production Bootstrap Script
# One-command setup for production deployment with SSL certificates

set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_header() {
    echo -e "${BLUE}=========================================="
    echo "XCentro Production Bootstrap"
    echo "==========================================${NC}"
    echo ""
}

print_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -e, --email EMAIL          Email for SSL certificate notifications (required)"
    echo "  -d, --domains DOMAINS      Comma-separated domains (default: api.xcentro.in,app.xcentro.in)"
    echo "  -s, --staging              Use Let's Encrypt staging environment"
    echo "  --skip-ssl                 Skip SSL certificate setup"
    echo "  --logs                     Show logs (foreground mode) instead of detached mode"
    echo "  -h, --help                 Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 --email admin@xcentro.in"
    echo "  $0 -e admin@xcentro.in -d api.com,app.com"
    echo "  $0 --skip-ssl              # Start services without SSL setup"
    echo "  $0 -e admin@xcentro.in --logs  # Show logs in foreground"
    echo ""
}

# Default values
EMAIL=""
DOMAINS="api.xcentro.in,app.xcentro.in"
STAGING_FLAG=""
SKIP_SSL=0
SHOW_LOGS=0

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -e|--email)
            EMAIL="$2"
            shift 2
            ;;
        -d|--domains)
            DOMAINS="$2"
            shift 2
            ;;
        -s|--staging)
            STAGING_FLAG="--staging"
            shift
            ;;
        --skip-ssl)
            SKIP_SSL=1
            shift
            ;;
        --logs)
            SHOW_LOGS=1
            shift
            ;;
        -h|--help)
            print_usage
            exit 0
            ;;
        *)
            echo -e "${RED}ERROR: Unknown option: $1${NC}"
            echo ""
            print_usage
            exit 1
            ;;
    esac
done

print_header

# Validate email if not skipping SSL
if [ $SKIP_SSL -eq 0 ] && [ -z "$EMAIL" ]; then
    echo -e "${RED}ERROR: Email is required for SSL setup${NC}"
    echo "Use --skip-ssl to start without SSL, or provide --email"
    echo ""
    print_usage
    exit 1
fi

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

if ! command -v docker &> /dev/null; then
    echo -e "${RED}ERROR: Docker is not installed${NC}"
    exit 1
fi

if ! command -v docker compose &> /dev/null; then
    echo -e "${RED}ERROR: Docker Compose is not installed${NC}"
    exit 1
fi

if [ ! -f .env ]; then
    echo -e "${YELLOW}WARNING: .env file not found${NC}"
    if [ -f .env.example ]; then
        echo "Creating .env from .env.example..."
        cp .env.example .env
        echo -e "${YELLOW}Please edit .env file with your credentials before proceeding${NC}"
        read -p "Press Enter when ready to continue..."
    else
        echo -e "${RED}ERROR: No .env.example found. Cannot create .env${NC}"
        exit 1
    fi
fi

echo -e "${GREEN}✓ Prerequisites check passed${NC}"
echo ""

# Summary
echo -e "${BLUE}Deployment Configuration:${NC}"
if [ $SKIP_SSL -eq 0 ]; then
    echo "  Email: $EMAIL"
    echo "  Domains: $DOMAINS"
    [ -n "$STAGING_FLAG" ] && echo "  Mode: STAGING (testing)" || echo "  Mode: PRODUCTION"
else
    echo "  SSL: Skipped (manual setup required)"
fi
[ $SHOW_LOGS -eq 1 ] && echo "  Logs: Foreground (showing logs)" || echo "  Logs: Detached mode"
echo ""
read -p "Continue with deployment? (y/n) " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
fi

echo ""
echo -e "${BLUE}=========================================="
echo "Step 1: Setting up SSL certificates"
echo "==========================================${NC}"

if [ $SKIP_SSL -eq 0 ]; then
    # Run init-letsencrypt.sh
    echo "Running SSL certificate initialization..."
    ./init-letsencrypt.sh --email "$EMAIL" --domains "$DOMAINS" $STAGING_FLAG

    if [ $? -ne 0 ]; then
        echo -e "${RED}ERROR: SSL setup failed${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ SSL certificates configured${NC}"
else
    echo -e "${YELLOW}Skipping SSL setup as requested${NC}"
    echo "You'll need to run ./init-letsencrypt.sh manually later"
fi

echo ""
echo -e "${BLUE}=========================================="
echo "Step 2: Starting all services"
echo "==========================================${NC}"

# Start all services
# Determine which services to start
if [ $SKIP_SSL -eq 1 ]; then
    # Skip nginx and certbot when SSL is skipped
    SERVICES="postgres redis migrate api notification-worker frontend"
    echo -e "${YELLOW}Note: nginx and certbot are not started (--skip-ssl mode)${NC}"
else
    SERVICES=""  # Start all services
fi

if [ $SHOW_LOGS -eq 1 ]; then
    echo ""
    echo -e "${YELLOW}Starting services in foreground mode (showing logs)...${NC}"
    echo "Press Ctrl+C to stop"
    echo ""
    docker compose -f docker-compose.production.yml up $SERVICES
else
    echo ""
    docker compose -f docker-compose.production.yml up -d $SERVICES

    echo ""
    echo "Waiting for services to be healthy..."
    sleep 5

    # Check service health
    echo ""
    echo -e "${BLUE}Service Status:${NC}"
    docker compose -f docker-compose.production.yml ps

    echo ""
    echo -e "${GREEN}=========================================="
    echo "✓ Production deployment complete!"
    echo "==========================================${NC}"
    echo ""
    echo -e "${BLUE}Next steps:${NC}"
    echo "1. Verify services are running:"
    echo "   docker compose -f docker-compose.production.yml ps"
    echo ""
    echo "2. Check logs if needed:"
    echo "   docker compose -f docker-compose.production.yml logs -f"
    echo ""
    echo "3. Access your application:"
    if [ $SKIP_SSL -eq 0 ]; then
        IFS=',' read -ra DOMAIN_ARRAY <<< "$DOMAINS"
        for domain in "${DOMAIN_ARRAY[@]}"; do
            echo "   https://$domain"
        done
    else
        echo "   Configure SSL first using: ./init-letsencrypt.sh --email your@email.com"
    fi
    echo ""
    echo -e "${BLUE}Management commands:${NC}"
    echo "  • View certificate status: ./manage-certs.sh list"
    echo "  • Stop services: docker compose -f docker-compose.production.yml down"
    echo "  • View logs: docker compose -f docker-compose.production.yml logs -f [service]"
    echo ""
fi

#!/bin/bash

# Certbot SSL Certificate Initialization Script
# This script obtains SSL certificates for your domains using Let's Encrypt
# and sets up automatic renewal via the certbot container

set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print usage
print_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -e, --email EMAIL          Email address for certificate notifications (required)"
    echo "  -d, --domains DOMAINS      Comma-separated list of domains (default: api.xcentro.in,app.xcentro.in)"
    echo "  -s, --staging              Use Let's Encrypt staging environment (for testing)"
    echo "  -h, --help                 Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 --email admin@xcentro.in"
    echo "  $0 -e admin@xcentro.in -d api.xcentro.in,app.xcentro.in"
    echo "  $0 -e admin@xcentro.in --staging"
    echo "  $0 -e admin@xcentro.in -d newdomain.com,www.newdomain.com -s"
    echo ""
}

# Default configuration
DOMAINS=("api.xcentro.in" "app.xcentro.in")
STAGING=0
EMAIL=""

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -e|--email)
            EMAIL="$2"
            shift 2
            ;;
        -d|--domains)
            IFS=',' read -ra DOMAINS <<< "$2"
            shift 2
            ;;
        -s|--staging)
            STAGING=1
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

echo "=========================================="
echo "Let's Encrypt SSL Certificate Setup"
echo "=========================================="
echo ""

# Validate email is provided
if [ -z "$EMAIL" ]; then
    echo -e "${RED}ERROR: Email address is required${NC}"
    echo ""
    print_usage
    exit 1
fi

# Prompt for confirmation
echo "This script will obtain SSL certificates for:"
for domain in "${DOMAINS[@]}"; do
    echo "  - $domain"
done
echo ""
echo "Email for notifications: $EMAIL"
if [ $STAGING -ne 0 ]; then
    echo -e "${YELLOW}Mode: STAGING (testing mode, certificates will not be trusted)${NC}"
else
    echo -e "${GREEN}Mode: PRODUCTION${NC}"
fi
echo ""
read -p "Continue? (y/n) " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
fi

# Check if certbot container exists (nginx might not be running yet - that's ok)
if ! docker compose -f docker-compose.production.yml ps certbot &>/dev/null; then
    echo -e "${RED}ERROR: Docker compose services not started${NC}"
    echo "Please start the services first:"
    echo "  docker compose -f docker-compose.production.yml up -d"
    exit 1
fi

# Set staging flag
STAGING_ARG=""
if [ $STAGING -ne 0 ]; then
    STAGING_ARG="--staging"
fi

echo ""
echo "=========================================="
echo "Step 1: Creating dummy certificates"
echo "=========================================="
echo "This prevents nginx from crashing before real certs exist"
echo ""

# Create dummy certificates if they don't exist
for domain in "${DOMAINS[@]}"; do
    cert_path="/etc/letsencrypt/live/$domain"

    # Check if certificate already exists
    if docker compose -f docker-compose.production.yml run --rm --entrypoint "" certbot sh -c "test -f '$cert_path/fullchain.pem'" 2>/dev/null; then
        echo -e "${YELLOW}Certificate for $domain already exists, skipping dummy creation${NC}"
    else
        echo -e "${YELLOW}Creating dummy certificate for $domain${NC}"

        # Create directory structure and dummy cert in volume
        docker compose -f docker-compose.production.yml run --rm --entrypoint "" certbot \
            sh -c "mkdir -p /etc/letsencrypt/live/$domain && \
                   openssl req -x509 -nodes -newkey rsa:2048 -days 1 \
                   -keyout /etc/letsencrypt/live/$domain/privkey.pem \
                   -out /etc/letsencrypt/live/$domain/fullchain.pem \
                   -subj '/CN=localhost'"

        echo -e "${GREEN}✓ Dummy certificate created for $domain${NC}"
    fi
done

echo ""
echo "Starting nginx with dummy certificates..."
docker compose -f docker-compose.production.yml up -d nginx
echo "Waiting for nginx to be ready..."
sleep 5

# Verify nginx is running
if ! docker ps | grep -q xcentro-nginx; then
    echo -e "${RED}ERROR: nginx failed to start${NC}"
    echo "Check logs with: docker compose -f docker-compose.production.yml logs nginx"
    exit 1
fi

echo -e "${GREEN}✓ nginx is running${NC}"

echo ""
echo "=========================================="
echo "Step 2: Obtaining real certificates"
echo "=========================================="

# Request certificates for each domain
for domain in "${DOMAINS[@]}"; do
    echo ""
    echo -e "${GREEN}Processing: $domain${NC}"

    # Request certificate (force renewal to replace dummy certs)
    docker compose -f docker-compose.production.yml run --rm --entrypoint "certbot" certbot \
        certonly \
        --force-renewal \
        --webroot \
        --webroot-path=/var/www/certbot \
        --email $EMAIL \
        --agree-tos \
        --no-eff-email \
        $STAGING_ARG \
        -d $domain

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Certificate obtained for $domain${NC}"
    else
        echo -e "${RED}✗ Failed to obtain certificate for $domain${NC}"
        exit 1
    fi
done

echo ""
echo "=========================================="
echo "Step 3: Reloading nginx with real certs"
echo "=========================================="

docker compose -f docker-compose.production.yml exec nginx nginx -s reload

echo ""
echo -e "${GREEN}=========================================="
echo "✓ SSL Certificate setup complete!"
echo "==========================================${NC}"
echo ""
echo "Next steps:"
echo "1. Certificates are stored in the 'certbot_conf' Docker volume"
echo "2. Auto-renewal is handled by the certbot container (runs twice daily)"
echo "3. nginx reloads every 6 hours to pick up renewed certificates"
echo ""
echo "Useful commands:"
echo "  - Check certificate expiry:"
echo "    docker compose -f docker-compose.production.yml run --rm certbot certificates"
echo ""
echo "  - Force renewal:"
echo "    docker compose -f docker-compose.production.yml run --rm certbot renew --force-renewal"
echo ""
echo "  - Test renewal (dry-run):"
echo "    docker compose -f docker-compose.production.yml run --rm certbot renew --dry-run"
echo ""

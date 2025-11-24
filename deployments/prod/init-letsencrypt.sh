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
    echo "  --break-my-certs           Delete existing certificates and request fresh ones"
    echo "  -v, --verbose              Show verbose certbot output"
    echo "  -h, --help                 Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 --email admin@xcentro.in"
    echo "  $0 -e admin@xcentro.in -d api.xcentro.in,app.xcentro.in"
    echo "  $0 -e admin@xcentro.in --staging"
    echo "  $0 -e admin@xcentro.in -d newdomain.com,www.newdomain.com -s"
    echo "  $0 -e admin@xcentro.in --break-my-certs -v  # Fresh start with verbose output"
    echo ""
}

# Default configuration
DOMAINS=("api.xcentro.in" "app.xcentro.in")
STAGING=0
EMAIL=""
BREAK_CERTS=0
VERBOSE=0

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
        --break-my-certs)
            BREAK_CERTS=1
            shift
            ;;
        -v|--verbose)
            VERBOSE=1
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
if ! docker compose -f docker-compose.yml ps certbot &>/dev/null; then
    echo -e "${RED}ERROR: Docker compose services not started${NC}"
    echo "Please start the services first:"
    echo "  docker compose -f docker-compose.yml up -d"
    exit 1
fi

# Set staging flag
STAGING_ARG=""
if [ $STAGING -ne 0 ]; then
    STAGING_ARG="--staging"
fi

# Set verbose flag (certbot accepts -v, -vv, or -vvv for increasing verbosity)
VERBOSE_ARG=""
if [ $VERBOSE -ne 0 ]; then
    VERBOSE_ARG="-vv"
fi

# Delete existing certificates if requested
if [ $BREAK_CERTS -ne 0 ]; then
    echo ""
    echo "=========================================="
    echo "WARNING: Deleting existing certificates"
    echo "=========================================="
    echo ""

    for domain in "${DOMAINS[@]}"; do
        cert_path="/etc/letsencrypt/live/$domain"

        if docker compose -f docker-compose.yml run --rm --entrypoint "" certbot sh -c "test -d '$cert_path'" 2>/dev/null; then
            echo -e "${YELLOW}Deleting existing certificate for $domain${NC}"
            docker compose -f docker-compose.yml run --rm --entrypoint "" certbot \
                sh -c "rm -rf /etc/letsencrypt/live/$domain /etc/letsencrypt/archive/$domain /etc/letsencrypt/renewal/$domain.conf"
            echo -e "${GREEN}✓ Certificate deleted for $domain${NC}"
        else
            echo "No existing certificate found for $domain"
        fi
    done

    echo ""
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
    if docker compose -f docker-compose.yml run --rm --entrypoint "" certbot sh -c "test -f '$cert_path/fullchain.pem'" 2>/dev/null; then
        echo -e "${YELLOW}Certificate for $domain already exists, skipping dummy creation${NC}"
    else
        echo -e "${YELLOW}Creating dummy certificate for $domain${NC}"

        # Create directory structure and dummy cert in volume
        docker compose -f docker-compose.yml run --rm --entrypoint "" certbot \
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
docker compose -f docker-compose.yml up -d nginx
echo "Waiting for nginx to be ready..."
sleep 5

# Verify nginx is running
if ! docker ps | grep -q xcentro-nginx; then
    echo -e "${RED}ERROR: nginx failed to start${NC}"
    echo "Check logs with: docker compose -f docker-compose.yml logs nginx"
    exit 1
fi

echo -e "${GREEN}✓ nginx is running${NC}"

echo ""
echo "=========================================="
echo "Step 2: Obtaining real certificates"
echo "=========================================="

# Verify domain accessibility before requesting certificates
echo ""
echo "Verifying domain accessibility..."
for domain in "${DOMAINS[@]}"; do
    echo -n "Checking $domain on port 80... "
    if curl -s -I -m 5 "http://$domain/.well-known/acme-challenge/" >/dev/null 2>&1 || \
       curl -s -I -m 5 "http://$domain/" >/dev/null 2>&1; then
        echo -e "${GREEN}✓ Accessible${NC}"
    else
        echo -e "${YELLOW}⚠ Warning: Cannot reach $domain${NC}"
        echo "  Make sure:"
        echo "  1. DNS points to this server's IP"
        echo "  2. Port 80 is open in firewall"
        echo "  3. nginx is running and accessible"
    fi
done

echo ""
echo "Removing dummy certificates and old certificate data..."
# Manually delete certificate files (certbot delete doesn't work for openssl-generated dummy certs)
# This is safe to do while nginx is running (certs are loaded in memory)
for domain in "${DOMAINS[@]}"; do
    echo "Cleaning certificate data for $domain..."
    docker compose -f docker-compose.yml run --rm --entrypoint "" certbot \
        sh -c "rm -rf /etc/letsencrypt/live/$domain /etc/letsencrypt/archive/$domain /etc/letsencrypt/renewal/$domain.conf" 2>/dev/null || true
done
echo -e "${GREEN}✓ Old certificate data cleaned${NC}"

# Request certificates for each domain
for domain in "${DOMAINS[@]}"; do
    echo ""
    echo -e "${GREEN}Processing: $domain${NC}"

    # Request certificate fresh (no --force-renewal needed since we deleted old certs)
    docker compose -f docker-compose.yml run --rm --entrypoint "certbot" certbot \
        certonly \
        --webroot \
        --webroot-path=/var/www/certbot \
        --email $EMAIL \
        --agree-tos \
        --no-eff-email \
        --keep-until-expiring \
        $VERBOSE_ARG \
        $STAGING_ARG \
        -d $domain

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Certificate obtained for $domain${NC}"
    else
        echo -e "${RED}✗ Failed to obtain certificate for $domain${NC}"
        echo ""
        echo "Common reasons for failure:"
        echo "  1. Domain doesn't point to this server"
        echo "  2. Port 80 is not accessible from internet"
        echo "  3. Firewall blocking Let's Encrypt servers"
        echo "  4. Rate limit reached (use --staging to test)"
        echo ""
        echo "To debug:"
        echo "  • Test with --staging flag first"
        echo "  • Check nginx logs: docker compose -f docker-compose.yml logs nginx"
        echo "  • Verify DNS: dig $domain"
        echo "  • Test port: curl -I http://$domain"
        exit 1
    fi
done

echo ""
echo "=========================================="
echo "Step 3: Reloading nginx with real certs"
echo "=========================================="

# Reload nginx to pick up new certificates (nginx was running the whole time for ACME challenges)
docker compose -f docker-compose.yml exec nginx nginx -s reload

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ nginx reloaded successfully with real certificates${NC}"
else
    echo -e "${RED}ERROR: nginx reload failed${NC}"
    echo "Check logs with: docker compose -f docker-compose.yml logs nginx"
    exit 1
fi

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
echo "    docker compose -f docker-compose.yml run --rm certbot certificates"
echo ""
echo "  - Force renewal:"
echo "    docker compose -f docker-compose.yml run --rm certbot renew --force-renewal"
echo ""
echo "  - Test renewal (dry-run):"
echo "    docker compose -f docker-compose.yml run --rm certbot renew --dry-run"
echo ""

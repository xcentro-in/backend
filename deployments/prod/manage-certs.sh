#!/bin/bash

# SSL Certificate Management Helper Script
# Provides convenient commands for managing Let's Encrypt certificates

set -e

COMPOSE_FILE="docker-compose.yml"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_usage() {
    echo "SSL Certificate Management Tool"
    echo ""
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  list              List all certificates and their expiry dates"
    echo "  renew [domain]    Renew certificates (all or specific domain)"
    echo "  test              Test renewal process (dry-run, safe)"
    echo "  add <domain>      Obtain certificate for a new domain"
    echo "  delete <domain>   Delete certificate for a domain"
    echo "  logs              View certbot container logs"
    echo "  reload-nginx      Reload nginx to pick up new certificates"
    echo ""
    echo "Examples:"
    echo "  $0 list"
    echo "  $0 renew"
    echo "  $0 renew api.xcentro.in"
    echo "  $0 test"
    echo "  $0 add new.xcentro.in"
    echo ""
}

check_containers() {
    if ! docker compose -f $COMPOSE_FILE ps | grep -q nginx; then
        echo -e "${RED}ERROR: Containers are not running${NC}"
        echo "Start the stack first:"
        echo "  docker compose -f $COMPOSE_FILE up -d"
        exit 1
    fi
}

list_certificates() {
    echo -e "${BLUE}=========================================="
    echo "SSL Certificates Status"
    echo "==========================================${NC}"
    echo ""

    docker compose -f $COMPOSE_FILE run --rm certbot certificates
}

renew_certificates() {
    local domain=$1

    if [ -z "$domain" ]; then
        echo -e "${YELLOW}Renewing all certificates...${NC}"
        docker compose -f $COMPOSE_FILE run --rm certbot renew
    else
        echo -e "${YELLOW}Renewing certificate for: $domain${NC}"
        docker compose -f $COMPOSE_FILE run --rm certbot renew --cert-name $domain
    fi

    if [ $? -eq 0 ]; then
        echo ""
        echo -e "${GREEN}✓ Certificate renewal successful${NC}"
        echo "Reloading nginx..."
        docker compose -f $COMPOSE_FILE exec nginx nginx -s reload
        echo -e "${GREEN}✓ nginx reloaded${NC}"
    else
        echo -e "${RED}✗ Certificate renewal failed${NC}"
        exit 1
    fi
}

test_renewal() {
    echo -e "${YELLOW}Testing certificate renewal (dry-run)...${NC}"
    echo "This is safe and won't make any actual changes"
    echo ""

    docker compose -f $COMPOSE_FILE run --rm certbot renew --dry-run

    if [ $? -eq 0 ]; then
        echo ""
        echo -e "${GREEN}✓ Renewal test successful${NC}"
        echo "Your certificates will auto-renew without issues"
    else
        echo -e "${RED}✗ Renewal test failed${NC}"
        echo "Please check the output above for errors"
        exit 1
    fi
}

add_certificate() {
    local domain=$1

    if [ -z "$domain" ]; then
        echo -e "${RED}ERROR: Domain name is required${NC}"
        echo "Usage: $0 add <domain>"
        exit 1
    fi

    echo -e "${YELLOW}Obtaining certificate for: $domain${NC}"
    echo ""

    read -p "Email for notifications: " email

    if [ -z "$email" ]; then
        echo -e "${RED}ERROR: Email is required${NC}"
        exit 1
    fi

    echo ""
    read -p "Use staging (testing) mode? (y/n) " -n 1 -r
    echo ""

    STAGING_ARG=""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        STAGING_ARG="--staging"
        echo -e "${YELLOW}Using STAGING mode (certificate will not be trusted)${NC}"
    fi

    docker compose -f $COMPOSE_FILE run --rm certbot \
        certonly \
        --webroot \
        --webroot-path=/var/www/certbot \
        --email $email \
        --agree-tos \
        --no-eff-email \
        $STAGING_ARG \
        -d $domain

    if [ $? -eq 0 ]; then
        echo ""
        echo -e "${GREEN}✓ Certificate obtained for $domain${NC}"
        echo "Reloading nginx..."
        docker compose -f $COMPOSE_FILE exec nginx nginx -s reload
        echo -e "${GREEN}✓ nginx reloaded${NC}"
        echo ""
        echo "Don't forget to:"
        echo "1. Add nginx configuration for $domain in nginx/conf.d/"
        echo "2. Update DNS records to point to this server"
    else
        echo -e "${RED}✗ Failed to obtain certificate${NC}"
        exit 1
    fi
}

delete_certificate() {
    local domain=$1

    if [ -z "$domain" ]; then
        echo -e "${RED}ERROR: Domain name is required${NC}"
        echo "Usage: $0 delete <domain>"
        exit 1
    fi

    echo -e "${RED}WARNING: This will delete the certificate for $domain${NC}"
    read -p "Are you sure? (y/n) " -n 1 -r
    echo ""

    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 0
    fi

    docker compose -f $COMPOSE_FILE run --rm certbot delete --cert-name $domain

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Certificate deleted for $domain${NC}"
    else
        echo -e "${RED}✗ Failed to delete certificate${NC}"
        exit 1
    fi
}

show_logs() {
    echo -e "${BLUE}Certbot container logs (last 50 lines):${NC}"
    echo ""
    docker compose -f $COMPOSE_FILE logs --tail=50 certbot
}

reload_nginx() {
    echo -e "${YELLOW}Reloading nginx...${NC}"
    docker compose -f $COMPOSE_FILE exec nginx nginx -s reload

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ nginx reloaded successfully${NC}"
    else
        echo -e "${RED}✗ Failed to reload nginx${NC}"
        exit 1
    fi
}

# Main command router
case "${1:-}" in
    list)
        check_containers
        list_certificates
        ;;
    renew)
        check_containers
        renew_certificates "$2"
        ;;
    test)
        check_containers
        test_renewal
        ;;
    add)
        check_containers
        add_certificate "$2"
        ;;
    delete)
        check_containers
        delete_certificate "$2"
        ;;
    logs)
        show_logs
        ;;
    reload-nginx)
        check_containers
        reload_nginx
        ;;
    help|-h|--help)
        print_usage
        ;;
    *)
        echo -e "${RED}ERROR: Unknown command: ${1:-}${NC}"
        echo ""
        print_usage
        exit 1
        ;;
esac

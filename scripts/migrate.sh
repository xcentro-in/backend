#!/bin/bash

# Migration Helper Script
# This script ensures migrations are run with proper workflow to avoid cached plan errors

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
ACTION="up"
VERSION=""
SKIP_WARNING=false

# Function to print colored output
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to display usage
usage() {
    cat << EOF
Migration Helper Script - Ensures proper migration workflow

Usage: $0 [OPTIONS]

Options:
    -a, --action ACTION     Migration action: up, down, version, goto, force, drop (default: up)
    -v, --version VERSION   Target version (required for goto/force actions)
    -y, --yes               Skip confirmation prompt
    -h, --help              Display this help message

Examples:
    $0 -a up                # Run all pending migrations
    $0 -a down              # Rollback one migration
    $0 -a version           # Check current migration version
    $0 -a goto -v 5         # Migrate to version 5
    $0 -a up -y             # Run migrations without confirmation

Important:
    - This script will check if your API server is running
    - You should stop the API server before running migrations
    - This prevents "cached plan must not change result type" errors
    - After migrations complete, remember to restart your API server

EOF
    exit 0
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -a|--action)
            ACTION="$2"
            shift 2
            ;;
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -y|--yes)
            SKIP_WARNING=true
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            print_error "Unknown option: $1"
            usage
            ;;
    esac
done

# Validate action
case $ACTION in
    up|down|version|goto|force|drop)
        ;;
    *)
        print_error "Invalid action: $ACTION"
        usage
        ;;
esac

# Check if version is required
if [[ "$ACTION" == "goto" || "$ACTION" == "force" ]]; then
    if [[ -z "$VERSION" ]]; then
        print_error "Version is required for action: $ACTION"
        usage
    fi
fi

print_info "Migration Action: $ACTION"

# Check if API server is running
API_RUNNING=false
if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null 2>&1 ; then
    API_RUNNING=true
fi

# If running schema-changing migrations, warn about API server
if [[ "$ACTION" == "up" || "$ACTION" == "down" || "$ACTION" == "goto" || "$ACTION" == "drop" ]]; then
    if $API_RUNNING; then
        print_warning "⚠️  API server appears to be running on port 8080"
        print_warning "Running migrations with the API server running may cause:"
        print_warning "  - Cached plan errors: 'cached plan must not change result type'"
        print_warning "  - Database connection issues"
        print_warning "  - Unexpected application behavior"
        echo ""
        print_info "Recommendation: Stop the API server before running migrations"
        echo ""
        
        if ! $SKIP_WARNING; then
            read -p "Do you want to continue anyway? (yes/no): " -r
            echo
            if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ && ! $REPLY =~ ^[Yy]$ ]]; then
                print_info "Migration cancelled. Please stop the API server and try again."
                exit 0
            fi
        fi
    else
        print_success "✓ API server is not running (good!)"
    fi
fi

# Run migration
print_info "Running migration command..."
echo ""

if [[ -z "$VERSION" ]]; then
    go run cmd/migrate/main.go -action="$ACTION"
else
    go run cmd/migrate/main.go -action="$ACTION" -version="$VERSION"
fi

echo ""

# Post-migration instructions
if [[ "$ACTION" == "up" || "$ACTION" == "down" || "$ACTION" == "goto" || "$ACTION" == "drop" ]]; then
    print_success "Migration completed successfully!"
    echo ""
    print_info "Next steps:"
    if $API_RUNNING; then
        print_warning "1. Restart your API server to clear cached prepared statements"
        print_info "2. Test your application to ensure everything works correctly"
    else
        print_info "1. Start your API server: go run cmd/api/http_server.go"
        print_info "2. Test your application to ensure everything works correctly"
    fi
fi

exit 0


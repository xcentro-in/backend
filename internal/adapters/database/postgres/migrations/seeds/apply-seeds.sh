#!/bin/bash

# #############################################################################
# # Database Seed Helper Script (Docker-based)
# # Purpose: Easy apply/rollback of seed files using Docker
# #############################################################################

set -e  # Exit on error

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values - matching your docker setup
CONTAINER_NAME="${CONTAINER_NAME:-xcentro-postgres}"
DB_NAME="${DB_NAME:-xcentro}"
DB_USER="${DB_USER:-xcentro_rw}"
SEED_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Function to print colored messages
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Function to check if Docker is available
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker command not found. Please install Docker."
        exit 1
    fi
}

# Function to check if container is running
check_container() {
    print_step "Checking if $CONTAINER_NAME container is running..."
    if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_error "Container '$CONTAINER_NAME' is not running"
        print_info "Start it with: cd deployments/docker && ./start.sh"
        exit 1
    fi
    print_info "✓ Container is running"
}

# Function to check database connection
check_connection() {
    print_step "Checking database connection..."
    if docker exec "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1;" > /dev/null 2>&1; then
        print_info "✓ Database connection successful"
    else
        print_error "✗ Cannot connect to database"
        print_error "Container: $CONTAINER_NAME, Database: $DB_NAME, User: $DB_USER"
        exit 1
    fi
}

# Function to apply seed file
apply_seed() {
    local seed_file="$1"
    local full_path="$SEED_DIR/$seed_file"

    print_step "Applying seed file: $seed_file"

    if [ ! -f "$full_path" ]; then
        print_error "Seed file not found: $full_path"
        exit 1
    fi

    # Use docker exec with stdin piping to avoid path issues
    cat "$full_path" | docker exec -i "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME"

    if [ $? -eq 0 ]; then
        print_info "✓ Seed file applied successfully"
    else
        print_error "✗ Failed to apply seed file"
        exit 1
    fi
}

# Function to rollback seed file
rollback_seed() {
    local rollback_file="$1"
    local full_path="$SEED_DIR/$rollback_file"

    print_step "Rolling back seed file: $rollback_file"

    if [ ! -f "$full_path" ]; then
        print_error "Rollback file not found: $full_path"
        exit 1
    fi

    # Use docker exec with stdin piping to avoid path issues
    cat "$full_path" | docker exec -i "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME"

    if [ $? -eq 0 ]; then
        print_info "✓ Seed file rolled back successfully"
    else
        print_error "✗ Failed to rollback seed file"
        exit 1
    fi
}

# Function to show usage
show_usage() {
    cat << EOF
${GREEN}Database Seed Helper Script${NC}

${BLUE}USAGE:${NC}
    $0 [OPTIONS] COMMAND

${BLUE}COMMANDS:${NC}
    apply <seed_number>     Apply a seed file (e.g., apply 001)
    rollback <seed_number>  Rollback a seed file (e.g., rollback 001)
    apply-all               Apply all seed files
    rollback-all            Rollback all seed files
    list                    List available seed files
    help                    Show this help message

${BLUE}OPTIONS:${NC}
    --container NAME        Docker container name (default: xcentro-postgres)
    --db DATABASE           Database name (default: xcentro)
    --user USER             Database user (default: xcentro_admin)

${BLUE}EXAMPLES:${NC}
    # Apply wallet test data
    $0 apply 001

    # Rollback wallet test data
    $0 rollback 001

    # Apply all seed files
    $0 apply-all

    # List available seeds
    $0 list

    # Use custom container name
    $0 --container my-postgres apply 001

${BLUE}ENVIRONMENT VARIABLES:${NC}
    CONTAINER_NAME, DB_NAME, DB_USER can be set instead of using flags

${BLUE}AVAILABLE SEED FILES:${NC}
    001 - Wallet test data (business, stored cards, bank accounts)

${BLUE}NOTES:${NC}
    - This script uses Docker exec to run psql inside the container
    - No local psql installation required
    - Files are piped via stdin to avoid path issues
    - Default user is 'xcentro_admin' (has DDL permissions for seeding)

EOF
}

# Function to list seed files
list_seeds() {
    print_info "Seed files in $SEED_DIR:"
    echo ""

    echo -e "${BLUE}Seed files:${NC}"
    local found_seed=false
    for file in "$SEED_DIR"/[0-9][0-9][0-9]_seed_*.sql; do
        if [ -f "$file" ] && [[ ! "$file" =~ _rollback\.sql$ ]]; then
            echo "  $(basename "$file")"
            found_seed=true
        fi
    done

    if [ "$found_seed" = false ]; then
        echo "  (none)"
    fi

    echo ""
    echo -e "${BLUE}Rollback files:${NC}"
    local found_rollback=false
    for file in "$SEED_DIR"/[0-9][0-9][0-9]_seed_*_rollback.sql; do
        if [ -f "$file" ]; then
            echo "  $(basename "$file")"
            found_rollback=true
        fi
    done

    if [ "$found_rollback" = false ]; then
        echo "  (none)"
    fi

    echo ""
}

# Parse command line arguments
COMMAND=""
SEED_NUMBER=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --container)
            CONTAINER_NAME="$2"
            shift 2
            ;;
        --db)
            DB_NAME="$2"
            shift 2
            ;;
        --user)
            DB_USER="$2"
            shift 2
            ;;
        apply|rollback|apply-all|rollback-all|list|help)
            COMMAND="$1"
            shift
            if [ "$COMMAND" == "apply" ] || [ "$COMMAND" == "rollback" ]; then
                if [ -n "$1" ] && [[ "$1" =~ ^[0-9]+$ ]]; then
                    SEED_NUMBER="$1"
                    shift
                fi
            fi
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Main execution
case "$COMMAND" in
    help)
        show_usage
        exit 0
        ;;
    list)
        list_seeds
        exit 0
        ;;
    apply)
        if [ -z "$SEED_NUMBER" ]; then
            print_error "Seed number required. Example: apply 001"
            exit 1
        fi
        check_docker
        check_container
        check_connection
        apply_seed "${SEED_NUMBER}_seed_wallet_test_data.sql"
        print_info "✓ Done! Use this business_id in Postman: 01936d7a-7c8e-7890-b123-456789abcdef"
        ;;
    rollback)
        if [ -z "$SEED_NUMBER" ]; then
            print_error "Seed number required. Example: rollback 001"
            exit 1
        fi
        check_docker
        check_container
        check_connection
        rollback_seed "${SEED_NUMBER}_seed_wallet_test_data_rollback.sql"
        ;;
    apply-all)
        check_docker
        check_container
        check_connection
        print_info "Applying all seed files..."
        local count=0
        for file in "$SEED_DIR"/[0-9][0-9][0-9]_seed_*.sql; do
            if [ -f "$file" ] && [[ ! "$file" =~ _rollback\.sql$ ]]; then
                apply_seed "$(basename "$file")"
                ((count++))
            fi
        done
        if [ $count -eq 0 ]; then
            print_warning "No seed files found"
        else
            print_info "✓ Applied $count seed file(s)"
        fi
        ;;
    rollback-all)
        check_docker
        check_container
        check_connection
        print_warning "This will rollback ALL seed files. Continue? (y/N)"
        read -r response
        if [[ ! "$response" =~ ^[Yy]$ ]]; then
            print_info "Cancelled."
            exit 0
        fi
        print_info "Rolling back all seed files..."
        local count=0
        for file in "$SEED_DIR"/[0-9][0-9][0-9]_seed_*_rollback.sql; do
            if [ -f "$file" ]; then
                rollback_seed "$(basename "$file")"
                ((count++))
            fi
        done
        if [ $count -eq 0 ]; then
            print_warning "No rollback files found"
        else
            print_info "✓ Rolled back $count seed file(s)"
        fi
        ;;
    "")
        print_error "No command specified. Use 'help' for usage information."
        exit 1
        ;;
    *)
        print_error "Invalid command: $COMMAND"
        print_info "Use 'help' for usage information."
        exit 1
        ;;
esac

exit 0

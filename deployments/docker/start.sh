#!/usr/bin/env bash
set -euo pipefail

# XCentro Database Stack Startup Script
# This script brings up PostgreSQL and Redis containers

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "$SCRIPT_DIR/utils.sh"

ENV_FILE="$SCRIPT_DIR/.env"

check_dependencies() {
    print_info "Checking dependencies..."
    
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed or not in PATH"
        exit 1
    fi
    
    if ! docker compose version &> /dev/null; then
        print_error "Docker Compose is not installed or not available"
        print_info "Please ensure you have Docker with Compose plugin installed"
        exit 1
    fi
    
    print_success "Dependencies check passed"
}

setup_env_file() {
    if [ ! -f "$ENV_FILE" ]; then
        print_warning "Environment file not found. Creating from example..."
        cp "$SCRIPT_DIR/.env.example" "$ENV_FILE"
        print_success "Created .env file from .env.example"
        print_warning "Please review and update passwords in .env file if needed"
    else
        print_success "Environment file exists"
    fi
}

start_services() {
    print_info "Starting database services..."
    
    # Use docker compose (modern Docker CLI)
    COMPOSE_CMD="docker compose"
    
    # Start services in detached mode
    cd "$SCRIPT_DIR"
    $COMPOSE_CMD up -d
    
    print_info "Waiting for services to be healthy..."
    
    # Wait for services to be healthy
    local max_attempts=30
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        local postgres_health=$($COMPOSE_CMD ps postgres --format "{{.Health}}" 2>/dev/null || echo "")
        local redis_health=$($COMPOSE_CMD ps redis --format "{{.Health}}" 2>/dev/null || echo "")
        
        if [[ "$postgres_health" == "healthy" && "$redis_health" == "healthy" ]]; then
            print_success "All services are healthy!"
            break
        fi
        
        attempt=$((attempt + 1))
        if [ $attempt -eq $max_attempts ]; then
            print_error "Services did not become healthy within expected time"
            print_info "Checking service status..."
            $COMPOSE_CMD ps
            exit 1
        fi
        
        printf "."
        sleep 2
    done
}

show_connection_info() {
    print_info "Loading environment variables..."
    source "$ENV_FILE"
    
    echo
    print_info "🗄️  Database Services Status:"
    cd "$SCRIPT_DIR"
    
    # Use docker compose for status
    docker compose ps
    
    echo
    print_info "📋 Connection Information:"
    echo -e "${GREEN}PostgreSQL:${NC}"
    echo "  • Host: localhost"
    echo "  • Port: ${POSTGRES_PORT:-5432}"
    echo "  • Database: ${POSTGRES_DB:-xcentro}"
    echo "  • Superuser: ${POSTGRES_SUPERUSER:-postgres}"
    echo
    echo -e "${GREEN}Redis:${NC}"
    echo "  • Host: localhost" 
    echo "  • Port: ${REDIS_PORT:-6379}"
    echo "  • Superuser Password: Set in .env file"
    echo
    print_info "🔧 Next Steps:"
    echo "  1. Set up PostgreSQL users: ./setup-postgres-users.sh"
    echo "  2. Set up Redis users: ./setup-redis-users.sh"
    echo "  3. Connect to PostgreSQL: ./connect-postgres.sh [user]"
    echo "  4. Connect to Redis: ./connect-redis.sh [user]"
    echo
    print_info "🛠️  Management Commands:"
    echo "  • Stop services: docker compose down"
    echo "  • Reset data: docker compose down -v && ./start.sh"
    echo "  • View logs: docker compose logs [service]"
}

main() {
    print_header
    
    check_dependencies
    setup_env_file
    start_services
    show_connection_info
    
    echo
    print_success "XCentro database stack is ready!"
    print_warning "Don't forget to run the user setup scripts!"
}

# Handle command line arguments
case "${1:-}" in
    "help"|"-h"|"--help")
        echo "Usage: $0 [help]"
        echo
        echo "This script starts the XCentro database stack (PostgreSQL + Redis)"
        echo "with basic superuser configuration."
        echo
        echo "Commands:"
        echo "  help    Show this help message"
        echo
        echo "The script will:"
        echo "  1. Check for required dependencies"
        echo "  2. Create .env file if missing"
        echo "  3. Start PostgreSQL and Redis containers"
        echo "  4. Wait for services to be healthy"
        echo "  5. Display connection information"
        echo
        echo "After running this script, you should run:"
        echo "  • ./setup-postgres-users.sh"
        echo "  • ./setup-redis-users.sh"
        exit 0
        ;;
    "")
        main
        ;;
    *)
        print_error "Unknown option: $1"
        echo "Use '$0 help' for usage information"
        exit 1
        ;;
esac
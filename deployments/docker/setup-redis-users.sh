#!/usr/bin/env bash
set -euo pipefail

# XCentro Redis User Setup Script
# This script connects to the Docker Redis instance and creates the required users

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "$SCRIPT_DIR/utils.sh"

ENV_FILE="$SCRIPT_DIR/.env"

# Load environment variables
if [ ! -f "$ENV_FILE" ]; then
    print_error "Environment file not found: $ENV_FILE"
    print_info "Please run ./start.sh first to create the environment file"
    exit 1
fi

source "$ENV_FILE"

# Check if Redis container is running
check_redis_running() {
    if ! docker ps --format "table {{.Names}}\t{{.Status}}" | grep -q "xcentro-redis.*Up"; then
        print_error "Redis container is not running"
        print_info "Start the services first with: ./start.sh"
        exit 1
    fi
}

# Create Redis users using ACL commands
create_redis_users() {
    print_info "Setting up Redis users..."
    
    # Create superuser (dedicated admin user separate from disabled 'default')
    print_info "Creating Redis superuser..."
    docker exec xcentro-redis redis-cli --no-auth-warning -a "$REDIS_SUPERUSER_PASSWORD" \
        ACL SETUSER "$REDIS_SUPERUSER" on ">"$REDIS_SUPERUSER_PASSWORD"" +@all ~* || {
        print_error "Failed to create Redis superuser"
        return 1
    }
    print_success "Redis superuser '$REDIS_SUPERUSER' created"
    
    # Create application user with restricted access
    print_info "Creating Redis application user..."
    docker exec xcentro-redis redis-cli --no-auth-warning -a "$REDIS_SUPERUSER_PASSWORD" \
        ACL SETUSER "$REDIS_APP_USER" on ">"$REDIS_APP_PASSWORD"" +@all -@dangerous ~xcentro:* || {
        print_error "Failed to create Redis application user"
        return 1
    }
    print_success "Redis application user '$REDIS_APP_USER' created with access to 'xcentro:*' keys"
    
    # Disable the default user for security
    print_info "Disabling default Redis user for security..."
    docker exec xcentro-redis redis-cli --no-auth-warning -a "$REDIS_SUPERUSER_PASSWORD" \
        ACL SETUSER default off || {
        print_warning "Failed to disable default user, but continuing..."
    }
    
    print_success "Redis user setup completed"
}

# Test user connections
test_user_connections() {
    print_info "Testing Redis user connections..."
    
    # Test superuser
    if docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$REDIS_SUPERUSER:$REDIS_SUPERUSER_PASSWORD@localhost:6379" ping >/dev/null 2>&1; then
        print_success "Superuser connection test passed"
    else
        print_error "Superuser connection test failed"
        return 1
    fi
    
    # Test application user
    if docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$REDIS_APP_USER:$REDIS_APP_PASSWORD@localhost:6379" ping >/dev/null 2>&1; then
        print_success "Application user connection test passed"
    else
        print_error "Application user connection test failed"
        return 1
    fi
    
    # Test application user key restrictions
    print_info "Testing application user key restrictions..."
    
    # Should be able to set keys with xcentro: prefix
    if docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$REDIS_APP_USER:$REDIS_APP_PASSWORD@localhost:6379" \
        set "xcentro:test" "hello" >/dev/null 2>&1; then
        print_success "Application user can set 'xcentro:*' keys"
    else
        print_error "Application user cannot set 'xcentro:*' keys"
        return 1
    fi
    
    # Retrieve the test key
    local test_value=$(docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$REDIS_APP_USER:$REDIS_APP_PASSWORD@localhost:6379" \
        get "xcentro:test" 2>/dev/null | tr -d '\r')
    if [[ "$test_value" == "hello" ]]; then
        print_success "Application user can retrieve 'xcentro:*' keys"
    else
        print_warning "Application user key retrieval issue"
    fi
    
    # Clean up test key
    docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$REDIS_SUPERUSER:$REDIS_SUPERUSER_PASSWORD@localhost:6379" \
        del "xcentro:test" >/dev/null 2>&1
    
    print_success "All Redis user tests passed"
}

# Display user information
show_user_info() {
    print_info "Redis User Setup Complete!"
    echo
    print_info "Created users:"
    echo -e "${GREEN}• Superuser (${REDIS_SUPERUSER}):${NC} Full access to all Redis commands and keys"
    echo -e "${GREEN}• Application user (${REDIS_APP_USER}):${NC} Read-write access to 'xcentro:*' keys only"
    echo
    print_info "Connection examples:"
    echo "  • Connect as superuser: ./connect-redis.sh super"
    echo "  • Connect as app user: ./connect-redis.sh app"
    echo
    print_info "Current Redis ACL users:"
    # Need to specify the superuser explicitly because the 'default' user is disabled.
    if ! docker exec xcentro-redis redis-cli --no-auth-warning --user "$REDIS_SUPERUSER" -a "$REDIS_SUPERUSER_PASSWORD" ACL LIST; then
        print_warning "Could not list ACL users (authentication failed). Verify superuser credentials."
    fi
}

main() {
    print_header
    
    print_info "Setting up Redis users for XCentro application..."
    
    check_redis_running
    create_redis_users
    test_user_connections
    show_user_info
    
    echo
    print_success "Redis user setup completed!"
}

# Handle command line arguments
case "${1:-}" in
    "help"|"-h"|"--help")
        echo "Usage: $0 [help]"
        echo
        echo "This script creates the required Redis users for the XCentro application:"
        echo "  • Superuser: Full access to all Redis commands and keys"
        echo "  • Application user: Read-write access to 'xcentro:*' keys only"
        echo
        echo "Prerequisites:"
        echo "  • Redis container must be running (./start.sh)"
        echo "  • .env file must exist with user credentials"
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
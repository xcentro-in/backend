#!/usr/bin/env bash
set -euo pipefail

# XCentro Redis Connection Script
# This script provides easy access to Redis with different user roles

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "$SCRIPT_DIR/utils.sh"

ENV_FILE="$SCRIPT_DIR/.env"

check_services() {
    if ! docker ps --format "table {{.Names}}\t{{.Status}}" | grep -q "xcentro-redis.*Up"; then
        print_error "Redis container is not running"
        print_info "Start the services first with: ./start.sh"
        exit 1
    fi
}

load_env() {
    if [ ! -f "$ENV_FILE" ]; then
        print_error "Environment file not found: $ENV_FILE"
        print_info "Please run ./start.sh first"
        exit 1
    fi
    source "$ENV_FILE"
}

show_help() {
    echo "Usage: $0 [USER] [OPTIONS]"
    echo
    echo "Connect to Redis with different user roles:"
    echo
    echo "USERS:"
    echo "  super, superuser       - Connect as Redis superuser (full access)"
    echo "  app, rw               - Connect as application user (limited to xcentro:* keys)"
    echo
    echo "OPTIONS:"
    echo "  -c, --command COMMAND  - Execute a single Redis command"
    echo "  -i, --info             - Show Redis server information"
    echo "  -u, --users            - List Redis ACL users"
    echo "  -p, --ping             - Test connection with PING"
    echo "  -t, --test             - Run connection tests for all users"
    echo "  -h, --help             - Show this help message"
    echo
    echo "EXAMPLES:"
    echo "  $0                     - Interactive shell as superuser (default)"
    echo "  $0 app                 - Interactive shell as app user"
    echo "  $0 super -c 'INFO'     - Show Redis info using superuser"
    echo "  $0 app -c 'SET xcentro:test hello' - Set a key using app user"
    echo "  $0 -p                  - Test connection (ping) for all users"
    echo "  $0 -t                  - Run comprehensive connection tests"
}

test_redis_connection() {
    local user="$1"
    local password="$2"
    local description="$3"
    
    print_info "Testing $description connection..."
    
    if docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$user:$password@localhost:6379" ping >/dev/null 2>&1; then
        print_success "$description connection successful"
        return 0
    else
        print_error "$description connection failed"
        return 1
    fi
}

run_connection_tests() {
    print_info "Running Redis connection tests..."
    echo
    
    # Test superuser
    if test_redis_connection "$REDIS_SUPERUSER" "$REDIS_SUPERUSER_PASSWORD" "Superuser"; then
        # Test a command
        docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$REDIS_SUPERUSER:$REDIS_SUPERUSER_PASSWORD@localhost:6379" \
            set "test:superuser" "ok" >/dev/null
        print_success "Superuser can execute all commands"
        docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$REDIS_SUPERUSER:$REDIS_SUPERUSER_PASSWORD@localhost:6379" \
            del "test:superuser" >/dev/null
    fi
    
    # Test app user  
    if test_redis_connection "$REDIS_APP_USER" "$REDIS_APP_PASSWORD" "Application user"; then
        # Test setting a key in allowed pattern
        if docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$REDIS_APP_USER:$REDIS_APP_PASSWORD@localhost:6379" \
            SET "xcentro:test" "hello" >/dev/null 2>&1; then
            print_success "Application user can set keys in xcentro:* pattern"
        else
            print_error "Application user cannot set keys in xcentro:* pattern"
        fi
        
        # Test getting the key back
        local value=$(docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$REDIS_APP_USER:$REDIS_APP_PASSWORD@localhost:6379" \
            GET "xcentro:test" 2>/dev/null | tr -d '\r')
        if [[ "$value" == "hello" ]]; then
            print_success "Application user can retrieve keys: xcentro:test = $value"
        else
            print_warning "Application user key retrieval issue"
        fi
        
        # Clean up test key
        docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$REDIS_SUPERUSER:$REDIS_SUPERUSER_PASSWORD@localhost:6379" \
            DEL "xcentro:test" >/dev/null 2>&1
    fi
    
    echo
    print_success "Connection tests completed"
}

show_redis_info() {
    print_info "Redis Server Information:"
    docker exec xcentro-redis redis-cli --no-auth-warning -a "$REDIS_SUPERUSER_PASSWORD" INFO server
}

list_redis_users() {
    print_info "Redis ACL Users:"
    docker exec xcentro-redis redis-cli --no-auth-warning -a "$REDIS_SUPERUSER_PASSWORD" ACL LIST
}

ping_test() {
    print_info "Testing Redis connections..."
    echo
    
    test_redis_connection "$REDIS_SUPERUSER" "$REDIS_SUPERUSER_PASSWORD" "Superuser"
    test_redis_connection "$REDIS_APP_USER" "$REDIS_APP_PASSWORD" "Application user"
}

connect_as_user() {
    local user_type="$1"
    local username=""
    local password=""
    local description=""
    
    case "$user_type" in
        "super"|"superuser"|"")
            username="$REDIS_SUPERUSER"
            password="$REDIS_SUPERUSER_PASSWORD"
            description="superuser"
            ;;
        "app"|"rw")
            username="$REDIS_APP_USER"
            password="$REDIS_APP_PASSWORD"
            description="application user"
            ;;
        *)
            print_error "Unknown user type: $user_type"
            echo "Use '$0 help' for available options"
            exit 1
            ;;
    esac
    
    print_info "Connecting to Redis as $description ($username)..."
    print_info "Type 'exit' or press Ctrl+C to quit"
    echo
    
    # Connect to Redis
    docker exec -it xcentro-redis redis-cli --no-auth-warning -u "redis://$username:$password@localhost:6379"
}

execute_command() {
    local user_type="$1"
    local command="$2"
    local username=""
    local password=""
    
    case "$user_type" in
        "super"|"superuser"|"")
            username="$REDIS_SUPERUSER"
            password="$REDIS_SUPERUSER_PASSWORD"
            ;;
        "app"|"rw")
            username="$REDIS_APP_USER"
            password="$REDIS_APP_PASSWORD"
            ;;
        *)
            print_error "Unknown user type: $user_type"
            exit 1
            ;;
    esac
    
    docker exec xcentro-redis redis-cli --no-auth-warning -u "redis://$username:$password@localhost:6379" $command
}

main() {
    check_services
    load_env
    
    local user_type=""
    local command=""
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -c|--command)
                command="$2"
                shift 2
                ;;
            -i|--info)
                show_redis_info
                exit 0
                ;;
            -u|--users)
                list_redis_users
                exit 0
                ;;
            -p|--ping)
                ping_test
                exit 0
                ;;
            -t|--test)
                run_connection_tests
                exit 0
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            super|superuser|app|rw)
                user_type="$1"
                shift
                ;;
            -*)
                print_error "Unknown option: $1"
                echo "Use '$0 help' for usage information"
                exit 1
                ;;
            *)
                if [ -z "$user_type" ]; then
                    user_type="$1"
                else
                    print_error "Unexpected argument: $1"
                    exit 1
                fi
                shift
                ;;
        esac
    done
    
    # Execute appropriate action
    if [ -n "$command" ]; then
        execute_command "$user_type" "$command"
    else
        connect_as_user "$user_type"
    fi
}

main "$@"
#!/usr/bin/env bash
set -euo pipefail

# XCentro PostgreSQL Connection Script
# This script provides easy access to PostgreSQL with different user roles

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
source "$SCRIPT_DIR/utils.sh"

ENV_FILE="$SCRIPT_DIR/.env"

check_services() {
    if ! docker ps --format "table {{.Names}}\t{{.Status}}" | grep -q "xcentro-postgres.*Up"; then
        print_error "PostgreSQL container is not running"
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
    echo "Connect to PostgreSQL with different user roles:"
    echo
    echo "USERS:"
    echo "  superuser, super, su    - Connect as PostgreSQL superuser (full admin access)"
    echo "  owner, own             - Connect as database owner (${DB_OWNER_USER:-xcentro_owner})"
    echo "  admin, adm             - Connect as admin user (${DB_ADMIN_USER:-xcentro_admin}) - for migrations"
    echo "  rw, app                - Connect as read-write user (${DB_RW_USER:-xcentro_rw}) - for application"
    echo
    echo "OPTIONS:"
    echo "  -c, --command COMMAND   - Execute a single SQL command"
    echo "  -f, --file FILE        - Execute SQL from a file"
    echo "  -l, --list-users       - List all database users"
    echo "  -h, --help             - Show this help message"
    echo
    echo "EXAMPLES:"
    echo "  $0                     - Interactive shell as superuser (default)"
    echo "  $0 admin               - Interactive shell as admin user"
    echo "  $0 rw                  - Interactive shell as read-write user"
    echo "  $0 admin -c '\\du'       - List users using admin user"
    echo "  $0 owner -f schema.sql - Execute schema.sql as owner"
    echo "  $0 -l                  - List all database users"
}

list_users() {
    print_info "Database users in ${POSTGRES_DB}:"
    docker exec xcentro-postgres psql -U "$POSTGRES_SUPERUSER" -d "$POSTGRES_DB" -c "
SELECT 
    rolname as \"User\",
    CASE 
        WHEN rolsuper THEN 'Superuser'
        WHEN rolname = '$DB_OWNER_USER' THEN 'Owner'
        WHEN rolname = '$DB_ADMIN_USER' THEN 'Admin'
        WHEN rolname = '$DB_RW_USER' THEN 'Read-Write'
        ELSE 'Other'
    END as \"Role Type\",
    CASE WHEN rolcanlogin THEN 'Yes' ELSE 'No' END as \"Can Login\"
FROM pg_roles 
WHERE rolname IN ('$POSTGRES_SUPERUSER', '$DB_OWNER_USER', '$DB_ADMIN_USER', '$DB_RW_USER')
ORDER BY rolname;"
}

connect_as_user() {
    local user_type="$1"
    local username=""
    local password=""
    local description=""
    
    case "$user_type" in
        "superuser"|"super"|"su"|"")
            username="$POSTGRES_SUPERUSER"
            password="$POSTGRES_SUPERUSER_PASSWORD"
            description="superuser"
            ;;
        "owner"|"own")
            username="$DB_OWNER_USER"
            password="$DB_OWNER_PASSWORD"
            description="database owner"
            ;;
        "admin"|"adm")
            username="$DB_ADMIN_USER"
            password="$DB_ADMIN_PASSWORD"
            description="admin user"
            ;;
        "rw"|"app")
            username="$DB_RW_USER"
            password="$DB_RW_PASSWORD"
            description="read-write user"
            ;;
        *)
            print_error "Unknown user type: $user_type"
            echo "Use '$0 help' for available options"
            exit 1
            ;;
    esac
    
    print_info "Connecting to PostgreSQL as $description ($username)..."
    
    # Set password for psql
    export PGPASSWORD="$password"
    
    # Connect to PostgreSQL
    docker exec -it -e PGPASSWORD="$password" xcentro-postgres \
        psql -U "$username" -d "$POSTGRES_DB"
}

execute_command() {
    local user_type="$1"
    local command="$2"
    local username=""
    local password=""
    
    case "$user_type" in
        "superuser"|"super"|"su"|"")
            username="$POSTGRES_SUPERUSER"
            password="$POSTGRES_SUPERUSER_PASSWORD"
            ;;
        "owner"|"own")
            username="$DB_OWNER_USER"
            password="$DB_OWNER_PASSWORD"
            ;;
        "admin"|"adm")
            username="$DB_ADMIN_USER"
            password="$DB_ADMIN_PASSWORD"
            ;;
        "rw"|"app")
            username="$DB_RW_USER"
            password="$DB_RW_PASSWORD"
            ;;
        *)
            print_error "Unknown user type: $user_type"
            exit 1
            ;;
    esac
    
    docker exec -e PGPASSWORD="$password" xcentro-postgres \
        psql -U "$username" -d "$POSTGRES_DB" -c "$command"
}

execute_file() {
    local user_type="$1"
    local file="$2"
    local username=""
    local password=""
    
    if [ ! -f "$file" ]; then
        print_error "File not found: $file"
        exit 1
    fi
    
    case "$user_type" in
        "superuser"|"super"|"su"|"")
            username="$POSTGRES_SUPERUSER"
            password="$POSTGRES_SUPERUSER_PASSWORD"
            ;;
        "owner"|"own")
            username="$DB_OWNER_USER"
            password="$DB_OWNER_PASSWORD"
            ;;
        "admin"|"adm")
            username="$DB_ADMIN_USER"
            password="$DB_ADMIN_PASSWORD"
            ;;
        "rw"|"app")
            username="$DB_RW_USER"
            password="$DB_RW_PASSWORD"
            ;;
        *)
            print_error "Unknown user type: $user_type"
            exit 1
            ;;
    esac
    
    print_info "Executing file $file as $username..."
    docker exec -i -e PGPASSWORD="$password" xcentro-postgres \
        psql -U "$username" -d "$POSTGRES_DB" < "$file"
}

main() {
    check_services
    load_env
    
    local user_type=""
    local command=""
    local file=""
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -c|--command)
                command="$2"
                shift 2
                ;;
            -f|--file)
                file="$2"
                shift 2
                ;;
            -l|--list-users)
                list_users
                exit 0
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            superuser|super|su|owner|own|admin|adm|rw|app)
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
    elif [ -n "$file" ]; then
        execute_file "$user_type" "$file"
    else
        connect_as_user "$user_type"
    fi
}

main "$@"
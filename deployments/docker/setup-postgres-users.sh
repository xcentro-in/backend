#!/usr/bin/env bash
set -euo pipefail

# XCentro PostgreSQL User Setup Script
# This script connects to the Docker PostgreSQL instance and creates the required users

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

# Check if PostgreSQL container is running
check_postgres_running() {
    if ! docker ps --format "table {{.Names}}\t{{.Status}}" | grep -q "xcentro-postgres.*Up"; then
        print_error "PostgreSQL container is not running"
        print_info "Start the services first with: ./start.sh"
        exit 1
    fi
}

# Create database users
create_users() {
    print_info "Creating PostgreSQL users..."
    
    # Create the SQL commands for user creation
    local sql_commands=$(cat <<SQL
-- Create owner user (full database permissions)
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${DB_OWNER_USER}') THEN
        CREATE ROLE ${DB_OWNER_USER} WITH LOGIN PASSWORD '${DB_OWNER_PASSWORD}' INHERIT;
        RAISE NOTICE 'Created role ${DB_OWNER_USER}';
    ELSE
        RAISE NOTICE 'Role ${DB_OWNER_USER} already exists';
    END IF;
END\$\$;

-- Create admin user (for migrations - can create/drop tables, indexes, etc.)
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${DB_ADMIN_USER}') THEN
        CREATE ROLE ${DB_ADMIN_USER} WITH LOGIN PASSWORD '${DB_ADMIN_PASSWORD}' INHERIT;
        RAISE NOTICE 'Created role ${DB_ADMIN_USER}';
    ELSE
        RAISE NOTICE 'Role ${DB_ADMIN_USER} already exists';
    END IF;
END\$\$;

-- Create read-write user (for application - standard CRUD operations)
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${DB_RW_USER}') THEN
        CREATE ROLE ${DB_RW_USER} WITH LOGIN PASSWORD '${DB_RW_PASSWORD}' INHERIT;
        RAISE NOTICE 'Created role ${DB_RW_USER}';
    ELSE
        RAISE NOTICE 'Role ${DB_RW_USER} already exists';
    END IF;
END\$\$;

-- Grant database ownership to owner user
ALTER DATABASE ${POSTGRES_DB} OWNER TO ${DB_OWNER_USER};

-- Grant permissions to owner user (full access)
GRANT ALL PRIVILEGES ON DATABASE ${POSTGRES_DB} TO ${DB_OWNER_USER};

-- Grant permissions to admin user (for migrations)
GRANT CONNECT, TEMP, CREATE ON DATABASE ${POSTGRES_DB} TO ${DB_ADMIN_USER};
GRANT USAGE, CREATE ON SCHEMA public TO ${DB_ADMIN_USER};
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO ${DB_ADMIN_USER};
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ${DB_ADMIN_USER};
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO ${DB_ADMIN_USER};

-- Grant permissions to read-write user (for application)
GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO ${DB_RW_USER};
GRANT USAGE ON SCHEMA public TO ${DB_RW_USER};
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ${DB_RW_USER};
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO ${DB_RW_USER};
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO ${DB_RW_USER};

-- Set up default privileges for future objects created by owner
ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_OWNER_USER} IN SCHEMA public
    GRANT ALL PRIVILEGES ON TABLES TO ${DB_ADMIN_USER};
ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_OWNER_USER} IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ${DB_RW_USER};
ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_OWNER_USER} IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO ${DB_ADMIN_USER};
ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_OWNER_USER} IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO ${DB_RW_USER};

-- Set up default privileges for future objects created by admin
ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_ADMIN_USER} IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ${DB_RW_USER};
ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_ADMIN_USER} IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO ${DB_RW_USER};

-- User setup completed
DO \$\$
BEGIN
    RAISE NOTICE 'User setup completed successfully';
END\$\$;
SQL
)

    # Execute the SQL commands
    docker exec -i xcentro-postgres psql -U "$POSTGRES_SUPERUSER" -d "$POSTGRES_DB" <<< "$sql_commands"
}

# Display user information
show_user_info() {
    print_info "PostgreSQL User Setup Complete!"
    echo
    print_info "Created users:"
    echo -e "${GREEN}• Owner user (${DB_OWNER_USER}):${NC} Full database ownership and permissions"
    echo -e "${GREEN}• Admin user (${DB_ADMIN_USER}):${NC} Can manage tables, indexes, schemas for migrations"
    echo -e "${GREEN}• Read-Write user (${DB_RW_USER}):${NC} Standard CRUD operations for application"
    echo
    print_info "Connection examples:"
    echo "  • Connect as owner: ./connect-postgres.sh owner"
    echo "  • Connect as admin: ./connect-postgres.sh admin"
    echo "  • Connect as rw: ./connect-postgres.sh rw"
    echo
    print_info "List current users:"
    docker exec xcentro-postgres psql -U "$POSTGRES_SUPERUSER" -d "$POSTGRES_DB" -c "\\du"
}

main() {
    print_header
    
    print_info "Setting up PostgreSQL users for XCentro database..."
    
    check_postgres_running
    create_users
    show_user_info
    
    echo
    print_success "PostgreSQL user setup completed!"
}

# Handle command line arguments
case "${1:-}" in
    "help"|"-h"|"--help")
        echo "Usage: $0 [help]"
        echo
        echo "This script creates the required PostgreSQL users for the XCentro application:"
        echo "  • Owner user: Full database permissions"
        echo "  • Admin user: Migration management (create/drop tables, indexes)"
        echo "  • Read-Write user: Standard CRUD operations"
        echo
        echo "Prerequisites:"
        echo "  • PostgreSQL container must be running (./start.sh)"
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
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
-- Create xcentro schema for application
CREATE SCHEMA IF NOT EXISTS xcentro;

-- Install Extensions
CREATE EXTENSION IF NOT EXISTS pg_stat_statements SCHEMA xcentro;
CREATE EXTENSION IF NOT EXISTS pgstattuple SCHEMA xcentro;
CREATE EXTENSION IF NOT EXISTS pgaudit SCHEMA xcentro;

-- Install pg_cron in its default 'cron' schema so background workers can find it
CREATE EXTENSION IF NOT EXISTS pg_cron;

-- CREATE EXTENSION IF NOT EXISTS pgcrypto SCHEMA xcentro;

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

-- Set Schema Ownership
ALTER SCHEMA xcentro OWNER TO ${DB_OWNER_USER};

-- Grant permissions to owner user on cron schema
GRANT USAGE ON SCHEMA cron TO ${DB_OWNER_USER};
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA cron TO ${DB_OWNER_USER};
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA cron TO ${DB_OWNER_USER};

-- Grant permissions to read-write user (for application)
GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO ${DB_RW_USER};
GRANT USAGE ON SCHEMA xcentro TO ${DB_RW_USER};

-- Grant permissions on existing tables and sequences (if any)
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA xcentro TO ${DB_RW_USER};
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA xcentro TO ${DB_RW_USER};

-- Grant EXECUTE permissions on existing functions (if any)
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA xcentro TO ${DB_RW_USER};

-- Set default privileges for future objects created by the owner
-- This ensures that tables/sequences/functions created by migrations will have correct permissions
ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_OWNER_USER} IN SCHEMA xcentro GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ${DB_RW_USER};
ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_OWNER_USER} IN SCHEMA xcentro GRANT USAGE, SELECT ON SEQUENCES TO ${DB_RW_USER};
ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_OWNER_USER} IN SCHEMA xcentro GRANT EXECUTE ON FUNCTIONS TO ${DB_RW_USER};

-- Fix search path so users land in the right schema
ALTER DATABASE ${POSTGRES_DB} SET search_path TO "\$user", xcentro;

-- Example: How owner creates RW users for backends (replace with actual values)
-- CREATE ROLE <backend_name>_rw WITH LOGIN PASSWORD '<secure_password>';
-- GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO <backend_name>_rw;
-- GRANT USAGE ON SCHEMA xcentro TO <backend_name>_rw;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA xcentro TO <backend_name>_rw;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA xcentro TO <backend_name>_rw;

-- Set up default privileges for admin (DDL only - no data access)
-- Admin gets structure management but no data access on future objects

-- User setup completed
DO \$\$
BEGIN
    RAISE NOTICE 'User setup completed successfully';
    RAISE NOTICE 'Owner handles all DDL operations. RW user handles application data access.';
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
    echo -e "${GREEN}• Owner user (${DB_OWNER_USER}):${NC} Database superuser - handles DDL + creates additional RW users"
    echo -e "${GREEN}• Read-Write user (${DB_RW_USER}):${NC} Standard CRUD operations on xcentro schema"
    echo
    print_info "Created schemas:"
    echo -e "${GREEN}• xcentro:${NC} Main application schema (owned by ${DB_OWNER_USER})"
    echo
    print_info "Installed extensions:"
    echo -e "${GREEN}• pg_stat_statements:${NC} Query performance statistics"
    echo -e "${GREEN}• pgstattuple:${NC} Tuple-level statistics"
    echo -e "${GREEN}• pgaudit:${NC} Audit logging"
    echo -e "${GREEN}• pg_cron:${NC} Job scheduling"
    echo
    print_info "Connection examples:"
    echo "  • Connect as owner: ./connect-postgres.sh owner"
    echo "  • Connect as rw: ./connect-postgres.sh rw"
    echo
    print_info "To create additional RW users for other backends (as owner):"
    echo "  CREATE ROLE <backend_name>_rw WITH LOGIN PASSWORD '<secure_password>';"
    echo "  GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO <backend_name>_rw;"
    echo "  GRANT USAGE ON SCHEMA xcentro TO <backend_name>_rw;"
    echo "  GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA xcentro TO <backend_name>_rw;"
    echo "  GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA xcentro TO <backend_name>_rw;"
    echo "  ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_OWNER_USER} IN SCHEMA xcentro GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO <backend_name>_rw;"
    echo "  ALTER DEFAULT PRIVILEGES FOR ROLE ${DB_OWNER_USER} IN SCHEMA xcentro GRANT EXECUTE ON FUNCTIONS TO <backend_name>_rw;"
    echo
    print_info "NOTE: Default privileges are set so future tables/functions created by migrations will have correct permissions."
    print_info "If you need to grant permissions on existing objects after migrations, run:"
    echo "  GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA xcentro TO ${DB_RW_USER};"
    echo "  GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA xcentro TO ${DB_RW_USER};"
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
        echo "  • Owner user: Database superuser (handles DDL operations, creates additional RW users)"
        echo "  • Read-Write user: Standard CRUD operations on xcentro schema"
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

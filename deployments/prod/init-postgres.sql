-- XCentro PostgreSQL Automatic User Setup
-- This script runs automatically when PostgreSQL container starts for the first time
-- It will be placed in /docker-entrypoint-initdb.d/ and executed by the official postgres image

-- Note: This script runs as the postgres superuser automatically

-- Create owner user (full database permissions)
DO $$
DECLARE
    v_db_owner_user TEXT := current_setting('xcentro.db_owner_user', true);
    v_db_owner_password TEXT := current_setting('xcentro.db_owner_password', true);
    v_db_rw_user TEXT := current_setting('xcentro.db_rw_user', true);
    v_db_rw_password TEXT := current_setting('xcentro.db_rw_password', true);
    v_db_name TEXT := current_database();
BEGIN
    -- Create owner role
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = v_db_owner_user) THEN
        EXECUTE format('CREATE ROLE %I WITH LOGIN PASSWORD %L INHERIT', v_db_owner_user, v_db_owner_password);
        RAISE NOTICE 'Created role %', v_db_owner_user;
    ELSE
        RAISE NOTICE 'Role % already exists', v_db_owner_user;
    END IF;

    -- Create read-write user
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = v_db_rw_user) THEN
        EXECUTE format('CREATE ROLE %I WITH LOGIN PASSWORD %L INHERIT', v_db_rw_user, v_db_rw_password);
        RAISE NOTICE 'Created role %', v_db_rw_user;
    ELSE
        RAISE NOTICE 'Role % already exists', v_db_rw_user;
    END IF;

    -- Grant database ownership to owner user
    EXECUTE format('ALTER DATABASE %I OWNER TO %I', v_db_name, v_db_owner_user);

    -- Grant permissions to owner user (full access)
    EXECUTE format('GRANT ALL PRIVILEGES ON DATABASE %I TO %I', v_db_name, v_db_owner_user);

    -- Create xcentro schema for application
    CREATE SCHEMA IF NOT EXISTS xcentro;
    EXECUTE format('ALTER SCHEMA xcentro OWNER TO %I', v_db_owner_user);

    -- Grant permissions to read-write user (for application)
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO %I', v_db_name, v_db_rw_user);
    EXECUTE format('GRANT USAGE ON SCHEMA xcentro TO %I', v_db_rw_user);

    -- Grant permissions on existing tables and sequences (if any)
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA xcentro TO %I', v_db_rw_user);
    EXECUTE format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA xcentro TO %I', v_db_rw_user);

    -- Grant EXECUTE permissions on existing functions (if any)
    EXECUTE format('GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA xcentro TO %I', v_db_rw_user);

    -- Set default privileges for future objects created by the owner
    EXECUTE format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA xcentro GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %I', v_db_owner_user, v_db_rw_user);
    EXECUTE format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA xcentro GRANT USAGE, SELECT ON SEQUENCES TO %I', v_db_owner_user, v_db_rw_user);
    EXECUTE format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA xcentro GRANT EXECUTE ON FUNCTIONS TO %I', v_db_owner_user, v_db_rw_user);

    RAISE NOTICE 'XCentro PostgreSQL user setup completed successfully';
    RAISE NOTICE 'Owner: % handles all DDL operations', v_db_owner_user;
    RAISE NOTICE 'RW User: % handles application data access', v_db_rw_user;
END$$;

#!/bin/sh

# XCentro Redis Automatic User Setup
# This script runs after Redis starts and creates the required ACL users

set -e

echo "Waiting for Redis to be ready..."
# Wait for Redis to be ready (Alpine compatible)
for i in $(seq 1 30); do
    if redis-cli --no-auth-warning -a "$REDIS_SUPERUSER_PASSWORD" ping 2>/dev/null | grep -q PONG; then
        break
    fi
    sleep 1
done

echo "Creating Redis users..."

# Create superuser (dedicated admin user separate from disabled 'default')
redis-cli --no-auth-warning -a "$REDIS_SUPERUSER_PASSWORD" \
    ACL SETUSER "$REDIS_SUPERUSER" on ">${REDIS_SUPERUSER_PASSWORD}" +@all ~* || {
    echo "ERROR: Failed to create Redis superuser"
    exit 1
}
echo "✓ Redis superuser '$REDIS_SUPERUSER' created"

# Create application user with restricted access
redis-cli --no-auth-warning -a "$REDIS_SUPERUSER_PASSWORD" \
    ACL SETUSER "$REDIS_APP_USER" on ">${REDIS_APP_PASSWORD}" +@all -@dangerous ~xcentro:* || {
    echo "ERROR: Failed to create Redis application user"
    exit 1
}
echo "✓ Redis application user '$REDIS_APP_USER' created with access to 'xcentro:*' keys"

# Persist ACL changes
if redis-cli --no-auth-warning -a "$REDIS_SUPERUSER_PASSWORD" ACL SAVE >/dev/null 2>&1; then
    echo "✓ ACL rules persisted to aclfile"
else
    echo "WARNING: Failed to persist ACL rules"
fi

# Disable the default user for security
redis-cli --no-auth-warning --user "$REDIS_SUPERUSER" -a "$REDIS_SUPERUSER_PASSWORD" \
    ACL SETUSER default off || {
    echo "WARNING: Failed to disable default user, but continuing..."
}

# Persist ACL changes after disabling default user
if redis-cli --no-auth-warning --user "$REDIS_SUPERUSER" -a "$REDIS_SUPERUSER_PASSWORD" ACL SAVE >/dev/null 2>&1; then
    echo "✓ ACL rules updated and persisted after disabling default user"
fi

echo "✓ XCentro Redis user setup completed successfully"

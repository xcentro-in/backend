#!/usr/bin/env sh
set -eu

REDIS_CONF="/usr/local/etc/redis/redis.conf"
ACLFILE="/data/users.acl"

echo "[entrypoint] Starting Redis with ACL bootstrap guard"

# Ensure ACL file exists & is valid (must contain a 'user ' directive)
ensure_aclfile() {
  if [ ! -f "$ACLFILE" ]; then
    echo "[entrypoint] ACL file missing. Creating initial $ACLFILE"
    create_aclfile
    return
  fi

  # Basic validation: first non-comment/non-empty line must start with 'user '
  first_line=$(grep -v '^#' "$ACLFILE" | grep -v '^[[:space:]]*$' | head -n1 || true)
  case "$first_line" in
    user\ * )
      echo "[entrypoint] ACL file appears valid (first rule: $first_line)" ;;
    * )
      echo "[entrypoint] WARNING: ACL file invalid. Backing up and recreating." >&2
      cp "$ACLFILE" "${ACLFILE}.bak.$(date +%s)" || true
      create_aclfile
      ;;
  esac
}

create_aclfile() {
  { \
    if [ "${REDIS_SUPERUSER_PASSWORD:-}" != "" ]; then \
      echo "user default on >${REDIS_SUPERUSER_PASSWORD} ~* +@all"; \
    else \
      echo "user default on nopass ~* +@all"; \
    fi; \
  } > "$ACLFILE"
  chmod 600 "$ACLFILE" || true
}

ensure_aclfile

# Exec Redis normally. We do NOT pass --requirepass because ACL file defines password.
exec redis-server "$REDIS_CONF"

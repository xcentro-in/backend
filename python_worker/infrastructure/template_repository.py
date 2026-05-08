"""PostgreSQL template repository using psycopg2 with a connection pool."""

from __future__ import annotations

import logging
import threading
from contextlib import contextmanager

import psycopg2
import psycopg2.extras
from psycopg2 import pool as pg_pool

from domain.ports import TemplateRepository

logger = logging.getLogger(__name__)

psycopg2.extras.register_uuid()


class PostgresTemplateRepository(TemplateRepository):
    """Stores invoice2data YAML templates in xcentro.templates.

    Uses the same SimpleConnectionPool pattern as PostgresDocumentRepository
    so connection handling is consistent across all adapters.
    """

    def __init__(self, dsn: str, minconn: int = 1, maxconn: int = 5) -> None:
        self._dsn = dsn
        self._lock = threading.Lock()
        self._pool = pg_pool.SimpleConnectionPool(minconn, maxconn, dsn=dsn)

    @contextmanager
    def _conn(self):
        conn = self._pool.getconn()
        try:
            if conn.closed:
                self._pool.putconn(conn, close=True)
                conn = self._pool.getconn()
            conn.autocommit = True
            yield conn
        except psycopg2.InterfaceError:
            try:
                self._pool.putconn(conn, close=True)
            except Exception:
                pass
            raise
        else:
            self._pool.putconn(conn)

    def close(self) -> None:
        with self._lock:
            if self._pool and not self._pool.closed:
                self._pool.closeall()

    def list_all(self) -> list[dict]:
        """Return all templates as [{"name": str, "content": str, "issuer": str|None}]."""
        with self._conn() as conn:
            with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
                cur.execute(
                    "SELECT name, content, issuer FROM xcentro.templates ORDER BY name"
                )
                rows = cur.fetchall()
        return [dict(r) for r in rows]

    def save(
        self,
        name: str,
        content: str,
        issuer: str | None = None,
        *,
        replaced_by: str = "regen",
    ) -> None:
        """Upsert a template by name.

        When a row with the same name already exists, its previous content is
        copied into ``xcentro.templates_history`` before the new content
        replaces it. The history insert is a CTE inside the same statement so
        the snapshot and the upsert succeed (or fail) atomically.
        """
        with self._conn() as conn:
            with conn.cursor() as cur:
                cur.execute(
                    """
                    WITH prior AS (
                        SELECT name, content, issuer
                        FROM xcentro.templates
                        WHERE name = %(name)s
                    ),
                    snapshot AS (
                        INSERT INTO xcentro.templates_history (name, content, issuer, replaced_by)
                        SELECT name, content, issuer, %(replaced_by)s FROM prior
                    )
                    INSERT INTO xcentro.templates (name, content, issuer)
                    VALUES (%(name)s, %(content)s, %(issuer)s)
                    ON CONFLICT (name) DO UPDATE
                        SET content    = EXCLUDED.content,
                            issuer     = EXCLUDED.issuer,
                            updated_at = NOW()
                    """,
                    {
                        "name": name,
                        "content": content,
                        "issuer": issuer,
                        "replaced_by": replaced_by,
                    },
                )
        logger.debug("Saved template '%s' to database", name)

    def get_by_name(self, name: str) -> str | None:
        """Return the YAML content for a template, or None if not found."""
        with self._conn() as conn:
            with conn.cursor() as cur:
                cur.execute(
                    "SELECT content FROM xcentro.templates WHERE name = %s",
                    (name,),
                )
                row = cur.fetchone()
        return row[0] if row else None

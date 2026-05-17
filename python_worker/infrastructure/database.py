"""PostgreSQL document repository using psycopg2 with a connection pool."""

from __future__ import annotations

import json
import logging
import threading
import uuid
from contextlib import contextmanager

import psycopg2
import psycopg2.extras
from psycopg2 import pool as pg_pool

from domain.entities import Document
from domain.enums import DocumentStatus, DocumentType, ProcessedBy
from domain.ports import DocumentRepository

logger = logging.getLogger(__name__)

# Register UUID adapter so psycopg2 handles uuid.UUID transparently
psycopg2.extras.register_uuid()


class PostgresDocumentRepository(DocumentRepository):
    """Concrete repository backed by a SimpleConnectionPool.

    A pool is used (instead of a single persistent connection) so that
    connection churn from transient network faults doesn't poison the worker,
    and so that multiple in-flight operations don't contend on one socket.
    """

    def __init__(self, dsn: str, minconn: int = 1, maxconn: int = 5) -> None:
        self._dsn = dsn
        self._lock = threading.Lock()
        self._pool = pg_pool.SimpleConnectionPool(minconn, maxconn, dsn=dsn)

    @contextmanager
    def _conn(self):
        conn = self._pool.getconn()
        try:
            # Test the connection; recycle if broken.
            if conn.closed:
                self._pool.putconn(conn, close=True)
                conn = self._pool.getconn()
            conn.autocommit = True
            yield conn
        except psycopg2.InterfaceError:
            # Broken socket — drop this conn from pool so pool replaces it.
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

    # -- port implementation --------------------------------------------------

    def get_by_id(self, document_id: uuid.UUID) -> Document | None:
        with self._conn() as conn:
            with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
                cur.execute(
                    "SELECT id, s3_path, doc_type, status, raw_ocr, raw_extraction, "
                    "draft_data, validated_data, i2d_template, processed_by "
                    "FROM xcentro.voucher_documents WHERE id = %s",
                    (document_id,),
                )
                row = cur.fetchone()

        if row is None:
            return None

        return Document(
            id=row["id"],
            s3_path=row["s3_path"],
            doc_type=DocumentType(row["doc_type"]) if row["doc_type"] else DocumentType.UNKNOWN,
            status=DocumentStatus(row["status"]) if row["status"] else DocumentStatus.QUEUED,
            raw_ocr=row["raw_ocr"],
            raw_extraction=row["raw_extraction"],
            draft_data=row["draft_data"],
            validated_data=row["validated_data"],
            i2d_template=row["i2d_template"],
            processed_by=ProcessedBy(row["processed_by"]) if row["processed_by"] else None,
        )

    def get_validated_by_template(self, template_name: str, limit: int = 5) -> list[dict]:
        with self._conn() as conn:
            with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
                cur.execute(
                    "SELECT validated_data FROM xcentro.voucher_documents "
                    "WHERE i2d_template = %s AND status = 'VALIDATED' AND validated_data IS NOT NULL "
                    "ORDER BY updated_at DESC LIMIT %s",
                    (template_name, limit),
                )
                rows = cur.fetchall()
        return [row["validated_data"] for row in rows if row["validated_data"]]

    def get_validated_with_ocr_by_template(
        self, template_name: str, limit: int = 5
    ) -> list[dict]:
        with self._conn() as conn:
            with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
                cur.execute(
                    "SELECT raw_ocr, validated_data FROM xcentro.voucher_documents "
                    "WHERE i2d_template = %s AND status = 'VALIDATED' "
                    "AND validated_data IS NOT NULL AND raw_ocr IS NOT NULL "
                    "AND raw_ocr <> '' "
                    "ORDER BY updated_at DESC LIMIT %s",
                    (template_name, limit),
                )
                rows = cur.fetchall()
        return [
            {"raw_ocr": row["raw_ocr"], "validated_data": row["validated_data"]}
            for row in rows
            if row["validated_data"] and row["raw_ocr"]
        ]

    def update(self, document: Document) -> None:
        with self._conn() as conn:
            with conn.cursor() as cur:
                cur.execute(
                    "UPDATE xcentro.voucher_documents SET "
                    "doc_type = %s, status = %s, raw_ocr = %s, raw_extraction = %s, "
                    "draft_data = %s, validated_data = %s, "
                    "i2d_template = %s, processed_by = %s "
                    "WHERE id = %s",
                    (
                        document.doc_type.value if document.doc_type else None,
                        document.status.value if document.status else None,
                        document.raw_ocr,
                        json.dumps(document.raw_extraction, default=str) if document.raw_extraction else None,
                        json.dumps(document.draft_data, default=str) if document.draft_data else None,
                        json.dumps(document.validated_data, default=str) if document.validated_data else None,
                        document.i2d_template,
                        document.processed_by.value if document.processed_by else None,
                        document.id,
                    ),
                )
        logger.debug("Updated document %s → status=%s", document.id, document.status.value)

"""PostgreSQL document repository using psycopg2."""

from __future__ import annotations

import json
import logging
import uuid

import psycopg2
import psycopg2.extras

from domain.entities import Document
from domain.enums import DocumentStatus, DocumentType, ProcessedBy
from domain.ports import DocumentRepository

logger = logging.getLogger(__name__)

# Register UUID adapter so psycopg2 handles uuid.UUID transparently
psycopg2.extras.register_uuid()


class PostgresDocumentRepository(DocumentRepository):
    """Concrete repository backed by a psycopg2 connection pool."""

    def __init__(self, dsn: str) -> None:
        self._dsn = dsn
        self._conn: psycopg2.extensions.connection | None = None

    # -- connection helpers ---------------------------------------------------

    def _get_conn(self) -> psycopg2.extensions.connection:
        """Return (and lazily create) a persistent connection."""
        if self._conn is None or self._conn.closed:
            self._conn = psycopg2.connect(self._dsn)
            self._conn.autocommit = True
        return self._conn

    def close(self) -> None:
        if self._conn and not self._conn.closed:
            self._conn.close()

    # -- port implementation --------------------------------------------------

    def get_by_id(self, document_id: uuid.UUID) -> Document | None:
        conn = self._get_conn()
        with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
            cur.execute(
                "SELECT id, s3_path, doc_type, status, raw_extraction, "
                "draft_data, i2d_template, processed_by "
                "FROM voucher_documents WHERE id = %s",
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
            raw_extraction=row["raw_extraction"],
            draft_data=row["draft_data"],
            i2d_template=row["i2d_template"],
            processed_by=ProcessedBy(row["processed_by"]) if row["processed_by"] else None,
        )

    def update(self, document: Document) -> None:
        conn = self._get_conn()
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE voucher_documents SET "
                "doc_type = %s, status = %s, raw_extraction = %s, "
                "draft_data = %s, i2d_template = %s, processed_by = %s "
                "WHERE id = %s",
                (
                    document.doc_type.value if document.doc_type else None,
                    document.status.value if document.status else None,
                    json.dumps(document.raw_extraction, default=str) if document.raw_extraction else None,
                    json.dumps(document.draft_data, default=str) if document.draft_data else None,
                    document.i2d_template,
                    document.processed_by.value if document.processed_by else None,
                    document.id,
                ),
            )
        logger.debug("Updated document %s → status=%s", document.id, document.status.value)

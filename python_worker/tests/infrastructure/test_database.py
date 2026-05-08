"""Tests for PostgresDocumentRepository.

The repo is a thin mapper around psycopg2 — real Postgres integration lives
behind docker-compose. Here we patch the SimpleConnectionPool and exercise
every branch of get_by_id / update / get_validated_by_template / close with
pure mocks. Ensures the SQL shape and JSON serialization contract stay pinned.
"""

from __future__ import annotations

import json
import uuid
from unittest.mock import MagicMock, patch

import pytest

from domain.entities import Document
from domain.enums import DocumentStatus, DocumentType, ProcessedBy


def _build_repo():
    """Construct PostgresDocumentRepository with its pool patched out.

    Returns (repo, pool_mock, conn_mock, cursor_mock). The cursor_mock is what
    tests configure fetchone / fetchall / execute assertions against.
    """
    pool = MagicMock()
    conn = MagicMock()
    cursor = MagicMock()

    pool.getconn.return_value = conn
    pool.closed = False
    conn.closed = False
    conn.cursor.return_value.__enter__.return_value = cursor
    conn.cursor.return_value.__exit__.return_value = False

    with patch("infrastructure.database.pg_pool.SimpleConnectionPool", return_value=pool):
        from infrastructure.database import PostgresDocumentRepository

        repo = PostgresDocumentRepository(dsn="host=x")

    return repo, pool, conn, cursor


def test_get_by_id_returns_mapped_document():
    repo, _pool, _conn, cursor = _build_repo()
    doc_id = uuid.uuid4()
    cursor.fetchone.return_value = {
        "id": doc_id,
        "s3_path": "uploads/a.pdf",
        "doc_type": "INVOICE",
        "status": "VALIDATED",
        "raw_ocr": "text",
        "raw_extraction": {"a": 1},
        "draft_data": {"b": 2},
        "validated_data": {"c": 3},
        "i2d_template": "acme.yml",
        "processed_by": "LLM",
    }

    doc = repo.get_by_id(doc_id)

    assert doc is not None
    assert doc.id == doc_id
    assert doc.doc_type == DocumentType.INVOICE
    assert doc.status == DocumentStatus.VALIDATED
    assert doc.raw_ocr == "text"
    assert doc.raw_extraction == {"a": 1}
    assert doc.i2d_template == "acme.yml"
    assert doc.processed_by == ProcessedBy.LLM


def test_get_by_id_returns_none_when_row_missing():
    repo, _pool, _conn, cursor = _build_repo()
    cursor.fetchone.return_value = None

    assert repo.get_by_id(uuid.uuid4()) is None


def test_get_by_id_defaults_null_doc_type_to_unknown():
    repo, _pool, _conn, cursor = _build_repo()
    doc_id = uuid.uuid4()
    cursor.fetchone.return_value = {
        "id": doc_id, "s3_path": "x",
        "doc_type": None, "status": None,
        "raw_ocr": None, "raw_extraction": None,
        "draft_data": None, "validated_data": None,
        "i2d_template": None, "processed_by": None,
    }

    doc = repo.get_by_id(doc_id)

    assert doc is not None
    assert doc.doc_type == DocumentType.UNKNOWN
    assert doc.status == DocumentStatus.QUEUED
    assert doc.processed_by is None


def test_update_serializes_jsonb_fields_and_enums():
    repo, _pool, _conn, cursor = _build_repo()
    doc_id = uuid.uuid4()
    doc = Document(
        id=doc_id,
        s3_path="uploads/a.pdf",
        doc_type=DocumentType.INVOICE,
        status=DocumentStatus.REVIEW_REQUIRED,
        raw_ocr="text",
        raw_extraction={"x": 1},
        draft_data={"y": 2},
        validated_data={"z": 3},
        i2d_template="t.yml",
        processed_by=ProcessedBy.INVOICE2DATA,
    )

    repo.update(doc)

    sql, params = cursor.execute.call_args[0]
    assert "UPDATE xcentro.voucher_documents" in sql
    # Enums → string values, not Enum instances.
    assert params[0] == "INVOICE"
    assert params[1] == "REVIEW_REQUIRED"
    # JSONB fields → JSON strings, not raw dicts.
    assert params[3] == json.dumps({"x": 1})
    assert params[4] == json.dumps({"y": 2})
    assert params[5] == json.dumps({"z": 3})
    assert params[6] == "t.yml"
    assert params[7] == "INVOICE2DATA"
    assert params[8] == doc_id


def test_update_handles_null_optional_fields():
    repo, _pool, _conn, cursor = _build_repo()
    doc = Document(
        id=uuid.uuid4(),
        s3_path="uploads/a.pdf",
        doc_type=DocumentType.UNKNOWN,
        status=DocumentStatus.QUEUED,
    )

    repo.update(doc)

    _sql, params = cursor.execute.call_args[0]
    # raw_ocr, raw_extraction, draft_data, validated_data, i2d_template, processed_by all None.
    assert params[2] is None  # raw_ocr
    assert params[3] is None  # raw_extraction
    assert params[4] is None  # draft_data
    assert params[5] is None  # validated_data
    assert params[6] is None  # i2d_template
    assert params[7] is None  # processed_by


def test_get_validated_by_template_returns_validated_data_list():
    repo, _pool, _conn, cursor = _build_repo()
    cursor.fetchall.return_value = [
        {"validated_data": {"invoice_number": "INV-1"}},
        {"validated_data": {"invoice_number": "INV-2"}},
        {"validated_data": None},  # filtered out
    ]

    out = repo.get_validated_by_template("acme.yml", limit=5)

    assert out == [{"invoice_number": "INV-1"}, {"invoice_number": "INV-2"}]
    sql, params = cursor.execute.call_args[0]
    assert "WHERE i2d_template = %s" in sql
    assert "status = 'VALIDATED'" in sql
    assert params == ("acme.yml", 5)


def test_close_closes_pool_and_is_idempotent():
    repo, pool, _conn, _cursor = _build_repo()

    repo.close()
    pool.closeall.assert_called_once()

    # Second close is a no-op because pool.closed becomes truthy.
    pool.closed = True
    repo.close()
    pool.closeall.assert_called_once()  # still only one call

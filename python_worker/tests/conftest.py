"""Shared pytest fixtures for the python_worker test suite.

The worker package imports modules via top-level names like `domain.ports` and
`infrastructure.redis_client` (see the app's own imports in main.py). Put the
worker root on sys.path so tests can do the same.
"""

from __future__ import annotations

import os
import sys
import uuid
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

_WORKER_ROOT = Path(__file__).resolve().parent.parent
if str(_WORKER_ROOT) not in sys.path:
    sys.path.insert(0, str(_WORKER_ROOT))

if TYPE_CHECKING:
    from domain.entities import Document


@pytest.fixture
def fake_redis():
    """A fresh fakeredis.FakeRedis instance per test.

    Uses decode_responses=True to match the production client config in
    infrastructure/redis_client.py.
    """
    import fakeredis

    client = fakeredis.FakeRedis(decode_responses=True)
    try:
        yield client
    finally:
        client.flushall()
        client.close()


@pytest.fixture
def sample_extraction_result():
    """A fully-populated ExtractionResult for mapping / normalization tests."""
    from domain.entities import ExtractionResult, LineItem
    from domain.enums import DocumentType

    return ExtractionResult(
        invoice_number="INV-001",
        invoice_date="2026-04-20",
        po_number="PO-123",
        vendor_name="Acme Corp",
        vendor_gstin="27AADCB2230M1ZV",
        buyer_gstin="27AAAAA0000A1Z5",
        total_amount=1180.0,
        subtotal=1000.0,
        gross_amount=1180.0,
        sgst=90.0,
        cgst=90.0,
        igst=None,
        gst_rate=18.0,
        line_items=[
            LineItem(sku="HSN1234", description="Widget", quantity=1.0, unit_price=1000.0, amount=1000.0),
        ],
        doc_type=DocumentType.INVOICE,
        confidence_scores={"total_amount": 1.0, "vendor_gstin": 1.0},
    )


# ---------------------------------------------------------------------------
# Stub adapters for extraction_service + main loop tests.
# ---------------------------------------------------------------------------


class FakeDocumentRepo:
    """In-memory DocumentRepository stub.

    Stores documents keyed by UUID, counts update calls, and exposes hooks
    for tests that need to inspect the last-written state.
    """

    def __init__(self) -> None:
        self._store: dict[uuid.UUID, Document] = {}
        self.update_calls: list[Document] = []
        self.validated_by_template: dict[str, list[dict]] = {}
        self.validated_with_ocr_by_template: dict[str, list[dict]] = {}

    def add(self, document: Document) -> None:
        self._store[document.id] = document

    def get_by_id(self, document_id):
        return self._store.get(document_id)

    def update(self, document) -> None:
        import copy

        self._store[document.id] = document
        self.update_calls.append(copy.deepcopy(document))

    def get_validated_by_template(self, template_name: str, limit: int = 5) -> list[dict]:
        return list(self.validated_by_template.get(template_name, []))[:limit]

    def get_validated_with_ocr_by_template(self, template_name: str, limit: int = 5) -> list[dict]:
        return list(self.validated_with_ocr_by_template.get(template_name, []))[:limit]


class FakeTemplateRepo:
    """In-memory TemplateRepository stub."""

    def __init__(self) -> None:
        self._store: dict[str, dict] = {}
        self.history: list[dict] = []
        self.save_calls: list[tuple[str, str, str | None]] = []

    def list_all(self) -> list[dict]:
        return [dict(v) for v in self._store.values()]

    def save(
        self,
        name: str,
        content: str,
        issuer: str | None = None,
        *,
        replaced_by: str = "regen",
    ) -> None:
        if name in self._store:
            prior = self._store[name]
            self.history.append({**prior, "replaced_by": replaced_by})
        self._store[name] = {"name": name, "content": content, "issuer": issuer}
        self.save_calls.append((name, content, issuer))

    def get_by_name(self, name: str) -> str | None:
        entry = self._store.get(name)
        return entry["content"] if entry else None


class FakeFileDownloader:
    """FileDownloader stub that writes a canned byte string to dest_dir."""

    def __init__(self, content: bytes = b"%PDF-1.4 fake content\n") -> None:
        self.content = content
        self.calls: list[tuple[str, str]] = []

    def download(self, s3_path: str, dest_dir: str) -> str:
        self.calls.append((s3_path, dest_dir))
        os.makedirs(dest_dir, exist_ok=True)
        filename = os.path.basename(s3_path) or "document"
        path = os.path.join(dest_dir, filename)
        with open(path, "wb") as fh:
            fh.write(self.content)
        return path


@pytest.fixture
def fake_document_repo():
    return FakeDocumentRepo()


@pytest.fixture
def fake_template_repo():
    return FakeTemplateRepo()


@pytest.fixture
def fake_file_downloader():
    return FakeFileDownloader()


@pytest.fixture
def sample_document():
    """A minimal Document in QUEUED status ready for pipeline seeding."""
    from domain.entities import Document
    from domain.enums import DocumentStatus, DocumentType

    return Document(
        id=uuid.uuid4(),
        s3_path="uploads/test.pdf",
        doc_type=DocumentType.UNKNOWN,
        status=DocumentStatus.QUEUED,
    )


def pytest_collection_modifyitems(config, items):
    """Skip live_llm tests unless RUN_LIVE_LLM_TESTS=1 is set."""
    if os.environ.get("RUN_LIVE_LLM_TESTS") == "1":
        return
    skip_live = pytest.mark.skip(reason="set RUN_LIVE_LLM_TESTS=1 to enable")
    for item in items:
        if "live_llm" in item.keywords:
            item.add_marker(skip_live)

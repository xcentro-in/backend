"""Port interfaces (abstract contracts) for infrastructure adapters.

These define *what* the domain needs without knowing *how* it is provided.
Infrastructure adapters implement these contracts.
"""

from __future__ import annotations

import abc
import uuid
from typing import Any

from domain.entities import Document, ExtractionResult


class DocumentRepository(abc.ABC):
    """Persistence port for the documents table."""

    @abc.abstractmethod
    def get_by_id(self, document_id: uuid.UUID) -> Document | None:
        """Fetch a document by its primary key."""

    @abc.abstractmethod
    def update(self, document: Document) -> None:
        """Persist all mutable fields of the document."""

    @abc.abstractmethod
    def get_validated_by_template(self, template_name: str, limit: int = 5) -> list[dict]:
        """Return validated_data dicts for the most recent VALIDATED documents using this i2d template.

        Used to supply historical examples to the LLM during template regeneration.
        Returns an empty list if no matching documents exist.
        """

    def get_validated_with_ocr_by_template(
        self, template_name: str, limit: int = 5
    ) -> list[dict]:
        """Return [{'raw_ocr': str, 'validated_data': dict}, ...] for recent
        VALIDATED docs using this i2d template.

        Pairs OCR text with validated values so a freshly-generated template
        can be tested against real prior documents (not just the source it was
        generated from). Default implementation returns [] so adapters that
        don't expose OCR can opt out without breaking the port.
        """
        return []


class TemplateExtractor(abc.ABC):
    """Port for invoice2data (or any template-based extractor)."""

    @abc.abstractmethod
    def extract(self, file_path: str) -> ExtractionResult:
        """Run template matching on a local file and return structured data.

        Raises:
            TemplateMatchError: If no template matches.
        """


class TemplateRepository(abc.ABC):
    """Persistence port for invoice2data YAML templates."""

    @abc.abstractmethod
    def list_all(self) -> list[dict]:
        """Return all templates as [{"name": str, "content": str, "issuer": str|None}]."""

    @abc.abstractmethod
    def save(
        self,
        name: str,
        content: str,
        issuer: str | None = None,
        *,
        replaced_by: str = "regen",
    ) -> None:
        """Upsert a template by name.

        ``replaced_by`` is a short tag stored on the templates_history row
        when an existing template is overwritten — e.g. "hitl_regen" or
        "initial_or_correction". It lets us audit *why* a template version
        was retired without having to dig through service logs.
        """

    @abc.abstractmethod
    def get_by_name(self, name: str) -> str | None:
        """Return the YAML content for a template, or None if not found."""


class LLMExtractor(abc.ABC):
    """Port for LLM-based extraction / template generation."""

    @abc.abstractmethod
    def extract(self, file_path: str) -> ExtractionResult:
        """Process the file using an LLM and return structured ExtractionResult."""
        pass

    @abc.abstractmethod
    def generate_template(
        self,
        raw_ocr: str,
        draft_data: dict,
        historical_examples: list[dict] | None = None,
        prior_attempt: str | None = None,
        prior_error: str | None = None,
    ) -> str:
        """Generate an invoice2data YAML template from OCR text and expected data.

        Args:
            raw_ocr: Plain-text OCR content of the document.
            draft_data: The target data values to extract (current document).
            historical_examples: Optional list of validated_data dicts from prior documents
                that used the same template. Included in the prompt to help the LLM
                produce a more accurate template.
            prior_attempt: Optional YAML from a previous generation attempt that
                failed round-trip verification. Passed back to the LLM on retry
                so it can learn from the failure instead of repeating it.
            prior_error: Optional short reason describing why ``prior_attempt``
                failed verification (e.g. "amount_mismatch: amount 100 vs 1180").

        Raises:
            LLMExtractionError: If the LLM call fails.
        """



class FileDownloader(abc.ABC):
    """Port for downloading the source file (e.g. from S3) to a local path."""

    @abc.abstractmethod
    def download(self, url: str, dest_dir: str) -> str:
        """Download the file and return the local filesystem path."""


class TaskQueue(abc.ABC):
    """Port for consuming tasks from a message queue."""

    @abc.abstractmethod
    def dequeue(self, timeout: int = 0) -> dict[str, Any] | None:
        """Block-pop a task payload from the queue.

        Returns None on timeout.
        """

    @abc.abstractmethod
    def enqueue(self, payload: dict[str, Any]) -> None:
        """Push a task payload back onto the queue (e.g. for retries)."""

    def depths(self) -> dict[str, int]:
        """Return LLEN for each source queue. Default returns empty so tests/fakes
        don't have to implement monitoring hooks."""
        return {}

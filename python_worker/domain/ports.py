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


class TemplateExtractor(abc.ABC):
    """Port for invoice2data (or any template-based extractor)."""

    @abc.abstractmethod
    def extract(self, file_path: str) -> ExtractionResult:
        """Run template matching on a local file and return structured data.

        Raises:
            TemplateMatchError: If no template matches.
        """


class LLMExtractor(abc.ABC):
    """Port for LLM-based extraction / template generation."""

    @abc.abstractmethod
    def extract(self, file_path: str) -> ExtractionResult:
        """Process the file using an LLM and return structured ExtractionResult."""
        pass

    @abc.abstractmethod
    def generate_template(self, raw_ocr: str, draft_data: dict, historical_examples: list[dict] | None = None) -> str:
        """Generate an invoice2data YAML template from OCR text and expected data.

        Args:
            raw_ocr: Plain-text OCR content of the document.
            draft_data: The target data values to extract (current document).
            historical_examples: Optional list of validated_data dicts from prior documents
                that used the same template. Included in the prompt to help the LLM
                produce a more accurate template.

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

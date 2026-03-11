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
        """Send the document to the LLM and return structured data.

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

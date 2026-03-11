"""Python Extraction Worker — entry point.

Wires together all infrastructure adapters and starts the Redis
consumer loop that feeds the ExtractionService use case.
"""

from __future__ import annotations

import sys
import logging
# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
print("THIS IS THE NEW BUILD")
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s — %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
    handlers=[logging.StreamHandler(sys.stdout)],
)
logger = logging.getLogger("worker")
import signal
import uuid

import boto3

from infrastructure.config import Settings
from infrastructure.database import PostgresDocumentRepository
from infrastructure.file_downloader import S3FileDownloader
from infrastructure.invoice2data_wrapper import Invoice2DataExtractor
from infrastructure.llm_client import GeminiLLMExtractor
from infrastructure.redis_client import RedisTaskQueue
from usecases.extraction_service import ExtractionService

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
MAX_RETRIES = 3

# ---------------------------------------------------------------------------
# Graceful shutdown
# ---------------------------------------------------------------------------
_shutdown = False


def _handle_signal(signum, _frame):
    global _shutdown
    logger.info("Received signal %s — shutting down gracefully …", signum)
    _shutdown = True


signal.signal(signal.SIGINT, _handle_signal)
signal.signal(signal.SIGTERM, _handle_signal)


def main() -> None:
    settings = Settings()

    # -- Build DSN -----------------------------------------------------------
    dsn = (
        f"host={settings.postgres_host} port={settings.postgres_port} "
        f"dbname={settings.postgres_db} user={settings.postgres_user} "
        f"password={settings.postgres_password}"
    )

    # -- Wire dependencies ---------------------------------------------------
    repo = PostgresDocumentRepository(dsn)
    queue = RedisTaskQueue(
        host=settings.redis_host,
        port=settings.redis_port,
        queue_name=settings.redis_queue,
        username=settings.redis_user,
        password=settings.redis_password,
    )
    template_extractor = Invoice2DataExtractor(settings.i2d_template_dir)
    llm_extractor = GeminiLLMExtractor(
        api_key=settings.gemini_api_key,
        model=settings.gemini_model,
    )

    # -- S3 client -----------------------------------------------------------
    s3_client = boto3.client(
        "s3",
        aws_access_key_id=settings.aws_access_key_id,
        aws_secret_access_key=settings.aws_secret_access_key,
        region_name=settings.aws_region,
    )
    downloader = S3FileDownloader(
        s3_client=s3_client,
        bucket_name=settings.s3_bucket_name,
    )

    service = ExtractionService(
        repo=repo,
        template_extractor=template_extractor,
        llm_extractor=llm_extractor,
        downloader=downloader,
        temp_dir=settings.temp_dir,
    )

    # -- Consumer loop -------------------------------------------------------
    logger.info("Worker started — listening on queue '%s'", settings.redis_queue)

    try:
        while not _shutdown:
            payload = queue.dequeue(timeout=5)
            if payload is None:
                continue  # timeout — loop back and check shutdown flag

            doc_id_raw = payload.get("document_id")

            if not doc_id_raw:
                logger.error("Malformed task payload — skipping: %s", payload)
                continue

            try:
                doc_id = uuid.UUID(doc_id_raw)
            except ValueError:
                logger.error("Invalid UUID in payload: %s", doc_id_raw)
                continue

            retry_count = payload.get("retry_count", 0)
            logger.info("Processing document %s (attempt %d/%d)", doc_id, retry_count + 1, MAX_RETRIES)

            try:
                service.process(doc_id)
            except Exception as exc:
                # Infrastructure error (S3, LLM API, etc.) — eligible for retry
                logger.exception("Error processing document %s (attempt %d/%d): %s",
                                 doc_id, retry_count + 1, MAX_RETRIES, exc)

                if retry_count + 1 < MAX_RETRIES:
                    retry_payload = {**payload, "retry_count": retry_count + 1}
                    queue.enqueue(retry_payload)
                    logger.info("Re-queued document %s for retry %d/%d",
                                doc_id, retry_count + 2, MAX_RETRIES)
                else:
                    logger.error("Document %s exhausted all %d retries — marking as FAILED",
                                 doc_id, MAX_RETRIES)
                    service.mark_failed(doc_id, reason=f"Max retries ({MAX_RETRIES}) exhausted: {exc}")

    finally:
        logger.info("Cleaning up connections …")
        queue.close()
        repo.close()
        logger.info("Worker shut down.")


if __name__ == "__main__":
    main()

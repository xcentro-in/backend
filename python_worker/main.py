"""Python Extraction Worker — entry point.

Wires together all infrastructure adapters and starts the Redis
consumer loop that feeds the ExtractionService use case.
"""

from __future__ import annotations

import logging
import signal
import sys
import tempfile
import time
import uuid

import boto3

# ---------------------------------------------------------------------------
# Logging (configure before importing modules that create loggers at import time)
# ---------------------------------------------------------------------------
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s — %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
    handlers=[logging.StreamHandler(sys.stdout)],
)
logger = logging.getLogger("worker")

from infrastructure.config import Settings
from infrastructure.database import PostgresDocumentRepository
from infrastructure.file_downloader import S3FileDownloader
from infrastructure.invoice2data_wrapper import Invoice2DataExtractor
from infrastructure.llm_client import GeminiLLMExtractor, configure_redis_concurrency
from infrastructure.redis_client import RedisTaskQueue
from infrastructure.template_repository import PostgresTemplateRepository
from infrastructure.template_seeder import seed_from_filesystem
from domain.exceptions import TemplateGenerationError
from usecases.extraction_service import ExtractionService

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
MAX_RETRIES = 3
CONNECTION_RETRY_DELAY = 5  # seconds to wait before reconnecting to infra
# A reaper sweep re-queues tasks whose heartbeat is older than this —
# covers the case where a worker was SIGKILLed mid-task.
STALE_TASK_AGE_SECONDS = 15 * 60
REAPER_INTERVAL_SECONDS = 60
# Per-document distributed lock TTL. Must be >= STALE_TASK_AGE_SECONDS so a
# legitimate long-running extraction never loses its lock before the reaper
# would consider the task stale — otherwise a second worker can pick up an
# in-flight document and produce duplicate work.
LOCK_TTL_SECONDS = STALE_TASK_AGE_SECONDS + 60

# ---------------------------------------------------------------------------
# Graceful shutdown — the flag is checked between tasks; an in-flight task
# is allowed to complete so we don't strand a half-processed document.
# ---------------------------------------------------------------------------
_shutdown = False


def _handle_signal(signum, _frame):
    global _shutdown
    logger.info("Received signal %s — finishing current task then exiting …", signum)
    _shutdown = True


signal.signal(signal.SIGINT, _handle_signal)
signal.signal(signal.SIGTERM, _handle_signal)


def _resolve_temp_dir(configured: str) -> str:
    """Return a private temp dir. Falls back to a 0700 tmpdir if none set."""
    if configured:
        return configured
    return tempfile.mkdtemp(prefix="extraction_", suffix="_wk")


def handle_message(payload, queue, service, settings) -> str:
    """Process one dequeued task. Returns a short outcome tag for tests / logs.

    Outcomes:
      - "dlq_malformed"         — missing document_id
      - "dlq_bad_uuid"          — document_id is not a UUID
      - "lock_busy"             — another worker holds the doc lock
      - "ok"                    — processed and ack'd
      - "retry"                 — failed, re-enqueued with incremented retry_count
      - "dlq_retries_exhausted" — failed past MAX_RETRIES → DLQ + mark_failed (extraction queue)
      - "dlq_non_retryable"     — non-retryable TemplateGenerationError → DLQ
      - "dlq_enqueue_failed"    — re-enqueue raised; payload sent to DLQ instead

    Lifting this body out of the dequeue loop makes the per-message
    behaviour testable without needing a real Redis or threads.
    """
    doc_id_raw = payload.get("document_id")
    if not doc_id_raw:
        logger.error("Malformed task payload — dropping to DLQ: %s", payload)
        queue.dead_letter(payload, reason="missing document_id")
        return "dlq_malformed"

    try:
        doc_id = uuid.UUID(doc_id_raw)
    except ValueError:
        logger.error("Invalid UUID in payload — DLQ: %s", doc_id_raw)
        queue.dead_letter(payload, reason=f"invalid uuid: {doc_id_raw}")
        return "dlq_bad_uuid"

    retry_count = payload.get("retry_count", 0)
    queue_name = payload.get("_queue_name", settings.redis_queue)
    task_type = "Extraction" if queue_name == settings.redis_queue else "Template Generation"
    logger.info("Processing %s task for document %s (attempt %d/%d)",
                task_type, doc_id, retry_count + 1, MAX_RETRIES)

    lock_key = f"xcentro:doc_lock:{doc_id}"
    if not queue.acquire_lock(lock_key, ttl=LOCK_TTL_SECONDS):
        logger.warning("Document %s is locked by another worker — releasing back to queue", doc_id)
        # Another worker owns it — hand the message back so we don't
        # drop it on the floor by ack'ing.
        queue.enqueue({k: v for k, v in payload.items() if k != "_raw"})
        queue.ack(payload)
        return "lock_busy"

    try:
        if queue_name == settings.redis_template_queue:
            force_regen = bool(payload.get("force_regen"))
            service.generate_template_for_document(doc_id, force_regen=force_regen)
        else:
            service.process(doc_id)
        queue.ack(payload)
        return "ok"
    except Exception as exc:
        logger.exception("Error processing document %s (attempt %d/%d): %s",
                         doc_id, retry_count + 1, MAX_RETRIES, exc)

        is_retryable = not (isinstance(exc, TemplateGenerationError) and not exc.retryable)

        if is_retryable and retry_count + 1 < MAX_RETRIES:
            retry_payload = {k: v for k, v in payload.items() if k != "_raw"}
            retry_payload["retry_count"] = retry_count + 1
            try:
                queue.enqueue(retry_payload)
                queue.ack(payload)
                logger.info("Re-queued document %s for retry %d/%d",
                            doc_id, retry_count + 2, MAX_RETRIES)
                return "retry"
            except Exception as enqueue_err:
                logger.error("Failed to re-queue document %s: %s — DLQ", doc_id, enqueue_err)
                queue.dead_letter(payload, reason=f"enqueue failed: {enqueue_err}; last error: {exc}")
                if queue_name != settings.redis_template_queue:
                    try:
                        service.mark_failed(doc_id, reason=f"Enqueue failed: {exc}")
                    except Exception:
                        logger.exception("Could not mark document %s as FAILED", doc_id)
                return "dlq_enqueue_failed"

        if not is_retryable:
            logger.error("Document %s has a permanent template generation failure (%s) — DLQ",
                         doc_id, exc)
            outcome = "dlq_non_retryable"
        else:
            logger.error("Document %s exhausted retries — DLQ", doc_id)
            outcome = "dlq_retries_exhausted"
        queue.dead_letter(payload, reason=f"max retries exhausted: {exc}")
        if queue_name != settings.redis_template_queue:
            try:
                service.mark_failed(doc_id, reason=f"Max retries ({MAX_RETRIES}) exhausted: {exc}")
            except Exception:
                logger.exception("Could not mark document %s as FAILED", doc_id)
        return outcome
    finally:
        queue.release_lock(lock_key)


def main() -> None:
    settings = Settings()
    temp_dir = _resolve_temp_dir(settings.temp_dir)

    dsn = (
        f"host={settings.postgres_host} port={settings.postgres_port} "
        f"dbname={settings.postgres_db} user={settings.postgres_user} "
        f"password={settings.postgres_password}"
    )

    repo = PostgresDocumentRepository(dsn)
    template_repo = PostgresTemplateRepository(dsn)
    seed_from_filesystem(settings.i2d_template_dir, template_repo)

    queue = RedisTaskQueue(
        host=settings.redis_host,
        port=settings.redis_port,
        queues=[settings.redis_queue, settings.redis_template_queue],
        username=settings.redis_user,
        password=settings.redis_password,
    )
    # Share the Gemini concurrency cap across all worker containers via Redis.
    configure_redis_concurrency(queue._client, settings.gemini_max_concurrent)

    template_extractor = Invoice2DataExtractor(
        template_dir=settings.i2d_template_dir,
        template_repo=template_repo,
    )
    llm_extractor = GeminiLLMExtractor(
        api_key=settings.gemini_api_key,
        model=settings.gemini_model,
        fallback_model=settings.gemini_fallback_model,
    )

    s3_client = boto3.client(
        "s3",
        aws_access_key_id=settings.aws_access_key_id,
        aws_secret_access_key=settings.aws_secret_access_key,
        region_name=settings.aws_region,
    )
    downloader = S3FileDownloader(
        s3_client=s3_client,
        bucket_name=settings.s3_bucket_name,
        max_bytes=settings.max_download_bytes,
    )

    service = ExtractionService(
        repo=repo,
        template_extractor=template_extractor,
        llm_extractor=llm_extractor,
        downloader=downloader,
        temp_dir=temp_dir,
        template_dir=settings.i2d_template_dir,
        template_repo=template_repo,
    )

    # Recover anything abandoned by a prior crash before we start accepting
    # new work.
    try:
        reaped = queue.reap_stale(STALE_TASK_AGE_SECONDS)
        if reaped:
            logger.warning("Reaped %d stale in-flight tasks at startup", reaped)
    except Exception as exc:
        logger.error("Startup reap failed: %s", exc)

    last_reap = time.time()
    logger.info("Worker started — listening on queues %s", [settings.redis_queue, settings.redis_template_queue])

    try:
        while not _shutdown:
            try:
                payload = queue.dequeue(timeout=5)
            except Exception as conn_err:
                logger.error("Redis connection error: %s — retrying in %ds", conn_err, CONNECTION_RETRY_DELAY)
                time.sleep(CONNECTION_RETRY_DELAY)
                continue

            # Periodic reaper tick runs even when idle.
            if time.time() - last_reap >= REAPER_INTERVAL_SECONDS:
                try:
                    queue.reap_stale(STALE_TASK_AGE_SECONDS)
                except Exception as exc:
                    logger.warning("Reaper sweep failed: %s", exc)
                try:
                    depths = queue.depths()
                    depth_parts = " ".join(f"{q}={n}" for q, n in depths.items())
                    logger.info("event=queue.depth %s", depth_parts)
                except Exception as exc:
                    logger.warning("Depth probe failed: %s", exc)
                last_reap = time.time()

            if payload is None:
                continue

            handle_message(payload, queue, service, settings)

    finally:
        logger.info("Cleaning up connections …")
        try:
            queue.close()
        except Exception:
            logger.exception("Error closing queue")
        try:
            repo.close()
        except Exception:
            logger.exception("Error closing repo")
        logger.info("Worker shut down.")


if __name__ == "__main__":
    main()

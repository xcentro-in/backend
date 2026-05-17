"""Tests for the per-message handler in main.py.

The dequeue/lock/retry/DLQ glue is the most failure-prone part of any
worker. handle_message() was extracted from the dequeue loop precisely so
this behaviour is testable without spinning up Redis or threads.
"""

from __future__ import annotations

import uuid
from types import SimpleNamespace

import pytest

import main
from domain.exceptions import TemplateGenerationError


# ---------------------------------------------------------------------------
# Fakes
# ---------------------------------------------------------------------------


class _FakeQueue:
    """Records every interaction so tests can assert exact call sequences."""

    def __init__(self, *, lock_succeeds: bool = True, enqueue_raises: bool = False) -> None:
        self.acks: list[dict] = []
        self.dlqs: list[tuple[dict, str]] = []
        self.enqueues: list[dict] = []
        self.lock_calls: list[tuple[str, int]] = []
        self.unlock_calls: list[str] = []
        self._lock_succeeds = lock_succeeds
        self._enqueue_raises = enqueue_raises

    def ack(self, payload):
        self.acks.append(payload)

    def dead_letter(self, payload, *, reason):
        self.dlqs.append((payload, reason))

    def enqueue(self, payload):
        if self._enqueue_raises:
            raise RuntimeError("boom enqueue")
        self.enqueues.append(payload)

    def acquire_lock(self, key, ttl):
        self.lock_calls.append((key, ttl))
        return self._lock_succeeds

    def release_lock(self, key):
        self.unlock_calls.append(key)


class _FakeService:
    """Stand-in for ExtractionService — drive process / template / mark_failed."""

    def __init__(
        self,
        *,
        process_raises: Exception | None = None,
        template_raises: Exception | None = None,
        mark_failed_raises: Exception | None = None,
    ) -> None:
        self._process_raises = process_raises
        self._template_raises = template_raises
        self._mark_failed_raises = mark_failed_raises
        self.process_calls: list[uuid.UUID] = []
        self.template_calls: list[tuple[uuid.UUID, bool]] = []
        self.mark_failed_calls: list[tuple[uuid.UUID, str]] = []

    def process(self, doc_id):
        self.process_calls.append(doc_id)
        if self._process_raises is not None:
            raise self._process_raises

    def generate_template_for_document(self, doc_id, *, force_regen: bool = False):
        self.template_calls.append((doc_id, force_regen))
        if self._template_raises is not None:
            raise self._template_raises

    def mark_failed(self, doc_id, *, reason):
        self.mark_failed_calls.append((doc_id, reason))
        if self._mark_failed_raises is not None:
            raise self._mark_failed_raises


def _settings():
    return SimpleNamespace(
        redis_queue="xcentro:extraction_tasks",
        redis_template_queue="xcentro:template_tasks",
    )


def _payload(**overrides):
    base = {
        "document_id": str(uuid.uuid4()),
        "_queue_name": "xcentro:extraction_tasks",
        "_raw": "{...}",
    }
    base.update(overrides)
    return base


# ---------------------------------------------------------------------------
# 1. Malformed payload — missing document_id
# ---------------------------------------------------------------------------


def test_missing_document_id_goes_to_dlq():
    queue = _FakeQueue()
    service = _FakeService()
    payload = {"_queue_name": "xcentro:extraction_tasks"}

    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "dlq_malformed"
    assert len(queue.dlqs) == 1
    assert queue.dlqs[0][1] == "missing document_id"
    # No lock acquired, no service calls.
    assert queue.lock_calls == []
    assert service.process_calls == []


# ---------------------------------------------------------------------------
# 2. Invalid UUID
# ---------------------------------------------------------------------------


def test_invalid_uuid_goes_to_dlq():
    queue = _FakeQueue()
    service = _FakeService()
    payload = _payload(document_id="not-a-uuid")

    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "dlq_bad_uuid"
    assert len(queue.dlqs) == 1
    assert "not-a-uuid" in queue.dlqs[0][1]
    assert queue.lock_calls == []


# ---------------------------------------------------------------------------
# 3. Lock busy → re-enqueue + ack, no service call
# ---------------------------------------------------------------------------


def test_lock_busy_reenqueues_without_processing():
    queue = _FakeQueue(lock_succeeds=False)
    service = _FakeService()
    payload = _payload()

    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "lock_busy"
    assert service.process_calls == []
    assert len(queue.enqueues) == 1
    # _raw is stripped from the requeued payload to avoid persisting Redis
    # internals back into the queue.
    assert "_raw" not in queue.enqueues[0]
    assert len(queue.acks) == 1
    # Lock was not held → release_lock should NOT have been called.
    assert queue.unlock_calls == []


# ---------------------------------------------------------------------------
# 4. Successful extraction
# ---------------------------------------------------------------------------


def test_successful_extraction_acks_and_releases_lock():
    queue = _FakeQueue()
    service = _FakeService()
    payload = _payload()

    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "ok"
    assert len(service.process_calls) == 1
    assert service.process_calls[0] == uuid.UUID(payload["document_id"])
    assert len(queue.acks) == 1
    assert len(queue.unlock_calls) == 1


# ---------------------------------------------------------------------------
# 5. Lock TTL matches LOCK_TTL_SECONDS
# ---------------------------------------------------------------------------


def test_lock_ttl_uses_module_constant():
    queue = _FakeQueue()
    service = _FakeService()

    main.handle_message(_payload(), queue, service, _settings())

    assert queue.lock_calls[0][1] == main.LOCK_TTL_SECONDS
    # Lock TTL must cover stale-task age — a long extraction must not lose
    # its lock before the reaper would consider the task stale.
    assert main.LOCK_TTL_SECONDS >= main.STALE_TASK_AGE_SECONDS


# ---------------------------------------------------------------------------
# 6. Template-queue routing
# ---------------------------------------------------------------------------


def test_template_queue_payload_calls_generate_template_for_document():
    queue = _FakeQueue()
    service = _FakeService()
    payload = _payload(_queue_name="xcentro:template_tasks")

    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "ok"
    assert service.process_calls == []
    assert len(service.template_calls) == 1
    assert service.template_calls[0] == (uuid.UUID(payload["document_id"]), False)


def test_template_queue_force_regen_flag_forwarded():
    queue = _FakeQueue()
    service = _FakeService()
    payload = _payload(_queue_name="xcentro:template_tasks", force_regen=True)

    main.handle_message(payload, queue, service, _settings())

    assert service.template_calls[0][1] is True


# ---------------------------------------------------------------------------
# 7. Retryable failure within budget → re-enqueue with retry_count++
# ---------------------------------------------------------------------------


def test_retryable_failure_within_budget_reenqueues_with_incremented_count():
    queue = _FakeQueue()
    service = _FakeService(process_raises=RuntimeError("transient"))
    payload = _payload(retry_count=0)

    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "retry"
    assert len(queue.enqueues) == 1
    assert queue.enqueues[0]["retry_count"] == 1
    assert "_raw" not in queue.enqueues[0]
    # Original message ack'd, no DLQ, no mark_failed.
    assert len(queue.acks) == 1
    assert queue.dlqs == []
    assert service.mark_failed_calls == []


# ---------------------------------------------------------------------------
# 8. Retries exhausted → DLQ + mark_failed (extraction queue)
# ---------------------------------------------------------------------------


def test_retries_exhausted_dlqs_and_marks_failed():
    queue = _FakeQueue()
    service = _FakeService(process_raises=RuntimeError("persistent"))
    # MAX_RETRIES=3 → final attempt is retry_count==2; one more failure exhausts.
    payload = _payload(retry_count=main.MAX_RETRIES - 1)

    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "dlq_retries_exhausted"
    assert len(queue.dlqs) == 1
    assert queue.enqueues == []
    assert len(service.mark_failed_calls) == 1


# ---------------------------------------------------------------------------
# 9. Non-retryable TemplateGenerationError → immediate DLQ; no mark_failed
#    on the template queue.
# ---------------------------------------------------------------------------


def test_non_retryable_template_error_goes_straight_to_dlq_without_mark_failed():
    queue = _FakeQueue()
    service = _FakeService(
        template_raises=TemplateGenerationError("verification_failed", retryable=False),
    )
    payload = _payload(_queue_name="xcentro:template_tasks", retry_count=0)

    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "dlq_non_retryable"
    assert len(queue.dlqs) == 1
    # Template-queue tasks must NOT be marked FAILED on the document — that
    # would clobber a perfectly-validated document just because template
    # generation failed.
    assert service.mark_failed_calls == []


def test_non_retryable_template_error_on_extraction_queue_does_mark_failed():
    """If a non-retryable error somehow surfaces on the extraction queue, the
    extraction-queue branch should still mark the document FAILED."""
    queue = _FakeQueue()
    service = _FakeService(
        process_raises=TemplateGenerationError("verification_failed", retryable=False),
    )
    payload = _payload(retry_count=0)

    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "dlq_non_retryable"
    assert len(service.mark_failed_calls) == 1


# ---------------------------------------------------------------------------
# 10. Re-enqueue itself fails → DLQ + mark_failed for extraction queue
# ---------------------------------------------------------------------------


def test_reenqueue_failure_falls_through_to_dlq():
    queue = _FakeQueue(enqueue_raises=True)
    service = _FakeService(process_raises=RuntimeError("transient"))
    payload = _payload(retry_count=0)

    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "dlq_enqueue_failed"
    assert len(queue.dlqs) == 1
    assert "enqueue failed" in queue.dlqs[0][1]
    assert len(service.mark_failed_calls) == 1


# ---------------------------------------------------------------------------
# 11. Lock always released in finally — even when the handler raises.
# ---------------------------------------------------------------------------


def test_lock_released_even_when_mark_failed_raises():
    queue = _FakeQueue()
    service = _FakeService(
        process_raises=RuntimeError("persistent"),
        mark_failed_raises=RuntimeError("mark_failed exploded"),
    )
    payload = _payload(retry_count=main.MAX_RETRIES - 1)

    # mark_failed exception is swallowed by the handler; outcome still set.
    outcome = main.handle_message(payload, queue, service, _settings())

    assert outcome == "dlq_retries_exhausted"
    assert len(queue.unlock_calls) == 1, "lock must be released in finally"

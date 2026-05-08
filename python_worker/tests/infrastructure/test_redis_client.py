"""Tests for RedisTaskQueue against fakeredis.

The queue replicates BRPOPLPUSH semantics (atomic move to an in-flight list) with
a heartbeat hash and periodic reaper. These tests cover the contracts the worker
relies on:

  - enqueue / dequeue round-trip preserves payload
  - dequeue leaves a copy in the in-flight list (crash safety)
  - reap_stale re-queues tasks whose heartbeat is older than max_age
  - heartbeat refresh keeps a task out of the reaper's claws
  - ack clears the in-flight bookkeeping
  - DLQ writes a structured record with payload + reason
  - acquire_lock / release_lock is SET-NX with TTL
"""

from __future__ import annotations

import time

import pytest

from infrastructure.redis_client import RedisTaskQueue


QUEUE = "test:extraction"
OTHER_QUEUE = "test:template_gen"


@pytest.fixture
def queue(fake_redis):
    return RedisTaskQueue.from_client(fake_redis, queues=[QUEUE, OTHER_QUEUE])


def test_enqueue_then_dequeue_round_trip(queue, fake_redis):
    queue.enqueue({"document_id": "doc-1", "retry_count": 0})

    payload = queue.dequeue(timeout=1)

    assert payload is not None
    assert payload["document_id"] == "doc-1"
    assert payload["retry_count"] == 0
    assert payload["_queue_name"] == QUEUE
    # The raw JSON must be preserved verbatim so ack() can LREM it exactly.
    assert payload["_raw"] is not None


def test_dequeue_moves_to_inflight_and_records_heartbeat(queue, fake_redis):
    queue.enqueue({"document_id": "doc-heart"})
    queue.dequeue(timeout=1)

    inflight = fake_redis.lrange(f"{QUEUE}:processing", 0, -1)
    assert len(inflight) == 1
    heartbeat = fake_redis.hgetall(f"{QUEUE}:processing:last_seen")
    assert heartbeat  # one entry keyed by raw JSON


def test_ack_removes_from_inflight_and_heartbeat(queue, fake_redis):
    queue.enqueue({"document_id": "doc-ack"})
    payload = queue.dequeue(timeout=1)

    queue.ack(payload)

    assert fake_redis.llen(f"{QUEUE}:processing") == 0
    assert fake_redis.hlen(f"{QUEUE}:processing:last_seen") == 0


def test_reap_stale_requeues_abandoned_task(queue, fake_redis):
    queue.enqueue({"document_id": "doc-abandoned"})
    queue.dequeue(timeout=1)

    # Simulate the worker crashing and never ack'ing: rewind the heartbeat
    # well past the max-age threshold.
    hb_key = f"{QUEUE}:processing:last_seen"
    (raw,) = fake_redis.hkeys(hb_key)
    fake_redis.hset(hb_key, raw, int(time.time()) - 10_000)

    recovered = queue.reap_stale(max_age_seconds=60)

    assert recovered == 1
    assert fake_redis.llen(QUEUE) == 1
    assert fake_redis.llen(f"{QUEUE}:processing") == 0
    assert fake_redis.hlen(hb_key) == 0


def test_reap_stale_leaves_fresh_heartbeat_alone(queue, fake_redis):
    queue.enqueue({"document_id": "doc-fresh"})
    queue.dequeue(timeout=1)

    recovered = queue.reap_stale(max_age_seconds=60)

    assert recovered == 0
    assert fake_redis.llen(f"{QUEUE}:processing") == 1


def test_heartbeat_refresh_protects_task_from_reaper(queue, fake_redis):
    queue.enqueue({"document_id": "doc-long"})
    payload = queue.dequeue(timeout=1)

    # Simulate an in-progress task: heartbeat was first stamped at dequeue,
    # then refreshed by the worker just before the reaper tick.
    hb_key = f"{QUEUE}:processing:last_seen"
    (raw,) = fake_redis.hkeys(hb_key)
    fake_redis.hset(hb_key, raw, int(time.time()) - 10_000)  # looks stale...
    queue.heartbeat(payload)  # ...until the worker refreshes it.

    recovered = queue.reap_stale(max_age_seconds=60)

    assert recovered == 0


def test_dead_letter_writes_payload_and_clears_inflight(queue, fake_redis):
    queue.enqueue({"document_id": "doc-dlq"})
    payload = queue.dequeue(timeout=1)

    queue.dead_letter(payload, reason="testing")

    dlq = fake_redis.lrange(f"{QUEUE}:dlq", 0, -1)
    assert len(dlq) == 1
    import json
    record = json.loads(dlq[0])
    assert record["reason"] == "testing"
    assert record["payload"]["document_id"] == "doc-dlq"
    assert "failed_at" in record
    # In-flight bookkeeping is cleared so the reaper doesn't touch it.
    assert fake_redis.llen(f"{QUEUE}:processing") == 0
    assert fake_redis.hlen(f"{QUEUE}:processing:last_seen") == 0


def test_acquire_lock_is_exclusive(queue, fake_redis):
    assert queue.acquire_lock("xcentro:doc_lock:abc", ttl=30) is True
    assert queue.acquire_lock("xcentro:doc_lock:abc", ttl=30) is False

    queue.release_lock("xcentro:doc_lock:abc")
    assert queue.acquire_lock("xcentro:doc_lock:abc", ttl=30) is True


def test_depths_returns_llen_per_queue(queue, fake_redis):
    queue.enqueue({"document_id": "a"})
    queue.enqueue({"document_id": "b"})
    queue.enqueue({"document_id": "c", "_queue_name": OTHER_QUEUE})

    depths = queue.depths()

    assert depths[QUEUE] == 2
    assert depths[OTHER_QUEUE] == 1


def test_invalid_json_payload_is_dropped_not_poisoned(queue, fake_redis):
    # Inject raw garbage directly, bypassing enqueue.
    fake_redis.lpush(QUEUE, "not-json")

    payload = queue.dequeue(timeout=1)

    assert payload is None
    # The garbage is removed from the in-flight list so the reaper doesn't
    # keep re-queueing it forever.
    assert fake_redis.llen(f"{QUEUE}:processing") == 0
    assert fake_redis.hlen(f"{QUEUE}:processing:last_seen") == 0


def test_dequeue_picks_from_either_registered_queue(queue, fake_redis):
    queue.enqueue({"document_id": "tg-1", "_queue_name": OTHER_QUEUE})

    payload = queue.dequeue(timeout=1)

    assert payload is not None
    assert payload["_queue_name"] == OTHER_QUEUE
    assert payload["document_id"] == "tg-1"


def test_heartbeat_noop_for_payload_without_bookkeeping(queue, fake_redis):
    """heartbeat() must not raise or create bookkeeping for unknown payloads.

    A payload missing `_queue_name` or `_raw` can occur if a caller forwards a
    dict that wasn't returned by dequeue(). The method should silently no-op.
    """
    queue.heartbeat({"document_id": "orphan"})  # no _queue_name, no _raw

    assert fake_redis.hlen(f"{QUEUE}:processing:last_seen") == 0
    assert fake_redis.hlen(f"{OTHER_QUEUE}:processing:last_seen") == 0


def test_ack_noop_for_payload_without_bookkeeping(queue, fake_redis):
    """ack() on a payload with no `_raw` / `_queue_name` must not blow up."""
    queue.ack({"document_id": "orphan"})

    assert fake_redis.llen(f"{QUEUE}:processing") == 0


def test_ack_second_call_on_same_payload_is_safe(queue, fake_redis):
    """Double-acking a payload (e.g., after a retry race) must not raise."""
    queue.enqueue({"document_id": "doc-double"})
    payload = queue.dequeue(timeout=1)

    queue.ack(payload)
    queue.ack(payload)  # LREM of an already-removed entry is a no-op in Redis

    assert fake_redis.llen(f"{QUEUE}:processing") == 0

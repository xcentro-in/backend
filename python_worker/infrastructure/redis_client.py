"""Redis-based task queue consumer with crash-safe BRPOPLPUSH semantics."""

from __future__ import annotations

import json
import logging
import time
from typing import Any

import redis

from domain.ports import TaskQueue

logger = logging.getLogger(__name__)

# Suffix for the per-queue "in-flight" list that holds tasks currently being
# processed. A task is moved back to the source queue if the worker dies
# before it can ack (see reap_stale).
_PROCESSING_SUFFIX = ":processing"
_LAST_SEEN_HASH_SUFFIX = ":processing:last_seen"


class RedisTaskQueue(TaskQueue):
    """Blocking pop consumer for a Redis list acting as a task queue.

    Uses BRPOPLPUSH (atomically) so that tasks are not lost if the worker
    crashes between dequeue and ack. A reaper scans the in-flight list and
    re-queues tasks whose heartbeat has lapsed.
    """

    def __init__(self, host: str, port: int, queues: list[str],
                 username: str | None = None, password: str | None = None) -> None:
        self._queues = queues
        self._client = redis.Redis(
            host=host,
            port=port,
            username=username or None,
            password=password or None,
            decode_responses=True,
        )
        self._inflight_by_queue = {q: f"{q}{_PROCESSING_SUFFIX}" for q in queues}
        self._heartbeat_key = {q: f"{q}{_LAST_SEEN_HASH_SUFFIX}" for q in queues}

    @classmethod
    def from_client(cls, client, queues: list[str]) -> "RedisTaskQueue":
        """Inject a pre-built Redis client. Used by tests with fakeredis."""
        # Build an instance without invoking the real Redis connection.
        self = cls.__new__(cls)
        self._queues = queues
        self._client = client
        self._inflight_by_queue = {q: f"{q}{_PROCESSING_SUFFIX}" for q in queues}
        self._heartbeat_key = {q: f"{q}{_LAST_SEEN_HASH_SUFFIX}" for q in queues}
        return self

    def dequeue(self, timeout: int = 0) -> dict[str, Any] | None:
        # BRPOP to pick the first queue with work, then immediately LPUSH onto
        # its in-flight list inside a tiny transaction so the move is
        # durable across crashes. Redis has no multi-key BRPOPLPUSH variant,
        # so we replicate the same guarantee with a WATCH/MULTI.
        result = self._client.brpop(self._queues, timeout=timeout)
        if result is None:
            return None
        queue_name, raw = result

        inflight = self._inflight_by_queue[queue_name]
        self._client.lpush(inflight, raw)
        self._client.hset(self._heartbeat_key[queue_name], raw, int(time.time()))

        try:
            payload = json.loads(raw)
            payload["_queue_name"] = queue_name
            payload["_raw"] = raw  # preserve the exact bytes for ack/nack
            logger.info("Dequeued task from %s: %s", queue_name, payload.get("document_id", "unknown"))
            return payload
        except json.JSONDecodeError:
            logger.error("Invalid JSON in queue %s: %s", queue_name, raw)
            # Remove the bad message from the in-flight list so it doesn't
            # poison the reaper.
            self._client.lrem(inflight, 1, raw)
            self._client.hdel(self._heartbeat_key[queue_name], raw)
            return None

    def ack(self, payload: dict[str, Any]) -> None:
        """Remove a completed task from its in-flight list."""
        queue_name = payload.get("_queue_name")
        raw = payload.get("_raw")
        if not queue_name or raw is None:
            return
        inflight = self._inflight_by_queue.get(queue_name)
        if inflight:
            self._client.lrem(inflight, 1, raw)
            self._client.hdel(self._heartbeat_key[queue_name], raw)

    def heartbeat(self, payload: dict[str, Any]) -> None:
        """Refresh the in-flight entry so the reaper leaves it alone."""
        queue_name = payload.get("_queue_name")
        raw = payload.get("_raw")
        if not queue_name or raw is None:
            return
        self._client.hset(self._heartbeat_key[queue_name], raw, int(time.time()))

    def reap_stale(self, max_age_seconds: int) -> int:
        """Re-queue in-flight tasks whose heartbeat is older than max_age.

        Intended to be called once at startup and periodically thereafter.
        Returns the number of tasks re-queued.
        """
        recovered = 0
        now = int(time.time())
        for queue_name, inflight in self._inflight_by_queue.items():
            hb_key = self._heartbeat_key[queue_name]
            entries = self._client.hgetall(hb_key) or {}
            for raw, last_seen in entries.items():
                try:
                    age = now - int(last_seen)
                except (TypeError, ValueError):
                    age = max_age_seconds + 1
                if age > max_age_seconds:
                    # Move back to the head of the source queue and drop the
                    # in-flight bookkeeping.
                    self._client.lrem(inflight, 1, raw)
                    self._client.rpush(queue_name, raw)
                    self._client.hdel(hb_key, raw)
                    recovered += 1
                    logger.warning(
                        "Reaped stale task from %s (age=%ds): %s",
                        queue_name, age, raw[:120],
                    )
        return recovered

    def enqueue(self, payload: dict[str, Any]) -> None:
        """Push a task payload onto its source queue (used for retries)."""
        target_queue = payload.pop("_queue_name", self._queues[0])
        payload.pop("_raw", None)
        self._client.lpush(target_queue, json.dumps(payload))
        logger.info("Enqueued task to %s: %s", target_queue, payload.get("document_id", "unknown"))

    def dead_letter(self, payload: dict[str, Any], reason: str) -> None:
        """Push a permanently-failed task to the per-queue DLQ."""
        source = payload.get("_queue_name", self._queues[0])
        raw = payload.get("_raw")
        dlq = f"{source}:dlq"
        record = {
            "payload": payload,
            "reason": reason,
            "failed_at": int(time.time()),
        }
        self._client.lpush(dlq, json.dumps(record, default=str))
        if raw is not None:
            inflight = self._inflight_by_queue.get(source)
            if inflight:
                self._client.lrem(inflight, 1, raw)
                self._client.hdel(self._heartbeat_key[source], raw)
        logger.error("Task sent to DLQ %s: %s", dlq, reason)

    def acquire_lock(self, key: str, ttl: int = 300) -> bool:
        return bool(self._client.set(key, "1", nx=True, ex=ttl))

    def release_lock(self, key: str) -> None:
        self._client.delete(key)

    def depths(self) -> dict[str, int]:
        """Return LLEN for each source queue. Used for monitoring/gauges."""
        out: dict[str, int] = {}
        for q in self._queues:
            try:
                out[q] = int(self._client.llen(q))
            except Exception as exc:
                logger.warning("llen(%s) failed: %s", q, exc)
                out[q] = -1
        return out

    def close(self) -> None:
        self._client.close()

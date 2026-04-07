"""Redis-based task queue consumer using redis-py."""

from __future__ import annotations

import json
import logging
from typing import Any

import redis

from domain.ports import TaskQueue

logger = logging.getLogger(__name__)


class RedisTaskQueue(TaskQueue):
    """Blocking pop consumer for a Redis list acting as a task queue."""

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

    def dequeue(self, timeout: int = 0) -> dict[str, Any] | None:
        """BRPOP from the extraction queue.

        Args:
            timeout: Seconds to block. 0 = block indefinitely.

        Returns:
            Parsed JSON payload or None on timeout.
        """
        result = self._client.brpop(self._queues, timeout=timeout)
        if result is None:
            return None

        queue_name, raw = result
        try:
            payload = json.loads(raw)
            payload["_queue_name"] = queue_name
            logger.info("Dequeued task from %s: %s", queue_name, payload.get("document_id", "unknown"))
            return payload
        except json.JSONDecodeError:
            logger.error("Invalid JSON in queue %s: %s", queue_name, raw)
            return None

    def enqueue(self, payload: dict[str, Any]) -> None:
        """Push a task payload onto the primary queue (used for retries)."""
        # Always push to the first queue for retries by default
        target_queue = payload.pop("_queue_name", self._queues[0])
        self._client.lpush(target_queue, json.dumps(payload))
        logger.info("Enqueued task to %s: %s", target_queue, payload.get("document_id", "unknown"))

    def acquire_lock(self, key: str, ttl: int = 300) -> bool:
        """Atomically acquire a distributed lock using SET NX EX.

        Args:
            key: Lock key (e.g. 'xcentro:doc_lock:{document_id}').
            ttl: Expiry in seconds. Prevents deadlock if the worker crashes.

        Returns:
            True if the lock was acquired, False if another worker holds it.
        """
        return bool(self._client.set(key, "1", nx=True, ex=ttl))

    def release_lock(self, key: str) -> None:
        """Release a previously acquired lock by deleting the key."""
        self._client.delete(key)

    def close(self) -> None:
        self._client.close()

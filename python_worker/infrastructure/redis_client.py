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

    def __init__(self, host: str, port: int, queue_name: str,
                 username: str | None = None, password: str | None = None) -> None:
        self._queue = queue_name
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
        result = self._client.brpop(self._queue, timeout=timeout)
        if result is None:
            return None

        _, raw = result
        try:
            payload = json.loads(raw)
            logger.info("Dequeued task: %s", payload.get("document_id", "unknown"))
            return payload
        except json.JSONDecodeError:
            logger.error("Invalid JSON in queue: %s", raw)
            return None

    def enqueue(self, payload: dict[str, Any]) -> None:
        """Push a task payload onto the queue (used for retries)."""
        self._client.lpush(self._queue, json.dumps(payload))
        logger.info("Enqueued task: %s", payload.get("document_id", "unknown"))

    def close(self) -> None:
        self._client.close()

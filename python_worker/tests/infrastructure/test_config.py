"""Tests for the Settings loader.

Settings is a pydantic BaseSettings — defaults come from the class, env vars
override, and invalid casts raise. These tests pin the contract so refactors
that silently break env parsing don't go unnoticed.
"""

from __future__ import annotations

import pytest


_ENV_KEYS = [
    "POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD",
    "REDIS_HOST", "REDIS_PORT", "REDIS_USER", "REDIS_PASSWORD",
    "REDIS_QUEUE", "REDIS_TEMPLATE_QUEUE",
    "GEMINI_API_KEY", "GEMINI_MODEL",
    "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION", "S3_BUCKET_NAME",
    "TEMP_DIR", "MAX_DOWNLOAD_BYTES", "I2D_TEMPLATE_DIR",
    "WORKER_POOL_SIZE", "GEMINI_MAX_CONCURRENT", "SHUTDOWN_GRACE_SECONDS",
]


@pytest.fixture(autouse=True)
def _isolated_env(monkeypatch, tmp_path):
    """Ensure each test starts with a clean env and no .env file bleeds in."""
    for key in _ENV_KEYS:
        monkeypatch.delenv(key, raising=False)
        monkeypatch.delenv(key.lower(), raising=False)
    # Move cwd so the production .env (if any) is not auto-discovered.
    monkeypatch.chdir(tmp_path)


def test_defaults_when_env_unset():
    from infrastructure.config import Settings

    s = Settings()

    assert s.postgres_host == "localhost"
    assert s.postgres_port == 5432
    assert s.postgres_db == "xcentro"
    assert s.redis_host == "localhost"
    assert s.redis_port == 6379
    assert s.redis_queue == "xcentro:extraction_tasks"
    assert s.redis_template_queue == "xcentro:template_generation_tasks"
    assert s.gemini_model == "gemini-2.5-flash"
    assert s.aws_region == "ap-south-1"
    assert s.max_download_bytes == 50 * 1024 * 1024
    assert s.i2d_template_dir == "/app/templates"
    assert s.worker_pool_size == 4
    assert s.gemini_max_concurrent == 5
    assert s.shutdown_grace_seconds == 60


def test_env_overrides_are_applied(monkeypatch):
    from infrastructure.config import Settings

    monkeypatch.setenv("WORKER_POOL_SIZE", "12")
    monkeypatch.setenv("GEMINI_MAX_CONCURRENT", "7")
    monkeypatch.setenv("GEMINI_MODEL", "gemini-2.5-pro")
    monkeypatch.setenv("REDIS_PORT", "6380")
    monkeypatch.setenv("SHUTDOWN_GRACE_SECONDS", "30")

    s = Settings()

    assert s.worker_pool_size == 12
    assert s.gemini_max_concurrent == 7
    assert s.gemini_model == "gemini-2.5-pro"
    assert s.redis_port == 6380
    assert s.shutdown_grace_seconds == 30


def test_int_fields_reject_garbage(monkeypatch):
    """A non-numeric int value must fail fast, not silently fall back."""
    from infrastructure.config import Settings
    from pydantic import ValidationError

    monkeypatch.setenv("WORKER_POOL_SIZE", "not-a-number")

    with pytest.raises(ValidationError):
        Settings()

"""Application configuration loaded from environment variables."""

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Centralised config — populated from env vars or .env file."""

    # PostgreSQL
    postgres_host: str = "localhost"
    postgres_port: int = 5432
    postgres_db: str = "xcentro"
    postgres_user: str = "xcentro_rw"
    postgres_password: str = ""

    # Redis
    redis_host: str = "localhost"
    redis_port: int = 6379
    redis_user: str = "xcentro_redis"
    redis_password: str = ""
    redis_queue: str = "xcentro:extraction_tasks"
    redis_template_queue: str = "xcentro:template_generation_tasks"

    # Gemini LLM
    gemini_api_key: str = ""
    gemini_model: str = "gemini-2.5-flash"
    gemini_fallback_model: str = ""  # e.g. "gemini-2.0-flash-lite"; empty = no fallback

    # AWS S3
    aws_access_key_id: str = ""
    aws_secret_access_key: str = ""
    aws_region: str = "ap-south-1"
    s3_bucket_name: str = ""

    # File handling
    # Defaults to a per-run private tempdir; override via TEMP_DIR env var.
    temp_dir: str = ""
    # Upload size cap shared by the Go uploader and the worker.
    max_download_bytes: int = 50 * 1024 * 1024  # 50 MB

    # invoice2data template directory (mounted volume)
    i2d_template_dir: str = "/app/templates"

    # Concurrency
    # worker_pool_size: per-process pool of threads that process tasks in parallel.
    # Each holds a Redis in-flight slot via the existing per-doc lock, so scale-out
    # across multiple worker processes is still safe with the same value.
    worker_pool_size: int = 4
    # gemini_max_concurrent: global cap on simultaneous Gemini API calls inside this
    # process. Set below the provider's burst limit to avoid 429s. LLM-bound workers
    # will block on this semaphore; throughput is still bounded by the minimum of
    # (worker_pool_size, gemini_max_concurrent) for LLM-path documents.
    gemini_max_concurrent: int = 5
    # Seconds to wait for in-flight futures when shutting down gracefully.
    shutdown_grace_seconds: int = 60

    # template_gen_candidates: when > 1, the template generator will sample
    # multiple LLM candidates per document and pick the one that scores best
    # against verification + perturbation + history checks. K=1 preserves the
    # historical behaviour and cost; raise to 2-3 for vendors with high doc
    # volume where extra LLM cost is justified by template quality.
    template_gen_candidates: int = 1

    model_config = {"env_file": ".env", "env_file_encoding": "utf-8"}

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

    # Gemini LLM
    gemini_api_key: str = ""
    gemini_model: str = "gemini-2.5-flash"

    # AWS S3
    aws_access_key_id: str = ""
    aws_secret_access_key: str = ""
    aws_region: str = "ap-south-1"
    s3_bucket_name: str = ""

    # File handling
    temp_dir: str = "/tmp/extraction"

    # invoice2data template directory (mounted volume)
    i2d_template_dir: str = "/app/templates"

    model_config = {"env_file": ".env", "env_file_encoding": "utf-8"}

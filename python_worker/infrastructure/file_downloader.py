"""File downloader — downloads from S3 using boto3 presigned URLs."""

from __future__ import annotations

import logging
import os

from botocore.client import BaseClient

from domain.ports import FileDownloader

logger = logging.getLogger(__name__)

_DEFAULT_MAX_BYTES = 50 * 1024 * 1024  # 50 MB


class S3FileDownloader(FileDownloader):
    """Downloads files directly from S3 via boto3 with a hard size cap.

    The download is streamed in chunks and aborted if the cumulative byte
    count exceeds `max_bytes`. This protects the worker host from a
    zip-bomb-style exhaustion attack via a maliciously-sized S3 object.
    """

    _CHUNK = 64 * 1024

    def __init__(
        self,
        s3_client: BaseClient,
        bucket_name: str,
        max_bytes: int = _DEFAULT_MAX_BYTES,
    ) -> None:
        self._s3 = s3_client
        self._bucket = bucket_name
        self._max_bytes = max_bytes

    def download(self, s3_path: str, dest_dir: str) -> str:
        os.makedirs(dest_dir, exist_ok=True)
        filename = os.path.basename(s3_path) or "document"
        dest_path = os.path.join(dest_dir, filename)

        # Pre-flight HEAD to reject oversized objects without pulling bytes.
        head = self._s3.head_object(Bucket=self._bucket, Key=s3_path)
        content_length = int(head.get("ContentLength", 0))
        if content_length > self._max_bytes:
            raise ValueError(
                f"s3://{self._bucket}/{s3_path} is {content_length} bytes, "
                f"exceeds cap {self._max_bytes}"
            )

        resp = self._s3.get_object(Bucket=self._bucket, Key=s3_path)
        body = resp["Body"]
        written = 0
        try:
            with open(dest_path, "wb") as fh:
                while True:
                    chunk = body.read(self._CHUNK)
                    if not chunk:
                        break
                    written += len(chunk)
                    if written > self._max_bytes:
                        fh.close()
                        os.remove(dest_path)
                        raise ValueError(
                            f"Download exceeded cap {self._max_bytes} bytes"
                        )
                    fh.write(chunk)
        finally:
            body.close()

        logger.info(
            "Downloaded s3://%s/%s → %s (%d bytes)",
            self._bucket, s3_path, dest_path, written,
        )
        return dest_path

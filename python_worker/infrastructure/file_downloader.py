"""File downloader — downloads from S3 using boto3 presigned URLs."""

from __future__ import annotations

import logging
import os
import urllib.request
from urllib.parse import urlparse

import boto3
from botocore.client import BaseClient

from domain.ports import FileDownloader

logger = logging.getLogger(__name__)


class HTTPFileDownloader(FileDownloader):
    """Downloads files from an HTTP/HTTPS URL (e.g. S3 pre-signed URL).

    Kept for backward-compatibility or testing with direct URLs.
    """

    def download(self, url: str, dest_dir: str) -> str:
        os.makedirs(dest_dir, exist_ok=True)

        parsed = urlparse(url)
        filename = os.path.basename(parsed.path) or "document"
        dest_path = os.path.join(dest_dir, filename)

        logger.info("Downloading %s → %s", url, dest_path)
        urllib.request.urlretrieve(url, dest_path)

        return dest_path


class S3FileDownloader(FileDownloader):
    """Downloads files from S3 by generating a presigned URL from the object key.

    The `download` method receives an S3 object key (s3_path), generates a
    time-limited presigned GET URL, and downloads the file to a local path.
    """

    def __init__(self, s3_client: BaseClient, bucket_name: str, expiry: int = 3600) -> None:
        self._s3 = s3_client
        self._bucket = bucket_name
        self._expiry = expiry

    def download(self, s3_path: str, dest_dir: str) -> str:
        os.makedirs(dest_dir, exist_ok=True)

        filename = os.path.basename(s3_path) or "document"
        dest_path = os.path.join(dest_dir, filename)

        presigned_url = self._s3.generate_presigned_url(
            "get_object",
            Params={"Bucket": self._bucket, "Key": s3_path},
            ExpiresIn=self._expiry,
        )
        logger.info("Generated presigned URL for s3://%s/%s (expires in %ds)", self._bucket, s3_path, self._expiry)

        urllib.request.urlretrieve(presigned_url, dest_path)
        logger.info("Downloaded s3://%s/%s → %s", self._bucket, s3_path, dest_path)

        return dest_path

"""Tests for S3FileDownloader.

Uses MagicMock in place of a real boto3 S3 client — the contract is narrow
(head_object + get_object returning a Body with .read and .close). We verify:

  - Happy path writes the file and returns the path.
  - HEAD pre-flight rejects oversized objects before any byte transfer.
  - Streaming cap kicks in if the body under-reports Content-Length.
  - Destination dir is created if missing.
  - S3 keys with empty basename default to "document".
  - Body.close() is called on success and on failure.
"""

from __future__ import annotations

import io
from unittest.mock import MagicMock

import pytest

from infrastructure.file_downloader import S3FileDownloader


class _RecordingBody:
    """Minimal S3 StreamingBody replacement that tracks close() invocations."""

    def __init__(self, data: bytes) -> None:
        self._buf = io.BytesIO(data)
        self.closed = False

    def read(self, size: int = -1) -> bytes:
        return self._buf.read(size)

    def close(self) -> None:
        self.closed = True


def _make_s3(*, content_length: int, body: _RecordingBody) -> MagicMock:
    client = MagicMock()
    client.head_object.return_value = {"ContentLength": content_length}
    client.get_object.return_value = {"Body": body}
    return client


def test_download_writes_file_and_returns_path(tmp_path):
    data = b"x" * 100
    body = _RecordingBody(data)
    client = _make_s3(content_length=100, body=body)
    dl = S3FileDownloader(client, bucket_name="bucket", max_bytes=1024)

    path = dl.download("uploads/doc.pdf", str(tmp_path))

    assert path == str(tmp_path / "doc.pdf")
    with open(path, "rb") as fh:
        assert fh.read() == data
    assert body.closed is True


def test_head_preflight_rejects_oversized(tmp_path):
    body = _RecordingBody(b"")
    client = _make_s3(content_length=10_000, body=body)
    dl = S3FileDownloader(client, bucket_name="bucket", max_bytes=1024)

    with pytest.raises(ValueError, match="exceeds cap"):
        dl.download("uploads/big.pdf", str(tmp_path))

    # get_object must never be called when HEAD already rejects the object.
    client.get_object.assert_not_called()


def test_streaming_cap_rejects_under_reported_size(tmp_path):
    # HEAD lies about the size — body actually delivers more bytes than cap.
    data = b"x" * 2000
    body = _RecordingBody(data)
    client = _make_s3(content_length=100, body=body)
    dl = S3FileDownloader(client, bucket_name="bucket", max_bytes=1024)

    with pytest.raises(ValueError, match="exceeded cap"):
        dl.download("uploads/lying.pdf", str(tmp_path))

    # Partial file must be removed on abort.
    assert not (tmp_path / "lying.pdf").exists()
    assert body.closed is True


def test_basename_fallback_to_document(tmp_path):
    body = _RecordingBody(b"hi")
    client = _make_s3(content_length=2, body=body)
    dl = S3FileDownloader(client, bucket_name="bucket", max_bytes=1024)

    path = dl.download("folder/", str(tmp_path))

    assert path == str(tmp_path / "document")
    assert (tmp_path / "document").exists()


def test_dest_dir_created_if_missing(tmp_path):
    nested = tmp_path / "nested" / "deeper"
    assert not nested.exists()
    body = _RecordingBody(b"hi")
    client = _make_s3(content_length=2, body=body)
    dl = S3FileDownloader(client, bucket_name="bucket", max_bytes=1024)

    dl.download("uploads/x.pdf", str(nested))

    assert nested.is_dir()
    assert (nested / "x.pdf").exists()


def test_body_closed_on_success_and_failure(tmp_path):
    # Success.
    body_ok = _RecordingBody(b"ok")
    client_ok = _make_s3(content_length=2, body=body_ok)
    S3FileDownloader(client_ok, "bucket", max_bytes=1024).download("a/ok.pdf", str(tmp_path))
    assert body_ok.closed is True

    # Failure via streaming cap — body must still be closed in the `finally`.
    body_fail = _RecordingBody(b"x" * 2000)
    client_fail = _make_s3(content_length=100, body=body_fail)
    dl = S3FileDownloader(client_fail, "bucket", max_bytes=1024)
    with pytest.raises(ValueError):
        dl.download("a/fail.pdf", str(tmp_path))
    assert body_fail.closed is True

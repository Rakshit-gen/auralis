"""S3-compatible object storage (MinIO locally, R2 in production)."""

from __future__ import annotations

import mimetypes
import os

import structlog
from minio import Minio

log = structlog.get_logger()


class ObjectStore:
    def __init__(self, endpoint: str, access_key: str, secret_key: str, bucket: str, secure: bool, region: str):
        self._client = Minio(endpoint, access_key=access_key, secret_key=secret_key, secure=secure, region=region)
        self._bucket = bucket
        if not self._client.bucket_exists(bucket):
            self._client.make_bucket(bucket, location=region)

    def put_file(self, key: str, path: str, content_type: str | None = None) -> None:
        ct = content_type or mimetypes.guess_type(path)[0] or "application/octet-stream"
        self._client.fput_object(self._bucket, key, path, content_type=ct)

    def put_bytes(self, key: str, data: bytes, content_type: str) -> None:
        import io

        self._client.put_object(self._bucket, key, io.BytesIO(data), length=len(data), content_type=content_type)

    def upload_dir(self, local_dir: str, key_prefix: str) -> int:
        """Upload every file under local_dir to key_prefix/<relpath>. Returns count."""
        count = 0
        for root, _dirs, files in os.walk(local_dir):
            for name in files:
                full = os.path.join(root, name)
                rel = os.path.relpath(full, local_dir)
                ct = (
                    "application/vnd.apple.mpegurl"
                    if name.endswith(".m3u8")
                    else ("video/mp2t" if name.endswith(".ts") else None)
                )
                self.put_file(f"{key_prefix}/{rel}", full, ct)
                count += 1
        return count

    def ping(self) -> None:
        self._client.bucket_exists(self._bucket)

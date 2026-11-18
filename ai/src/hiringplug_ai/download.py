"""Fetch a document from the signed link in a job (never with storage credentials)."""

from __future__ import annotations

import httpx


class DownloadError(RuntimeError):
    """Temporary failure: the job is retried."""


class FileTooLargeError(DownloadError):
    """The file is larger than the worker may read: trying again cannot help,
    so the job is given up at once (see worker.process)."""


def download(url: str, max_bytes: int, timeout_seconds: float = 30.0) -> bytes:
    try:
        with httpx.stream("GET", url, timeout=timeout_seconds, follow_redirects=False) as response:
            if response.status_code != httpx.codes.OK:
                raise DownloadError(f"download failed with HTTP {response.status_code}")
            chunks: list[bytes] = []
            size = 0
            for chunk in response.iter_bytes():
                size += len(chunk)
                if size > max_bytes:
                    raise FileTooLargeError(f"file is larger than {max_bytes} bytes")
                chunks.append(chunk)
            return b"".join(chunks)
    except httpx.HTTPError as exc:
        raise DownloadError(f"download failed: {exc}") from exc

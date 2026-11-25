"""Fetch a document from the signed link in a job (never with storage credentials).

Messages never carry the link: it is signed (anyone holding it could fetch
the CV until it expires), and they end up in logs and in the job's error.
"""

from __future__ import annotations

import httpx


class DownloadError(RuntimeError):
    """Temporary failure (the server busy or unreachable): the job is retried."""


class GiveUpError(DownloadError):
    """Trying again cannot help: the job is given up at once (see
    worker.process), and the app shows why with a way to start again."""


class FileTooLargeError(GiveUpError):
    """The file is larger than the worker may read."""


class DownloadGoneError(GiveUpError):
    """The link points to no file (404, 410: deleted, or not on this server)
    or no longer works (403: expired, or signed by another server)."""


# What the API's answer means for trying again.
GONE = {httpx.codes.FORBIDDEN, httpx.codes.NOT_FOUND, httpx.codes.GONE}


def download(url: str, max_bytes: int, timeout_seconds: float = 30.0) -> bytes:
    try:
        with httpx.stream("GET", url, timeout=timeout_seconds, follow_redirects=False) as response:
            status = response.status_code
            if status in GONE:
                raise DownloadGoneError(
                    f"download failed with HTTP {status}: the file is gone or its link no longer works"
                )
            if status != httpx.codes.OK:
                raise DownloadError(f"download failed with HTTP {status}")
            chunks: list[bytes] = []
            size = 0
            for chunk in response.iter_bytes():
                size += len(chunk)
                if size > max_bytes:
                    raise FileTooLargeError(f"file is larger than {max_bytes} bytes")
                chunks.append(chunk)
            return b"".join(chunks)
    except httpx.HTTPError as exc:
        # The exception's own text may hold the link: only its kind is kept.
        raise DownloadError(f"download failed: {type(exc).__name__}") from None

"""Fetch a document from the signed link in a job (never with storage credentials).

Messages never carry the link: it is signed (anyone holding it could fetch
the CV until it expires), and they end up in logs and in the job's error.
Each says what to do, as the app shows it: the Go worker reads "file
missing" and "link expired" in a job's last error (documents/reading.go).
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
    """This link will never give the file: missing, or the link expired."""


class FileMissingError(DownloadGoneError):
    """HiringPlug's file server says the file is not there (its own 404,
    marked by MISSING_HEADER): deleted, or never copied to this server.
    Uploading it again is the way back."""


class LinkExpiredError(DownloadGoneError):
    """The file server refused the link (403): expired, or signed by another
    server (a new HP_FILES_URL_SECRET). This payload's link never works
    again; reading the file again signs a new one."""


# The header HiringPlug's file server sets on its own "not found"
# (platform/blob: blob.MissingHeader). A 404 without it is not about the
# file: something else answered (a wrong HP_INTERNAL_BASE_URL, a proxy),
# a setting to fix, so it is tried again.
MISSING_HEADER = "x-hyplug-file"


def download(url: str, max_bytes: int, timeout_seconds: float = 30.0) -> bytes:
    try:
        with httpx.stream("GET", url, timeout=timeout_seconds, follow_redirects=False) as response:
            status = response.status_code
            if status == httpx.codes.NOT_FOUND and response.headers.get(MISSING_HEADER) == "missing":
                raise FileMissingError(
                    f"file missing (HTTP {status}): it is not on the server any more; upload it again"
                )
            if status == httpx.codes.FORBIDDEN:
                raise LinkExpiredError(
                    f"link expired (HTTP {status}): the file's link no longer works; read it again"
                )
            if status in (httpx.codes.NOT_FOUND, httpx.codes.GONE):
                raise DownloadError(
                    f"download failed: answer {status} from something other than HiringPlug's file server"
                    " (check HP_INTERNAL_BASE_URL)"
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

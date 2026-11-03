"""Converting documents with LibreOffice, in a private profile per run and
its own process group, killed (with its helper process) on a time limit.

Used to read old .doc files (converted to .docx) and to make PDFs of
formatted CVs.
"""

from __future__ import annotations

import contextlib
import logging
import os
import shutil
import signal
import subprocess
from pathlib import Path

log = logging.getLogger(__name__)

TIMEOUT_SECONDS = 90


def available() -> bool:
    return bool(shutil.which("soffice") or shutil.which("libreoffice"))


def convert(path: Path, tmp: Path, to: str) -> bytes | None:
    """Convert the file at path to format `to` (e.g. "docx", "pdf"); None when
    LibreOffice is missing, times out or fails. tmp is a private folder."""
    exe = shutil.which("soffice") or shutil.which("libreoffice")
    if not exe:
        return None
    out = tmp / "out"
    # A private profile per run, so conversions can run side by side and never
    # depend on (or change) a shared LibreOffice profile.
    profile = (tmp / "profile").as_uri()
    args = [exe, "--headless", "--norestore", f"-env:UserInstallation={profile}"]
    args += ["--convert-to", to, "--outdir", str(out), str(path)]
    # Its own process group: `soffice` is a wrapper around soffice.bin, and a
    # time-out must stop both, or a hung conversion keeps running in the worker.
    with subprocess.Popen(  # noqa: S603 - fixed program and arguments, our own temporary files
        args,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        env={**os.environ, "HOME": str(tmp)},
        start_new_session=True,
    ) as proc:
        try:
            proc.wait(timeout=TIMEOUT_SECONDS)
        except subprocess.TimeoutExpired:
            log.warning("libreoffice timed out")
            _kill_group(proc)
            return None
    result = out / f"{path.stem}.{to}"
    return result.read_bytes() if result.exists() else None


def _kill_group(proc: subprocess.Popen[bytes]) -> None:
    with contextlib.suppress(ProcessLookupError):  # already gone
        os.killpg(proc.pid, signal.SIGKILL)
    proc.wait()

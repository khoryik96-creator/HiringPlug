"""Old Word (.doc) files.

antiword first: small, fast, and enough for most files. Some .doc files it
cannot read ("is not a Word Document": other Word versions, files saved by
other programs); LibreOffice converts those to .docx, which the Word reader
then reads like any other, headers, tables and text boxes included. Each tool
runs in its own temporary folder with a time limit; missing tools are skipped.
"""

from __future__ import annotations

import contextlib
import logging
import os
import re
import shutil
import signal
import subprocess
import tempfile
from dataclasses import replace
from pathlib import Path

from .docx_reader import extract_docx
from .limits import DocumentRejectedError, Limits
from .result import Extraction
from .text import clean_text

log = logging.getLogger(__name__)

ANTIWORD_TIMEOUT_SECONDS = 30
LIBREOFFICE_TIMEOUT_SECONDS = 90
_PICTURE = re.compile(r"\[pic\]")


def extract_doc(data: bytes, limits: Limits) -> Extraction:
    with tempfile.TemporaryDirectory(prefix="hp-doc-") as tmp:
        path = Path(tmp) / "cv.doc"
        path.write_bytes(data)
        text = _antiword(path)
        if text and text.strip():
            return Extraction("extracted", clean_text(_PICTURE.sub("", text)), None, "antiword@0.37")
        converted = _libreoffice_to_docx(path, Path(tmp))
        if converted is not None:
            result = extract_docx(converted, limits)
            return replace(result, extractor=f"libreoffice+{result.extractor}")
    raise DocumentRejectedError(
        "This old Word (.doc) file could not be read. "
        "Open it in Word, save it as .docx or PDF, and upload that."
    )


def _antiword(path: Path) -> str | None:
    exe = shutil.which("antiword")
    if not exe:
        return None
    try:
        # -w 0: one line per paragraph, as the other readers produce.
        done = subprocess.run(  # noqa: S603 - fixed program, our own temporary file
            [exe, "-w", "0", str(path)], capture_output=True, timeout=ANTIWORD_TIMEOUT_SECONDS, check=False
        )
    except subprocess.TimeoutExpired:
        log.warning("antiword timed out")
        return None
    if done.returncode != 0:
        return None
    return done.stdout.decode("utf-8", errors="replace")


def _libreoffice_to_docx(path: Path, tmp: Path) -> bytes | None:
    exe = shutil.which("soffice") or shutil.which("libreoffice")
    if not exe:
        return None
    out = tmp / "out"
    # A private profile per run, so conversions can run side by side and never
    # depend on (or change) a shared LibreOffice profile.
    profile = (tmp / "profile").as_uri()
    args = [exe, "--headless", "--norestore", f"-env:UserInstallation={profile}"]
    args += ["--convert-to", "docx", "--outdir", str(out), str(path)]
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
            proc.wait(timeout=LIBREOFFICE_TIMEOUT_SECONDS)
        except subprocess.TimeoutExpired:
            log.warning("libreoffice timed out")
            _kill_group(proc)
            return None
    result = out / "cv.docx"
    return result.read_bytes() if result.exists() else None


def _kill_group(proc: subprocess.Popen[bytes]) -> None:
    with contextlib.suppress(ProcessLookupError):  # already gone
        os.killpg(proc.pid, signal.SIGKILL)
    proc.wait()

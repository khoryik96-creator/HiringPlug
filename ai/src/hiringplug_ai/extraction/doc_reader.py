"""Old Word (.doc) files.

antiword first: small, fast, and enough for most files. Some .doc files it
cannot read ("is not a Word Document": other Word versions, files saved by
other programs); LibreOffice converts those to .docx, which the Word reader
then reads like any other, headers, tables and text boxes included. Each tool
runs in its own temporary folder with a time limit; missing tools are skipped.
"""

from __future__ import annotations

import logging
import re
import shutil
import subprocess
import tempfile
from dataclasses import replace
from pathlib import Path

from .. import office
from .docx_reader import extract_docx
from .limits import DocumentRejectedError, Limits
from .result import Extraction
from .text import clean_text

log = logging.getLogger(__name__)

ANTIWORD_TIMEOUT_SECONDS = 30
_PICTURE = re.compile(r"\[pic\]")


def extract_doc(data: bytes, limits: Limits) -> Extraction:
    with tempfile.TemporaryDirectory(prefix="hp-doc-") as tmp:
        path = Path(tmp) / "cv.doc"
        path.write_bytes(data)
        text = _antiword(path)
        if text and text.strip():
            return Extraction("extracted", clean_text(_PICTURE.sub("", text)), None, "antiword@0.37")
        converted = office.convert(path, Path(tmp), "docx")
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

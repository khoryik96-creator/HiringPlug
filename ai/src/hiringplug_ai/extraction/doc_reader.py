"""Old Word (.doc) files.

antiword first: small, fast, and enough for most files. Some .doc files it
cannot read ("is not a Word Document": other Word versions, files saved by
other programs); LibreOffice converts those to .docx, which the Word reader
then reads like any other, headers, tables and text boxes included. antiword
also leaves out a page's header and text boxes, where many CVs put the name,
email and phone: when its text has no email address, LibreOffice's is used
if it has one. Each tool runs in its own temporary folder with a time limit;
missing tools are skipped.
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
_EMAIL = re.compile(r"[\w.+-]+@[\w-]+\.[\w.-]+")


def extract_doc(data: bytes, limits: Limits) -> Extraction:
    with tempfile.TemporaryDirectory(prefix="hp-doc-") as tmp:
        path = Path(tmp) / "cv.doc"
        path.write_bytes(data)
        text = _antiword(path)
        read = (
            Extraction("extracted", clean_text(_PICTURE.sub("", text)), None, "antiword@0.37")
            if text and text.strip()
            else None
        )
        if read is not None and _EMAIL.search(read.text or ""):
            return read
        converted = office.convert(path, Path(tmp), "docx")
        if converted is not None:
            result = extract_docx(converted, limits)
            if read is None or (result.status == "extracted" and _EMAIL.search(result.text or "")):
                return replace(result, extractor=f"libreoffice+{result.extractor}")
        if read is not None:
            return read
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

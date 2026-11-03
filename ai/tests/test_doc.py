"""Old Word (.doc) files: antiword, and LibreOffice when antiword cannot read the file.

The fixture was made from invented text (Word -> "Word 97" in LibreOffice).
"""

from __future__ import annotations

import shutil
import time
from pathlib import Path

import pytest

from hiringplug_ai import office
from hiringplug_ai.extraction import extract

DOC = (Path(__file__).parent / "fixtures" / "old-word.doc").read_bytes()


def test_old_word_files_are_read_with_antiword() -> None:
    result = extract(DOC, ocr=None)
    assert result.status == "extracted", result.error
    assert result.extractor.startswith("antiword")
    assert result.text
    assert "Ahmad Rizal bin Hassan" in result.text
    assert "[pic]" not in result.text


def test_libreoffice_reads_what_antiword_cannot(monkeypatch: pytest.MonkeyPatch) -> None:
    real_which = shutil.which
    monkeypatch.setattr(shutil, "which", lambda name: None if name == "antiword" else real_which(name))
    result = extract(DOC, ocr=None)
    assert result.status == "extracted", result.error
    assert result.extractor.startswith("libreoffice+python-docx")
    assert result.text
    assert "Network Engineer | Contoh Teknologi Sdn Bhd" in result.text


def test_other_office_files_are_refused_with_advice(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(shutil, "which", lambda name: None)  # neither tool can read it
    result = extract(DOC, ocr=None)
    assert result.status == "failed"
    assert result.error
    assert "save it as .docx or PDF" in result.error


def test_a_hung_libreoffice_is_stopped_with_its_children(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    # A stand-in "soffice" that starts a child (like soffice.bin) and hangs.
    fake = tmp_path / "soffice"
    marker = tmp_path / "child.pid"
    fake.write_text(f"#!/bin/sh\nsleep 300 &\necho $! > {marker}\nwait\n")
    fake.chmod(0o755)
    monkeypatch.setattr(office, "TIMEOUT_SECONDS", 1)
    monkeypatch.setattr(shutil, "which", lambda name: str(fake) if name == "soffice" else None)
    result = extract(DOC, ocr=None)
    assert result.status == "failed"
    stat = Path(f"/proc/{int(marker.read_text())}/stat")
    time.sleep(0.2)
    # The child was stopped too: gone, or a zombie waiting to be reaped.
    assert not stat.exists() or stat.read_text().split(") ", 1)[1].startswith("Z")

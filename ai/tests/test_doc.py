"""Old Word (.doc) files: antiword, and LibreOffice when antiword cannot read the file.

The fixture was made from invented text (Word -> "Word 97" in LibreOffice).
"""

from __future__ import annotations

import shutil
from pathlib import Path

import pytest

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

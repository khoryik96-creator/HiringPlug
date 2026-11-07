"""document.render_pdf: formatted CVs as PDFs, with real LibreOffice."""

from __future__ import annotations

import base64
import io
from pathlib import Path

import docx
import pytest

from hiringplug_ai import office
from hiringplug_ai.contracts import DocumentPdfRenderedJob, DocumentRenderPdfJob, JobKind
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, render_pdf

DOC_ID = "doc_01jbz6w0m0k8q9x2r3s4t5v6w7"


def a_docx() -> bytes:
    d = docx.Document()
    d.add_heading("Siti Aisyah", level=1)
    d.add_paragraph("Senior Accountant")
    buf = io.BytesIO()
    d.save(buf)
    return buf.getvalue()


def deps_for(data: bytes) -> Deps:
    return Deps(
        download=lambda _url, _max: data, ocr=None, limits=DEFAULT_LIMITS, max_download_bytes=10_000_000
    )


def payload() -> DocumentRenderPdfJob:
    return DocumentRenderPdfJob.model_validate(
        {"documentId": DOC_ID, "downloadUrl": "http://api.test/internal/files/x?sig=1"}
    )


def test_a_formatted_cv_becomes_a_pdf() -> None:
    assert office.available(), "LibreOffice is needed to make PDFs (see AGENTS.md)"
    [follow_up] = render_pdf.run(payload(), deps_for(a_docx()))
    assert follow_up.queue == "core"
    assert follow_up.kind == JobKind.document_pdf_rendered
    result = follow_up.payload
    assert isinstance(result, DocumentPdfRenderedJob)
    assert result.error is None
    assert result.pdfBase64 is not None
    assert base64.b64decode(result.pdfBase64).startswith(b"%PDF-")


def test_an_old_word_doc_becomes_a_pdf_too() -> None:
    """CVs sent as old Word .doc files are previewed as well."""
    data = (Path(__file__).parent / "fixtures" / "old-word.doc").read_bytes()
    [follow_up] = render_pdf.run(payload(), deps_for(data))
    result = follow_up.payload
    assert isinstance(result, DocumentPdfRenderedJob)
    assert result.error is None
    assert result.pdfBase64 is not None
    assert base64.b64decode(result.pdfBase64).startswith(b"%PDF-")


def test_without_libreoffice_the_reason_goes_back(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(office, "available", lambda: False)
    [follow_up] = render_pdf.run(payload(), deps_for(a_docx()))
    result = follow_up.payload
    assert isinstance(result, DocumentPdfRenderedJob)
    assert result.pdfBase64 is None
    assert result.error is not None
    assert "LibreOffice" in result.error


def test_any_file_gets_an_answer_not_a_crash() -> None:
    [follow_up] = render_pdf.run(payload(), deps_for(b"not a word file"))
    result = follow_up.payload
    assert isinstance(result, DocumentPdfRenderedJob)
    assert (result.pdfBase64 is None) != (result.error is None), "a PDF, or why not"

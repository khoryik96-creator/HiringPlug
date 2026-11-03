"""document.render_pdf: turn a formatted CV (.docx) into a PDF for Go to save."""

from __future__ import annotations

import base64
import logging
import tempfile
from pathlib import Path
from typing import TYPE_CHECKING

from .. import office
from ..contracts import DocumentPdfRenderedJob, DocumentRenderPdfJob, JobKind
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py


def run(payload: DocumentRenderPdfJob, deps: Deps) -> list[FollowUp]:
    data = deps.download(payload.downloadUrl, deps.max_download_bytes)
    pdf: bytes | None = None
    error: str | None = None
    if not office.available():
        error = "LibreOffice is not installed in the AI worker, so no PDF can be made"
    else:
        with tempfile.TemporaryDirectory(prefix="hp-pdf-") as tmp:
            path = Path(tmp) / "cv.docx"
            path.write_bytes(data)
            pdf = office.convert(path, Path(tmp), "pdf")
        if pdf is None or not pdf.startswith(b"%PDF-"):
            pdf, error = None, "LibreOffice could not turn this document into a PDF"
    log.info("pdf rendered", extra={"document_id": payload.documentId.root, "ok": pdf is not None})
    return [
        FollowUp(
            queue=RESULT_QUEUE,
            kind=JobKind.document_pdf_rendered,
            payload=DocumentPdfRenderedJob(
                documentId=payload.documentId,
                pdfBase64=base64.b64encode(pdf).decode("ascii") if pdf else None,
                error=error,
            ),
        )
    ]

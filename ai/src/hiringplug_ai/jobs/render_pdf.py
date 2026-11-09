"""document.render_pdf: turn a formatted CV (.docx) into a PDF for Go to save."""

from __future__ import annotations

import base64
import io
import logging
import re
import tempfile
import zipfile
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
        pdf = _convert(data)
        if pdf is None and (patched := _end_after_frame(data)) is not None:
            pdf = _convert(patched)  # the Word file itself is kept as it was made
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


def _convert(data: bytes) -> bytes | None:
    with tempfile.TemporaryDirectory(prefix="hp-pdf-") as tmp:
        # Named as what it is, as LibreOffice expects: an old Word .doc (an OLE
        # file), RTF, OpenDocument text, or a .docx.
        path = Path(tmp) / _office_name(data)
        path.write_bytes(data)
        return office.convert(path, Path(tmp), "pdf")


def _office_name(data: bytes) -> str:
    if data.startswith(b"\xd0\xcf\x11\xe0"):
        return "cv.doc"
    if data.startswith(b"{\\rtf"):
        return "cv.rtf"
    if data.startswith(b"PK") and b"application/vnd.oasis.opendocument.text" in data[:200]:
        return "cv.odt"
    return "cv.docx"


_LAST_PARAGRAPH = re.compile(
    rb"<w:p\b(?:(?!<w:p\b).)*?</w:p>\s*(?=<w:sectPr\b(?:(?!<w:sectPr\b).)*</w:body>)", re.S
)


def _end_after_frame(data: bytes) -> bytes | None:
    """A copy of a .docx whose last paragraph is a frame (CV Studio pins its
    closing terms to the foot of the page that way) with an empty paragraph
    after it: LibreOffice cannot open a document that ends on a frame, Word
    can. None when the document does not end like that."""
    try:
        with zipfile.ZipFile(io.BytesIO(data)) as zin:
            body = zin.read("word/document.xml")
            last = _LAST_PARAGRAPH.search(body)
            if last is None or b"<w:framePr" not in last.group(0):
                return None
            fixed = body[: last.end()] + b"<w:p/>" + body[last.end() :]
            out = io.BytesIO()
            with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as zout:
                for item in zin.infolist():
                    zout.writestr(
                        item, fixed if item.filename == "word/document.xml" else zin.read(item.filename)
                    )
            return out.getvalue()
    except (zipfile.BadZipFile, KeyError):
        return None

"""jd.render_pdf: a Word (or RTF, ODT) job description as a PDF, so the job's
page previews it as it looks; Go saves it with the file (jd.pdf_rendered)."""

from __future__ import annotations

import base64
import logging
from typing import TYPE_CHECKING

from ..contracts import JdPdfRenderedJob, JdRenderPdfJob, JobKind
from ..queue import FollowUp
from .render_pdf import make_pdf

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py


def run(payload: JdRenderPdfJob, deps: Deps) -> list[FollowUp]:
    data = deps.download(payload.downloadUrl, deps.max_download_bytes)
    pdf, error = make_pdf(data)
    log.info("jd pdf rendered", extra={"jd_file_id": payload.jdFileId.root, "ok": pdf is not None})
    return [
        FollowUp(
            queue=RESULT_QUEUE,
            kind=JobKind.jd_pdf_rendered,
            payload=JdPdfRenderedJob(
                jdFileId=payload.jdFileId,
                pdfBase64=base64.b64encode(pdf).decode("ascii") if pdf else None,
                error=error,
            ),
        )
    ]

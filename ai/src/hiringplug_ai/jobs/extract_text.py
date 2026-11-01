"""document.extract_text: read an uploaded CV and send its text back to Go."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from ..contracts import DocumentExtractTextJob, DocumentTextExtractedJob, ExtractionStatus, JobKind
from ..extraction import extract
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py


def run(payload: DocumentExtractTextJob, deps: Deps) -> list[FollowUp]:
    data = deps.download(payload.downloadUrl, deps.max_download_bytes)
    result = extract(data, ocr=deps.ocr, limits=deps.limits)
    log.info(
        "document read",
        extra={
            "document_id": payload.documentId.root,
            "status": result.status,
            "pages": result.page_count,
            "extractor": result.extractor,
            "warnings": len(result.warnings),
        },
    )
    return [
        FollowUp(
            queue=RESULT_QUEUE,
            kind=JobKind.document_text_extracted,
            payload=DocumentTextExtractedJob(
                documentId=payload.documentId,
                status=ExtractionStatus(result.status),
                text=result.text,
                pageCount=result.page_count,
                extractor=result.extractor,
                errorMessage=result.error,
                warnings=[w[:500] for w in result.warnings[:20]],
            ),
        )
    ]

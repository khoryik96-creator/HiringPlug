"""job.read_jd: read the text of a client's job description and send it back to Go,
which reads the ad's details from it."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from ..contracts import ExtractionStatus, JobJdReadJob, JobKind, JobReadJdJob
from ..extraction import extract
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py


def run(payload: JobReadJdJob, deps: Deps) -> list[FollowUp]:
    data = deps.download(payload.downloadUrl, deps.max_download_bytes)
    result = extract(data, ocr=deps.ocr, limits=deps.limits)
    log.info(
        "job description read",
        extra={"jd_file_id": payload.jdFileId.root, "status": result.status, "pages": result.page_count},
    )
    return [
        FollowUp(
            queue=RESULT_QUEUE,
            kind=JobKind.job_jd_read,
            payload=JobJdReadJob(
                jdFileId=payload.jdFileId,
                status=ExtractionStatus(result.status),
                text=result.text,
                errorMessage=result.error,
            ),
        )
    ]

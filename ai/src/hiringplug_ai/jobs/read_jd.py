"""job.read_jd: read the text of a client's job description and send it back to Go,
which reads the ad's details from it; with the AI on, the AI reads the details too
(never the pay: see jd_ai)."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from ..contracts import ExtractionStatus, JdAiDetails, JobJdReadJob, JobKind, JobReadJdJob
from ..extraction import extract
from ..providers import ProviderError
from ..queue import FollowUp
from . import jd_ai

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
    found: JdAiDetails | None = None
    tokens = 0
    if payload.useAi and result.status == "extracted" and result.text:
        try:
            ai = deps.provider(payload.ai)
            if ai is not None:
                read = jd_ai.read(result.text, ai)
                found, tokens = read.details, read.tokens
        except ProviderError:
            log.warning("ai key could not be opened", extra={"jd_file_id": payload.jdFileId.root})
        except Exception:  # an AI-step bug must not lose the rules' reading
            log.warning("ai jd read failed", exc_info=True, extra={"jd_file_id": payload.jdFileId.root})
    return [
        FollowUp(
            queue=RESULT_QUEUE,
            kind=JobKind.job_jd_read,
            payload=JobJdReadJob(
                jdFileId=payload.jdFileId,
                status=ExtractionStatus(result.status),
                text=result.text,
                errorMessage=result.error,
                aiDetails=found,
                aiTokens=tokens,
            ),
        )
    ]

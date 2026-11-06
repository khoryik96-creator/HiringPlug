"""contact.read_linkedin: read a contact's LinkedIn profile saved as PDF and the
jobs in it, with the rules (no AI), and send them to Go, which compares them
with the contact's company."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from ..contracts import ContactLinkedinReadJob, ContactReadLinkedinJob, JobKind
from ..extraction import extract
from ..parsing import parse_cv
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py


def run(payload: ContactReadLinkedinJob, deps: Deps) -> list[FollowUp]:
    data = deps.download(payload.downloadUrl, deps.max_download_bytes)
    result = extract(data, ocr=deps.ocr, limits=deps.limits)
    profile = error = None
    if result.status != "extracted" or not (result.text or "").strip():
        error = result.error or "no text was found in it"
    else:
        try:
            profile = parse_cv(result.text or "")
        except Exception:  # a rule bug must not leave the check "reading" forever
            log.warning(
                "linkedin profile could not be parsed",
                exc_info=True,
                extra={"check_id": payload.checkId.root},
            )
            error = "the details could not be read from it"
    # Counts only: the profile is about a person.
    log.info(
        "linkedin profile read",
        extra={"check_id": payload.checkId.root, "jobs": len(profile.experience) if profile else 0},
    )
    return [
        FollowUp(
            queue=RESULT_QUEUE,
            kind=JobKind.contact_linkedin_read,
            payload=ContactLinkedinReadJob(checkId=payload.checkId, profile=profile, errorMessage=error),
        )
    ]

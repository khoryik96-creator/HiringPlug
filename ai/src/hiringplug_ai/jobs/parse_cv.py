"""document.parse_cv: read profile details out of a CV's text and send them to Go."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from ..contracts import DocumentCvParsedJob, DocumentParseCvJob, JobKind
from ..parsing import parse_cv
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py


def run(payload: DocumentParseCvJob, deps: Deps) -> list[FollowUp]:
    profile = parse_cv(payload.text)
    # Counts only: CV contents (names, emails) never go in logs.
    log.info(
        "cv parsed",
        extra={
            "document_id": payload.documentId.root,
            "parser": profile.parser,
            "jobs": len(profile.experience),
            "schools": len(profile.education),
            "skills": len(profile.skills),
        },
    )
    return [
        FollowUp(
            queue=RESULT_QUEUE,
            kind=JobKind.document_cv_parsed,
            payload=DocumentCvParsedJob(documentId=payload.documentId, profile=profile, errorMessage=None),
        )
    ]

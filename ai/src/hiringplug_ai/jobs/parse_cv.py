"""document.parse_cv: read profile details out of a CV's text and send them to Go."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from ..contracts import DocumentCvParsedJob, DocumentParseCvJob, JobKind, ParsedProfile
from ..parsing import parse_cv
from ..providers import ProviderError
from ..queue import FollowUp
from . import cv_ai

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py


def run(payload: DocumentParseCvJob, deps: Deps) -> list[FollowUp]:
    tokens = 0
    try:
        profile = parse_cv(payload.text, file_name=payload.fileName)
        ai = None
        if payload.useAi and payload.aiText:
            try:
                ai = deps.provider(payload.ai)
            except ProviderError:
                log.warning("ai key could not be opened", extra={"document_id": payload.documentId.root})
        if ai is not None and payload.aiText:
            try:
                filled = cv_ai.fill(profile, payload.aiText, ai)  # never the salary figures
                profile, tokens = filled.profile, filled.tokens
            except Exception:  # an AI-step bug must not lose the rules' reading
                log.warning("ai fill failed", exc_info=True, extra={"document_id": payload.documentId.root})
    except Exception:  # a rule bug must not leave the CV "being read" forever
        # Retrying cannot change the outcome for the same text: report the
        # failure, so the document shows "details could not be read".
        log.warning("cv could not be parsed", exc_info=True, extra={"document_id": payload.documentId.root})
        return [_result(payload, None, "The details could not be read from this CV.")]
    # Counts only: CV contents (names, emails) never go in logs.
    log.info(
        "cv parsed",
        extra={
            "document_id": payload.documentId.root,
            "parser": profile.parser,
            "jobs": len(profile.experience),
            "schools": len(profile.education),
            "skills": len(profile.skills),
            "ai_tokens": tokens,
        },
    )
    return [_result(payload, profile, None, tokens)]


def _result(
    payload: DocumentParseCvJob, profile: ParsedProfile | None, error: str | None, tokens: int = 0
) -> FollowUp:
    return FollowUp(
        queue=RESULT_QUEUE,
        kind=JobKind.document_cv_parsed,
        payload=DocumentCvParsedJob(
            documentId=payload.documentId, profile=profile, errorMessage=error, aiTokens=tokens
        ),
    )

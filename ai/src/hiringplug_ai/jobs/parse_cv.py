"""document.parse_cv: read profile details out of a CV's text and send them to Go."""

from __future__ import annotations

import logging
import os
import traceback
from typing import TYPE_CHECKING

from ..contracts import DocumentCvParsedJob, DocumentParseCvJob, JobKind, ParsedProfile
from ..parsing import parse_cv
from ..providers import ProviderError
from ..queue import FollowUp
from . import cv_ai, cv_history

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
                # The AI reads the work history whenever it is on (the rules confuse a
                # client or a team with the employer), then fills what is still missing.
                labels = [i.root for i in payload.industries or []]
                history = cv_history.read(profile, payload.aiText, ai, labels)  # never the salary figures
                filled = cv_ai.fill(history.profile, payload.aiText, ai)
                profile, tokens = filled.profile, history.tokens + filled.tokens
            except Exception:  # an AI-step bug must not lose the rules' reading
                log.warning("ai fill failed", exc_info=True, extra={"document_id": payload.documentId.root})
    except Exception as exc:  # a rule bug must not leave the CV "being read" forever
        # Retrying cannot change the outcome for the same text: report the
        # failure, so the document shows "details could not be read", and where
        # it broke, to be fixed (never the exception's message: it may quote the CV).
        log.warning("cv could not be parsed", exc_info=True, extra={"document_id": payload.documentId.root})
        return [_result(payload, None, f"The details could not be read from this CV ({where(exc)}).")]
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


def where(exc: BaseException) -> str:
    """The error's type and the parser line it came from ("IndexError at experience.py:120"): no CV text."""
    frames = traceback.extract_tb(exc.__traceback__)
    ours = [f for f in frames if "hiringplug_ai" in f.filename] or frames
    if not ours:
        return type(exc).__name__
    last = ours[-1]
    return f"{type(exc).__name__} at {os.path.basename(last.filename)}:{last.lineno}"


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

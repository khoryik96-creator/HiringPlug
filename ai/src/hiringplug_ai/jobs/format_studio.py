"""cv.format_studio: format a candidate's CV CV Studio's way, and send the
Word file back to Go (cv.studio_formatted), which saves it with their
documents.

The order is CV Studio's own: the CV's text (lines stating the candidate's
pay removed first, so they never reach the AI); the AI reads it into CV
Studio's structure with its prompt; CV Studio's rules tidy and check it
against the text (a warning when something may be lost or changed); for a
blind CV the AI anonymises it and CV Studio's checks run again; its renderer
writes the Word file with its template. Anything that fails is sent back as
the reason, never raised, so the person waiting sees why.
"""

from __future__ import annotations

import base64
import json
import logging
from typing import TYPE_CHECKING

from .. import cvstudio
from ..contracts import CvFormatStudioJob, CvStudioFormattedJob, JobKind
from ..extraction import extract
from ..providers import ProviderError
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py
MAX_SOURCE_CHARS = 60_000
PARSE_TOKENS = 16_000  # a long CV's structure
BLIND_TOKENS = 16_000


class _FailedError(Exception):
    """A reason to send back instead of a file."""


def run(payload: CvFormatStudioJob, deps: Deps) -> list[FollowUp]:
    tokens = {"in": 0, "out": 0}
    model: list[str] = []
    result = CvStudioFormattedJob(
        studioCvId=payload.studioCvId,
        docxBase64=None,
        error=None,
        warning=None,
        payStatementsRemoved=0,
        model=None,
        inputTokens=0,
        outputTokens=0,
    )
    try:
        docx, warning, removed = _format(payload, deps, tokens, model)
        result.docxBase64 = base64.b64encode(docx).decode("ascii")
        result.warning = warning[:4000] if warning else None
        result.payStatementsRemoved = removed
    except _FailedError as exc:
        result.error = str(exc)[:2000]
    except ProviderError as exc:
        result.error = str(exc)[:2000]
    result.model = model[-1] if model else None
    result.inputTokens, result.outputTokens = tokens["in"], tokens["out"]
    log.info(
        "studio cv",
        extra={
            "studio_cv_id": payload.studioCvId.root,
            "ok": result.error is None,
            "warning": result.warning is not None,
            "tokens": tokens["in"] + tokens["out"],
        },
    )
    return [FollowUp(queue=RESULT_QUEUE, kind=JobKind.cv_studio_formatted, payload=result)]


def _format(
    payload: CvFormatStudioJob, deps: Deps, tokens: dict[str, int], model: list[str]
) -> tuple[bytes, str | None, int]:
    ai = deps.provider(payload.ai)
    if ai is None:
        raise _FailedError("no AI provider is set up")
    data = deps.download(payload.downloadUrl, deps.max_download_bytes)
    read = extract(data, ocr=deps.ocr, limits=deps.limits)
    if not read.text or not read.text.strip():
        raise _FailedError(read.error or "no text was found in the CV")
    text, removed = cvstudio.without_pay(read.text[:MAX_SOURCE_CHARS])
    source = cvstudio.prepare_source(text)

    def ask(system: str, user: str, max_tokens: int) -> dict[str, object]:
        done = ai.complete(system, user, max_tokens=max_tokens)
        tokens["in"] += done.input_tokens
        tokens["out"] += done.output_tokens
        model.append(done.model)
        value = cvstudio.answer_json(done.text)
        if value is None:
            raise _FailedError("the AI's answer could not be read")
        return value

    parsed = ask(cvstudio.parse_prompt(payload.correctLanguage), source, PARSE_TOKENS)
    try:
        checked = cvstudio.after_parse(parsed, source, correct_language=payload.correctLanguage)
    except Exception as exc:
        log.exception("cv studio rules failed")
        raise _FailedError("CV Studio's rules could not use the AI's reading") from exc
    cv = checked["data"]
    removed += int(checked.get("summary_pay_removed") or 0)
    warning = checked.get("warning") if checked.get("degraded") else None

    if payload.blind:
        blinded = ask(
            cvstudio.blind_prompt(payload.neutralGender), json.dumps(cv, ensure_ascii=False), BLIND_TOKENS
        )
        try:
            cv = cvstudio.after_blind(blinded, cv)
        except Exception as exc:
            log.exception("cv studio blind checks failed")
            raise _FailedError("CV Studio's blind checks could not use the AI's answer") from exc

    levels = checked.get("bullet_levels") or []
    if payload.contentType.endswith("wordprocessingml.document"):  # the Word file's own list depth first
        levels = cvstudio.merge_levels(cvstudio.docx_bullet_levels(data), levels)
    try:
        docx = cvstudio.render(cv, alignment=payload.alignment.value, bullet_levels=list(levels))
    except Exception as exc:
        log.exception("cv studio renderer failed")
        raise _FailedError("the Word file could not be made") from exc
    return docx, str(warning) if warning else None, removed

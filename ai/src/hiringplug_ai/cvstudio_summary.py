"""CV Studio's "About Him / Her" summary, written while a CV is formatted
(cv-format.js: requestFormattingSummary): its prompt and its reading of the
answer come from its own candidate-summary.js, run by Node through
summary_cli.cjs; lines stating the candidate's pay are then removed, as its
server does (strip_candidate_pay).
"""

from __future__ import annotations

import json
import subprocess
from typing import Any

import cvstudio_cv_normalize as normal  # type: ignore[import-not-found]

from .cvstudio import ENGINE  # also puts CV Studio's engine on the module path

TIMEOUT_SECONDS = 30
MAX_TOKENS = 1000  # CV Studio's, for the summary written while formatting


class SummaryError(RuntimeError):
    """CV Studio's summary helpers could not run (never carries CV text)."""


def _call(op: str, **request: Any) -> Any:
    done = subprocess.run(  # noqa: S603 - fixed program and script, our own files
        ["node", str(ENGINE / "summary_cli.cjs")],  # noqa: S607 - node from PATH
        input=json.dumps({"op": op, **request}, ensure_ascii=False),
        capture_output=True,
        text=True,
        timeout=TIMEOUT_SECONDS,
        check=False,
    )
    try:
        answer = json.loads(done.stdout or "{}")
    except ValueError as exc:
        raise SummaryError(f"CV Studio's summary helpers ({op}) failed") from exc
    if not isinstance(answer, dict) or not answer.get("ok"):
        raise SummaryError(f"CV Studio's summary helpers ({op}) failed")
    return answer.get("value")


def prompt(cv_text: str, *, length: str = "normal", focus: str = "general", direction: str = "") -> str:
    """CV Studio's prompt: short (4-5 bullets), normal (6-7) or detailed
    (8-10); bringing out its focus (general, technology, leadership or
    commercial) and following the recruiter's direction and tone."""
    return str(_call("prompt", cv=cv_text, detail=length, focus=focus, direction=direction))


def bullets(answer: str) -> tuple[list[str], int]:
    """The summary's bullets from the AI's answer, without lines stating the
    candidate's pay; and how many such lines were removed."""
    lines = [str(x) for x in _call("bullets", text=answer)]
    kept, removed = normal._cv_strip_pay_from_summary_counted(lines)
    return [str(x) for x in kept], int(removed)

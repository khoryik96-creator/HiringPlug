"""jd.blind: rewrite a job ad as a blind job description, CV Studio's way,
and send it back to Go (jd.blinded), which makes it the ad's blind version.

The order is CV Studio's own (generateAnonJD in blind-jd.js): the text's PDF
damage repaired; CV Studio's prompt (identity stripped, wording paraphrased
at the chosen strength and tone, company scale generalised, tech stack kept)
sent alone; its JSON read; asked once more when responsibilities or
requirements still copy long runs of the ad (the answer kept only when it
copies less); then CV Studio's checks: genuine tech terms from the ad put
back in the stack, company size and reach softened, the client's name
scrubbed. What is left to check comes back as warnings. Anything that fails
is sent back as the reason, never raised, so the person waiting sees why.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from .. import cvstudio, cvstudio_jd
from ..contracts import BlindJd, JdBlindedJob, JdBlindJob, JobKind
from ..providers import ProviderError
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py
MAX_TOKENS = 4000  # CV Studio's
MIN_TEXT = 80  # CV Studio asks for more below this
CORE = ("responsibilities", "requirements")

# CV Studio's JSON keys -> the contract's, with their size limits.
_TEXTS = {
    "job_title": ("jobTitle", 200),
    "about_client": ("aboutClient", 4000),
    "about_role": ("aboutRole", 4000),
    "work_mode": ("workMode", 200),
    "location": ("location", 200),
    "industry": ("industry", 200),
}
_LISTS = {
    "tech_stack": ("techStack", 80, 200),
    "responsibilities": ("responsibilities", 60, 2000),
    "requirements": ("requirements", 60, 2000),
    "nice_to_have": ("niceToHave", 60, 2000),
    "why_join": ("whyJoin", 60, 2000),
    "alt_titles": ("altTitles", 20, 200),
}


class _FailedError(Exception):
    """A reason to send back instead of a rewrite."""


def run(payload: JdBlindJob, deps: Deps) -> list[FollowUp]:
    tokens = {"in": 0, "out": 0}
    model: list[str] = []
    out: dict[str, Any] = {"jd": None, "error": None, "warnings": [], "retried": False}
    try:
        jd, warnings, retried = _rewrite(payload, deps, tokens, model)
        out.update(jd=jd.model_dump(mode="json"), warnings=[w[:1000] for w in warnings[:10]], retried=retried)
    except (_FailedError, ProviderError) as exc:
        out["error"] = str(exc)[:2000]
    except cvstudio_jd.GuardError as exc:
        log.exception("cv studio jd checks failed")
        out["error"] = str(exc)[:2000]
    result = JdBlindedJob.model_validate(
        {
            **out,
            "rewriteId": payload.rewriteId.root,
            "model": model[-1] if model else None,
            "inputTokens": tokens["in"],
            "outputTokens": tokens["out"],
        }
    )
    log.info(
        "blind jd",
        extra={
            "rewrite_id": payload.rewriteId.root,
            "ok": result.error is None,
            "retried": result.retried,
            "tokens": tokens["in"] + tokens["out"],
        },
    )
    return [FollowUp(queue=RESULT_QUEUE, kind=JobKind.jd_blinded, payload=result)]


def _rewrite(
    payload: JdBlindJob, deps: Deps, tokens: dict[str, int], model: list[str]
) -> tuple[BlindJd, list[str], bool]:
    ai = deps.provider(payload.ai)
    if ai is None:
        raise _FailedError("no AI provider is set up")
    raw = cvstudio_jd.normalize(payload.raw).strip()
    if len(raw) < MIN_TEXT:
        raise _FailedError("the ad has too little text to rewrite")
    company = payload.companyName
    prompt = cvstudio_jd.prompt(
        raw=raw,
        companyName=company,
        industry=payload.industry or "",
        location=payload.location or "",
        workMode=payload.workMode or "",
        clientDesc=payload.clientDescription or "",
        context=payload.context or "",
        redact=payload.removeAlso or "",
        whyJoinUs=payload.whyJoinUs or "",
        tone=payload.tone.value,
        strength=payload.strength.value,
    )

    def ask(text: str) -> dict[str, Any] | None:
        deps.keep_alive()
        done = ai.complete("", text, max_tokens=MAX_TOKENS)
        tokens["in"] += done.input_tokens
        tokens["out"] += done.output_tokens
        model.append(done.model)
        return cvstudio.answer_json(done.text)

    jd = ask(prompt)
    if jd is None:  # CV Studio shows plain text here; a blind ad needs its structure
        raise _FailedError("the AI's answer could not be read")
    retried = False
    if cvstudio_jd.needs_retry(jd, raw):
        retried = True
        found = cvstudio_jd.issues(jd, raw)
        instruction = cvstudio_jd.retry_instruction(found)
        retry_prompt = prompt.replace(
            "\nORIGINAL JD:\n---\n", "\n" + instruction + "\n\nORIGINAL JD:\n---\n", 1
        )
        if retry_prompt == prompt:
            retry_prompt = prompt + instruction
        again = ask(retry_prompt)
        if again is not None:  # an unreadable retry keeps the first answer
            left = cvstudio_jd.issues(again, raw)
            first_core = any(f in found for f in CORE)
            if len(left) < len(found) or (first_core and not any(f in left for f in CORE)):
                jd = again
    done = cvstudio_jd.finalize(jd, raw, company)
    return _contract(done["data"]), _warnings(done), retried


def _warnings(done: dict[str, Any]) -> list[str]:
    warnings = []
    missing = [str(t) for t in done.get("missingAfter") or []]
    if missing:
        warnings.append("Check the tech stack: " + ", ".join(missing[:20]) + " from the ad may be missing.")
    if any(f in (done.get("paraphraseIssues") or []) for f in CORE):
        warnings.append(
            "Some responsibilities or requirements are still close to the ad's wording:"
            " reword them before sharing."
        )
    return warnings


def _contract(data: dict[str, Any]) -> BlindJd:
    """CV Studio's JD in the contract's shape: text where text is expected,
    lists of non-empty lines, everything held to its size."""
    out: dict[str, Any] = {}
    for key, (name, limit) in _TEXTS.items():
        value = data.get(key)
        out[name] = value.strip()[:limit] if isinstance(value, str) and value != "null" else ""
    for key, (name, count, limit) in _LISTS.items():
        value = data.get(key)
        items = value if isinstance(value, list) else ([value] if isinstance(value, str) and value else [])
        out[name] = [
            str(x).strip()[:limit] for x in items if isinstance(x, (str, int, float)) and str(x).strip()
        ][:count]
    return BlindJd.model_validate(out)

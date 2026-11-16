"""company.industry: pick the closest of the agency's industries for a new
company, from what is known of it (its name and website, what it says of
itself, and an industry named elsewhere such as LinkedIn's), and send it back
to Go (company.industried), which gives it to the company if it still has
none. The choices are numbered and the AI answers with a number only, so it
can never invent an industry; anything else is "none fits". Company facts
only: nothing about people is sent.
"""

from __future__ import annotations

import logging
import re
from typing import TYPE_CHECKING

from ..contracts import CompanyIndustriedJob, CompanyIndustryJob, IndustryId, JobKind
from ..providers import ProviderError
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py
MAX_TOKENS = 20
MAX_ABOUT = 3000

SYSTEM = (
    "You sort companies into a recruitment agency's industry list. You are given a company and a numbered "
    'list of industries and subindustries ("Industry > Subindustry"). Answer with the number of the one '
    "that fits the company best: the most specific subindustry that fits, else its industry. If nothing on "
    "the list fits, answer 0. Answer with the number only."
)


def prompt(payload: CompanyIndustryJob) -> str:
    """The company and the numbered choices."""
    lines = [f"Company: {payload.companyName}"]
    if payload.website:
        lines.append(f"Website: {payload.website}")
    if payload.hint:
        lines.append(f"Industry named on LinkedIn (not on the list): {payload.hint}")
    if payload.about:
        lines.append(f"What it says of itself: {payload.about[:MAX_ABOUT]}")
    lines.append("")
    lines.append("Industries:")
    lines.extend(f"{i}. {c.label}" for i, c in enumerate(payload.choices, start=1))
    return "\n".join(lines)


def picked(text: str, choices: int) -> int | None:
    """The choice's index (0-based) from the AI's answer; None for 0, nothing or a number off the list."""
    m = re.search(r"\d+", text)
    if not m:
        return None
    n = int(m.group())
    return n - 1 if 1 <= n <= choices else None


def run(payload: CompanyIndustryJob, deps: Deps) -> list[FollowUp]:
    choice: IndustryId | None = None
    error = model = None
    input_tokens = output_tokens = 0
    try:
        ai = deps.provider(payload.ai)
        if ai is None:
            error = "the AI is switched off in the AI worker (HP_AI_PROVIDER is off)"
        else:
            done = ai.complete(SYSTEM, prompt(payload), max_tokens=MAX_TOKENS)
            model, input_tokens, output_tokens = done.model, done.input_tokens, done.output_tokens
            i = picked(done.text, len(payload.choices))
            if i is not None:
                choice = payload.choices[i].id
    except ProviderError as exc:
        error = str(exc)
    log.info(
        "company industry",
        extra={"pick_id": payload.pickId.root, "picked": choice is not None, "ok": error is None},
    )
    return [
        FollowUp(
            queue=RESULT_QUEUE,
            kind=JobKind.company_industried,
            payload=CompanyIndustriedJob(
                pickId=payload.pickId,
                choiceId=choice,
                error=error,
                model=model,
                inputTokens=input_tokens,
                outputTokens=output_tokens,
            ),
        )
    ]

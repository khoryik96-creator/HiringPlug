"""aiprovider.check: send a provider one tiny request, to tell the company
whether the key and model it saved in Settings work."""

from __future__ import annotations

import dataclasses
import logging
from typing import TYPE_CHECKING

from ..contracts import AiProviderCheckedJob, AiProviderCheckJob, JobKind
from ..paid_switch import PaidAIPausedError
from ..providers import ProviderError
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py

SYSTEM = "You check that an API connection works. Reply with the single word OK."
MAX_TOKENS = 16


def run(payload: AiProviderCheckJob, deps: Deps) -> list[FollowUp]:
    message: str | None = None
    paused = False
    input_tokens = output_tokens = 0
    # Checking a key (often of a provider the company does not use yet) says
    # nothing about the AI in use: it is not reported as an outage.
    deps = dataclasses.replace(deps, log=None)
    try:
        ai = deps.provider(payload.access)
        if ai is None:  # cannot happen: a check always carries the access
            message = "the AI worker has no provider to check"
        else:
            done = ai.complete(SYSTEM, "Reply with OK.", max_tokens=MAX_TOKENS)
            # A check uses the provider too: counted in the company's AI use.
            input_tokens, output_tokens = max(done.input_tokens, 0), max(done.output_tokens, 0)
    except PaidAIPausedError as exc:
        # Queued before paid AI was paused: the key was not asked, so it is
        # not failed; the app says it was not checked.
        message, paused = str(exc), True
    except ProviderError as exc:
        message = str(exc)
    outcome = {"provider": payload.provider.value, "ok": message is None, "paused": paused}
    log.info("ai provider checked", extra=outcome)
    return [
        FollowUp(
            queue=RESULT_QUEUE,
            kind=JobKind.aiprovider_checked,
            payload=AiProviderCheckedJob(
                provider=payload.provider,
                sealedKey=payload.access.sealedKey,
                ok=message is None,
                message=message,
                paused=paused or None,
                checkId=payload.checkId,
                askedBy=payload.askedBy,
                inputTokens=input_tokens,
                outputTokens=output_tokens,
            ),
        )
    ]

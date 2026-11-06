"""aiprovider.check: send a provider one tiny request, to tell the company
whether the key and model it saved in Settings work."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from ..contracts import AiProviderCheckedJob, AiProviderCheckJob, JobKind
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
    try:
        ai = deps.provider(payload.access)
        if ai is None:  # cannot happen: a check always carries the access
            message = "the AI worker has no provider to check"
        else:
            ai.complete(SYSTEM, "Reply with OK.", max_tokens=MAX_TOKENS)
    except ProviderError as exc:
        message = str(exc)
    log.info("ai provider checked", extra={"provider": payload.provider.value, "ok": message is None})
    return [
        FollowUp(
            queue=RESULT_QUEUE,
            kind=JobKind.aiprovider_checked,
            payload=AiProviderCheckedJob(
                provider=payload.provider,
                sealedKey=payload.access.sealedKey,
                ok=message is None,
                message=message,
            ),
        )
    ]

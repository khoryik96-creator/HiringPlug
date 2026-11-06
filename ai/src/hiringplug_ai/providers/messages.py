"""Claude, and DeepSeek through its Anthropic-compatible endpoint, with
Anthropic's official Python SDK (the Messages API)."""

from __future__ import annotations

import time
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

import anthropic
import httpx2

from . import Completion, ProviderError
from .retry import REJECTED_BEFORE_WORK, RejectedError, with_retries

DEEPSEEK_URL = "https://api.deepseek.com/anthropic"

# DeepSeek retired these names (CV Studio maps them the same way): a saved
# setting from before must not make every AI feature fail.
DEEPSEEK_RETIRED = {"deepseek-chat": "deepseek-v4-flash", "deepseek-reasoner": "deepseek-v4-flash"}

# Claude models that take server-side refusal fallbacks: when the model
# declines (a safety classifier, sometimes wrongly on recruiting text), the
# API answers with another model in the same call instead of failing.
FALLBACK_MODELS = frozenset({"claude-fable-5-1", "claude-opus-5-5", "claude-opus-5", "claude-sonnet-5-5"})
FALLBACK_BETA = "server-side-fallback-2026-07-01"

# Claude thinks before answering (it cannot be switched off on the current
# models), and thinking counts against max_tokens: room is added so the
# answer itself is never cut short. Kept under the size the SDK allows
# without streaming.
THINKING_ROOM = 8000
MAX_TOKENS = 20000


@dataclass(frozen=True)
class Messages:
    provider: str  # "claude" or "deepseek"
    api_key: str = field(repr=False)
    model: str
    timeout_seconds: float = 90.0
    http_client: httpx2.Client | None = None  # tests
    sleep: Callable[[float], None] = time.sleep

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        deepseek = self.provider == "deepseek"
        model = DEEPSEEK_RETIRED.get(self.model, self.model) if deepseek else self.model
        client = anthropic.Anthropic(
            api_key=self.api_key,
            base_url=DEEPSEEK_URL if deepseek else None,
            timeout=self.timeout_seconds,
            max_retries=0,  # retries are ours: only when no work was done
            http_client=self.http_client,
        )
        params: dict[str, Any] = {
            "model": model,
            "system": system,
            "messages": [{"role": "user", "content": user}],
        }
        if deepseek:
            # DeepSeek V4 thinks by default: slow (30-160 s seen by CV Studio)
            # and no better at these extraction tasks.
            params["max_tokens"] = max_tokens
            params["thinking"] = {"type": "disabled"}
        else:
            params["max_tokens"] = min(max_tokens + THINKING_ROOM, MAX_TOKENS)

        def send() -> Any:
            try:
                if not deepseek and model in FALLBACK_MODELS:
                    return client.beta.messages.create(**params, betas=[FALLBACK_BETA], fallbacks="default")
                return client.messages.create(**params)
            except anthropic.APIStatusError as exc:
                if exc.status_code in REJECTED_BEFORE_WORK and not _no_credit(exc):
                    raise RejectedError(exc.response.headers.get("retry-after")) from exc
                raise _status_error(exc, model) from exc
            except anthropic.APITimeoutError as exc:
                raise ProviderError("the AI provider did not answer in time") from exc
            except anthropic.APIConnectionError as exc:
                raise ProviderError("the AI provider could not be reached") from exc

        # One client per request, closed after it: a worker must not keep a
        # connection pool open for every call it ever made.
        with client:
            message = with_retries(
                send,
                lambda: ProviderError(
                    "the AI provider is busy (rate limited or overloaded); try again in a minute"
                ),
                self.sleep,
            )
        if message.stop_reason == "refusal":
            raise ProviderError("the AI declined to answer this request")
        if message.stop_reason == "max_tokens":
            raise ProviderError("the AI's answer was cut off (it ran past the length allowed)")
        text = "".join(block.text for block in message.content if block.type == "text")
        usage = message.usage
        return Completion(
            text=text.strip(),
            model=str(message.model or model),
            input_tokens=int(usage.input_tokens or 0),
            output_tokens=int(usage.output_tokens or 0),
        )


def _no_credit(exc: anthropic.APIStatusError) -> bool:
    body = str(exc.body or "").lower()
    return exc.status_code == 402 or any(
        w in body for w in ("credit balance", "insufficient balance", "insufficient_quota")
    )


def _status_error(exc: anthropic.APIStatusError, model: str) -> ProviderError:
    """A fixed message per kind of failure: the provider's own text can echo
    part of the key, so it is never passed on."""
    status = exc.status_code
    if status in (401, 403):
        return ProviderError("the AI provider refused the API key: check it in Settings > AI provider")
    if _no_credit(exc):
        return ProviderError("the AI provider account has no credit left: top it up, or switch provider")
    if status == 404 or (status in (400, 422) and "model" in str(exc.body or "").lower()):
        return ProviderError(
            f"the AI provider does not offer the model {model!r}: change it in Settings > AI provider"
        )
    return ProviderError(f"the AI provider answered with HTTP {status}")

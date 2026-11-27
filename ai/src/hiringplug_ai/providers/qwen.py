"""Qwen through Alibaba Cloud Model Studio's OpenAI-compatible chat API.

The international (Singapore) endpoint: keys made in Model Studio's
international console work there, and CVs stay out of mainland China.
Thinking is off: HyPlug's prompts ask for short, structured answers, and
some Qwen 3 models refuse a non-streamed request while they think.
"""

from __future__ import annotations

import time
from collections.abc import Callable
from dataclasses import dataclass, field

import httpx

from . import Completion, ProviderError
from .retry import REJECTED_BEFORE_WORK, RejectedError, with_retries

QWEN_URL = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions"

# Model Studio's codes for an account that cannot pay (overdue, or no quota).
NO_CREDIT = frozenset({"arrearage", "insufficient_quota", "freetieronly"})


@dataclass(frozen=True)
class Qwen:
    api_key: str = field(repr=False)
    model: str
    timeout_seconds: float = 90.0
    transport: httpx.BaseTransport | None = None  # tests
    sleep: Callable[[float], None] = time.sleep

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        body: dict[str, object] = {
            "model": self.model,
            # No system message when there is none (CV Studio sends some prompts alone).
            "messages": ([{"role": "system", "content": system}] if system else [])
            + [{"role": "user", "content": user}],
            "max_tokens": max_tokens,
            "temperature": 0.2,
            "stream": False,
            "enable_thinking": False,
        }
        headers = {"Content-Type": "application/json", "Authorization": f"Bearer {self.api_key}"}

        def send() -> httpx.Response:
            try:
                with httpx.Client(
                    timeout=self.timeout_seconds, transport=self.transport, follow_redirects=False
                ) as client:
                    response = client.post(QWEN_URL, json=body, headers=headers)
            except httpx.TimeoutException as exc:
                raise ProviderError("the AI provider did not answer in time") from exc
            except httpx.HTTPError as exc:
                raise ProviderError("the AI provider could not be reached") from exc
            if response.status_code in REJECTED_BEFORE_WORK and _code(response) not in NO_CREDIT:
                raise RejectedError(response.headers.get("retry-after"))
            return response

        response = with_retries(
            send,
            lambda: ProviderError(
                "the AI provider is busy (rate limited or overloaded); try again in a minute"
            ),
            self.sleep,
        )
        if response.status_code != httpx.codes.OK:
            raise _status_error(response, self.model)
        try:
            data = response.json()
            choice = data["choices"][0]
            text = choice["message"]["content"]
            usage = data.get("usage") or {}
        except (ValueError, KeyError, IndexError, TypeError) as exc:
            raise ProviderError("the AI provider's answer could not be read") from exc
        if choice.get("finish_reason") == "length":
            raise ProviderError("the AI's answer was cut off (it ran past the length allowed)")
        return Completion(
            text=str(text or "").strip(),
            model=str(data.get("model") or self.model),
            input_tokens=int(usage.get("prompt_tokens") or 0),
            output_tokens=int(usage.get("completion_tokens") or 0),
        )


def _code(response: httpx.Response) -> str:
    try:
        error = response.json().get("error") or {}
        return str(error.get("code") or error.get("type") or "").lower()
    except (ValueError, AttributeError):
        return ""


def _status_error(response: httpx.Response, model: str) -> ProviderError:
    """A fixed message per kind of failure (the provider's own text can echo part of the key)."""
    status, code = response.status_code, _code(response)
    if status in (401, 403) or code == "invalid_api_key":
        return ProviderError("the AI provider refused the API key: check it in Settings > AI provider")
    if status == 402 or code in NO_CREDIT:
        return ProviderError("the AI provider account has no credit left: top it up, or switch provider")
    if status == 404 or code in ("model_not_found", "modelnotfound"):
        return ProviderError(
            f"the AI provider does not offer the model {model!r}: change it in Settings > AI provider"
        )
    return ProviderError(f"the AI provider answered with HTTP {status}")

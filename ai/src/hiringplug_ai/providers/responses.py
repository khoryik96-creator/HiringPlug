"""ChatGPT through OpenAI's Responses API (as CV Studio calls it)."""

from __future__ import annotations

import time
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

import httpx

from . import Completion, ProviderError
from .retry import REJECTED_BEFORE_WORK, RejectedError, with_retries

OPENAI_URL = "https://api.openai.com/v1/responses"

# GPT-5 models reason before answering, and reasoning counts against
# max_output_tokens: room is added so the answer is never cut short.
REASONING_ROOM = 8000


@dataclass(frozen=True)
class Responses:
    api_key: str = field(repr=False)
    model: str
    timeout_seconds: float = 90.0
    transport: httpx.BaseTransport | None = None  # tests
    sleep: Callable[[float], None] = time.sleep

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        body = {
            "model": self.model,
            "instructions": system,
            "input": user,
            "max_output_tokens": max_tokens + REASONING_ROOM,
            "store": False,  # CVs are not kept at OpenAI for later retrieval
        }
        headers = {"Content-Type": "application/json", "Authorization": f"Bearer {self.api_key}"}

        def send() -> httpx.Response:
            try:
                with httpx.Client(
                    timeout=self.timeout_seconds, transport=self.transport, follow_redirects=False
                ) as client:
                    response = client.post(OPENAI_URL, json=body, headers=headers)
            except httpx.TimeoutException as exc:
                raise ProviderError("the AI provider did not answer in time") from exc
            except httpx.HTTPError as exc:
                raise ProviderError("the AI provider could not be reached") from exc
            if response.status_code in REJECTED_BEFORE_WORK and not _no_credit(response):
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
        except ValueError as exc:
            raise ProviderError("the AI provider's answer could not be read") from exc
        if data.get("status") == "incomplete":
            raise ProviderError("the AI's answer was cut off (it ran past the length allowed)")
        usage = data.get("usage") or {}
        return Completion(
            text=_text(data).strip(),
            model=str(data.get("model") or self.model),
            input_tokens=int(usage.get("input_tokens") or 0),
            output_tokens=int(usage.get("output_tokens") or 0),
        )


def _text(data: dict[str, Any]) -> str:
    """The answer's text: output_text when given, else the message blocks."""
    if data.get("output_text"):
        return str(data["output_text"])
    parts: list[str] = []
    for item in data.get("output") or []:
        if not isinstance(item, dict):
            continue
        if item.get("type") == "message":
            for block in item.get("content") or []:
                if isinstance(block, dict) and block.get("type") in ("output_text", "text"):
                    parts.append(str(block.get("text") or ""))
        elif item.get("type") in ("output_text", "text"):
            parts.append(str(item.get("text") or ""))
    return "".join(parts)


def _code(response: httpx.Response) -> str:
    try:
        error = response.json().get("error") or {}
        return str(error.get("code") or error.get("type") or "").lower()
    except (ValueError, AttributeError):
        return ""


def _no_credit(response: httpx.Response) -> bool:
    return response.status_code == 402 or _code(response) == "insufficient_quota"


def _status_error(response: httpx.Response, model: str) -> ProviderError:
    """A fixed message per kind of failure (OpenAI's own text can echo part of the key)."""
    status, code = response.status_code, _code(response)
    if status in (401, 403):
        return ProviderError("the AI provider refused the API key: check it in Settings > AI provider")
    if _no_credit(response):
        return ProviderError("the AI provider account has no credit left: top it up, or switch provider")
    if status == 404 or code == "model_not_found":
        return ProviderError(
            f"the AI provider does not offer the model {model!r}: change it in Settings > AI provider"
        )
    return ProviderError(f"the AI provider answered with HTTP {status}")

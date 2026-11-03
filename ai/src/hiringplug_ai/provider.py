"""The AI provider, behind one small interface.

`OpenAICompatible` speaks the OpenAI-style chat API that DeepSeek's own API,
hosts running DeepSeek's open model (in a region you choose) and self-hosted
servers (vLLM, Ollama) all offer. Another provider is one more class with a
`complete` method. The provider's key is never logged.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

import httpx


@dataclass(frozen=True)
class Completion:
    text: str
    model: str
    input_tokens: int
    output_tokens: int


class ProviderError(RuntimeError):
    """The provider did not answer; the message is safe to show users."""


class Provider(Protocol):
    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion: ...


@dataclass(frozen=True)
class OpenAICompatible:
    base_url: str  # e.g. https://api.deepseek.com/v1
    api_key: str
    model: str
    timeout_seconds: float = 90.0
    transport: httpx.BaseTransport | None = None  # tests

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        body = {
            "model": self.model,
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
            "max_tokens": max_tokens,
            "temperature": 0.2,
            "stream": False,
        }
        try:
            with httpx.Client(
                timeout=self.timeout_seconds, transport=self.transport, follow_redirects=False
            ) as client:
                response = client.post(
                    self.base_url.rstrip("/") + "/chat/completions", json=body, headers=headers
                )
        except httpx.TimeoutException as exc:
            raise ProviderError("the AI provider did not answer in time") from exc
        except httpx.HTTPError as exc:
            raise ProviderError("the AI provider could not be reached") from exc
        if response.status_code == httpx.codes.TOO_MANY_REQUESTS:
            raise ProviderError("the AI provider is busy (rate limited); try again in a minute")
        if response.status_code in (httpx.codes.UNAUTHORIZED, httpx.codes.FORBIDDEN):
            raise ProviderError("the AI provider refused the key (check HP_AI_PROVIDER_KEY)")
        if response.status_code != httpx.codes.OK:
            raise ProviderError(f"the AI provider answered with HTTP {response.status_code}")
        try:
            data = response.json()
            text = data["choices"][0]["message"]["content"]
            usage = data.get("usage") or {}
            return Completion(
                text=str(text or "").strip(),
                model=str(data.get("model") or self.model),
                input_tokens=int(usage.get("prompt_tokens") or 0),
                output_tokens=int(usage.get("completion_tokens") or 0),
            )
        except (ValueError, KeyError, IndexError, TypeError) as exc:
            raise ProviderError("the AI provider's answer could not be read") from exc

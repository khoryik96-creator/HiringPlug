"""Qwen (Alibaba Cloud Model Studio) as a company's chosen provider."""

from __future__ import annotations

import base64
import hashlib
import json

import httpx
import pytest
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

from hiringplug_ai.contracts import AiAccess
from hiringplug_ai.providers import Completion, ProviderError, connect
from hiringplug_ai.providers.qwen import QWEN_URL, Qwen

SECRET = "test-secret-0123456789-0123456789-abcdef"
KEY = "sk-qwen-0123456789abcdef"


def seal(key: str, provider: str) -> str:
    """As Go's aiproviders.Seal does (a fixed nonce, for the test only)."""
    nonce = bytes(range(12))
    aes = AESGCM(hashlib.sha256(SECRET.encode()).digest())
    raw = nonce + aes.encrypt(nonce, key.encode(), f"hiringplug-ai-key:{provider}".encode())
    return "v1." + base64.urlsafe_b64encode(raw).decode().rstrip("=")


def qwen(responses: list[httpx.Response]) -> tuple[list[httpx.Request], httpx.MockTransport]:
    seen: list[httpx.Request] = []

    def handle(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return responses[min(len(seen), len(responses)) - 1]

    return seen, httpx.MockTransport(handle)


def answer(text: str = "Strong fit.", finish: str = "stop") -> httpx.Response:
    return httpx.Response(
        200,
        json={
            "model": "qwen-plus",
            "choices": [{"message": {"role": "assistant", "content": text}, "finish_reason": finish}],
            "usage": {"prompt_tokens": 40, "completion_tokens": 7},
        },
    )


def test_connect_builds_qwen_with_its_own_key() -> None:
    ai = connect(AiAccess(provider="qwen", model="qwen-plus", sealedKey=seal(KEY, "qwen")), SECRET, 30)
    assert isinstance(ai, Qwen)
    assert ai.api_key == KEY
    assert ai.model == "qwen-plus"
    assert KEY not in repr(ai)
    with pytest.raises(ProviderError, match="cannot be opened"):  # sealed for another provider
        connect(AiAccess(provider="qwen", model="qwen-plus", sealedKey=seal(KEY, "deepseek")), SECRET, 30)


def test_qwen_calls_model_studio_without_thinking() -> None:
    seen, transport = qwen([answer()])
    done = Qwen(KEY, "qwen-plus", transport=transport).complete("sys", "user", max_tokens=1000)
    assert done == Completion(text="Strong fit.", model="qwen-plus", input_tokens=40, output_tokens=7)
    [req] = seen
    assert str(req.url) == QWEN_URL
    assert req.url.host == "dashscope-intl.aliyuncs.com"
    assert req.headers["authorization"] == f"Bearer {KEY}"
    assert json.loads(req.content) == {
        "model": "qwen-plus",
        "messages": [{"role": "system", "content": "sys"}, {"role": "user", "content": "user"}],
        "max_tokens": 1000,
        "temperature": 0.2,
        "stream": False,
        "enable_thinking": False,
    }


def test_qwen_without_instructions_sends_the_prompt_alone() -> None:
    seen, transport = qwen([answer()])
    Qwen(KEY, "qwen-plus", transport=transport).complete("", "user", max_tokens=10)
    assert json.loads(seen[0].content)["messages"] == [{"role": "user", "content": "user"}]


def test_a_rate_limit_is_tried_again_but_no_credit_is_not() -> None:
    seen, transport = qwen([httpx.Response(429, json={"error": {"code": "Throttling"}}), answer("ok")])
    assert (
        Qwen(KEY, "qwen-plus", transport=transport, sleep=lambda s: None)
        .complete("s", "u", max_tokens=10)
        .text
        == "ok"
    )
    assert len(seen) == 2

    seen, transport = qwen(
        [httpx.Response(429, json={"error": {"code": "insufficient_quota", "message": KEY}})]
    )
    with pytest.raises(ProviderError, match="no credit") as exc:
        Qwen(KEY, "qwen-plus", transport=transport, sleep=lambda s: None).complete("s", "u", max_tokens=10)
    assert len(seen) == 1
    assert KEY not in str(exc.value)


def test_a_server_error_is_never_sent_twice() -> None:
    seen, transport = qwen([httpx.Response(500, json={"error": {"message": KEY}}), answer()])
    with pytest.raises(ProviderError, match="HTTP 500"):
        Qwen(KEY, "qwen-plus", transport=transport, sleep=lambda s: None).complete("s", "u", max_tokens=10)
    assert len(seen) == 1


@pytest.mark.parametrize(
    ("response", "wanted"),
    [
        (
            httpx.Response(401, json={"error": {"code": "invalid_api_key", "message": KEY}}),
            "refused the API key",
        ),
        (httpx.Response(400, json={"error": {"code": "Arrearage", "message": KEY}}), "no credit"),
        (
            httpx.Response(404, json={"error": {"code": "model_not_found"}}),
            "does not offer the model 'qwen-plus'",
        ),
        (answer("half", finish="length"), "cut off"),
        (httpx.Response(200, json={"choices": []}), "could not be read"),
    ],
)
def test_qwen_failures_are_told_plainly_and_never_echo_the_key(response: httpx.Response, wanted: str) -> None:
    _, transport = qwen([response])
    with pytest.raises(ProviderError, match=wanted) as exc:
        Qwen(KEY, "qwen-plus", transport=transport, sleep=lambda s: None).complete("s", "u", max_tokens=10)
    assert KEY not in str(exc.value)

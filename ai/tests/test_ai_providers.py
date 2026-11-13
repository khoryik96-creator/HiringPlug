"""The providers a company chooses in Settings > AI provider: sealed keys,
DeepSeek and Claude (Messages API), ChatGPT (Responses API), and the check."""

from __future__ import annotations

import json
from collections.abc import Callable
from typing import Any

import httpx
import httpx2
import pytest

from hiringplug_ai.contracts import (
    AiAccess,
    AiProviderCheckedJob,
    AiProviderCheckJob,
    AiProviderReportedJob,
    AssistantAskJob,
    JobKind,
)
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import HANDLERS, Deps, ProviderLog, ai_check, assistant_ask
from hiringplug_ai.jobs import Handler as JobHandler
from hiringplug_ai.keys import SealedKeyError, open_key
from hiringplug_ai.providers import Completion, ProviderError, connect
from hiringplug_ai.providers.messages import Messages
from hiringplug_ai.providers.responses import Responses
from hiringplug_ai.queue import ClaimedJob, FollowUp
from hiringplug_ai.worker import process

# The same vector as the Go side's TestSealMatchesTheWorkersVector: Go seals,
# this worker opens, and neither can drift.
SECRET = "test-secret-0123456789-0123456789-abcdef"
KEY = "sk-test-0123456789abcdef"
SEALED = "v1.AAECAwQFBgcICQoL0L_sahf3mXCUsEjZQevTMjksVWQCL7wDjTIRxzMizn_bZoPTlfISJw"


def test_opens_a_key_go_sealed() -> None:
    assert open_key(SEALED, "deepseek", SECRET) == KEY


@pytest.mark.parametrize(
    ("sealed", "provider", "secret"),
    [
        (SEALED, "claude", SECRET),  # sealed for another provider
        (SEALED, "deepseek", SECRET + "x"),  # another secret
        (SEALED[:-4], "deepseek", SECRET),  # damaged
        ("v2.abc", "deepseek", SECRET),  # unknown format
        (SEALED, "deepseek", ""),  # no secret set
    ],
)
def test_refuses_keys_it_cannot_open(sealed: str, provider: str, secret: str) -> None:
    with pytest.raises(SealedKeyError) as exc:
        open_key(sealed, provider, secret)
    assert KEY not in str(exc.value)


# --- Claude and DeepSeek (the Messages API, Anthropic's SDK) ---

Handler = Callable[[httpx2.Request], httpx2.Response]


def recording(responses: list[httpx2.Response]) -> tuple[list[httpx2.Request], httpx2.Client]:
    seen: list[httpx2.Request] = []

    def handle(request: httpx2.Request) -> httpx2.Response:
        seen.append(request)
        return responses[min(len(seen), len(responses)) - 1]

    return seen, httpx2.Client(transport=httpx2.MockTransport(handle))


def message(text: str = "OK", stop: str = "end_turn", model: str = "claude-opus-5-5") -> httpx2.Response:
    return httpx2.Response(
        200,
        json={
            "id": "msg_1",
            "type": "message",
            "role": "assistant",
            "model": model,
            "content": [{"type": "text", "text": text}],
            "stop_reason": stop,
            "stop_sequence": None,
            "usage": {"input_tokens": 12, "output_tokens": 3},
        },
    )


def error(
    status: int, body: dict[str, Any] | None = None, headers: dict[str, str] | None = None
) -> httpx2.Response:
    return httpx2.Response(
        status, json=body or {"type": "error", "error": {"type": "x", "message": KEY}}, headers=headers
    )


def test_claude_sends_the_key_and_takes_refusal_fallbacks() -> None:
    seen, client = recording([message("A strong fit.")])
    done = Messages("claude", KEY, "claude-opus-5-5", http_client=client).complete(
        "sys", "user", max_tokens=1500
    )
    assert done == Completion(text="A strong fit.", model="claude-opus-5-5", input_tokens=12, output_tokens=3)
    [req] = seen
    assert str(req.url).startswith("https://api.anthropic.com/v1/messages")
    assert req.headers["x-api-key"] == KEY
    assert "server-side-fallback-2026-07-01" in req.headers["anthropic-beta"]
    body = json.loads(req.content)
    assert body["fallbacks"] == "default"
    assert body["max_tokens"] == 1500 + 8000  # room to think before answering
    assert body["system"] == "sys"
    assert "temperature" not in body
    assert "thinking" not in body


def test_claude_models_without_fallbacks_use_the_plain_api() -> None:
    seen, client = recording([message(model="claude-haiku-5-5")])
    Messages("claude", KEY, "claude-haiku-5-5", http_client=client).complete("s", "u", max_tokens=20000)
    body = json.loads(seen[0].content)
    assert "fallbacks" not in body
    assert body["max_tokens"] == 20000  # capped
    assert "anthropic-beta" not in seen[0].headers


def test_deepseek_uses_its_anthropic_endpoint_without_thinking() -> None:
    seen, client = recording([message(model="deepseek-v4-flash")])
    Messages("deepseek", KEY, "deepseek-chat", http_client=client).complete("s", "u", max_tokens=900)
    [req] = seen
    assert str(req.url).startswith("https://api.deepseek.com/anthropic/v1/messages")
    body = json.loads(req.content)
    assert body["model"] == "deepseek-v4-flash"  # the retired name is mapped
    assert body["thinking"] == {"type": "disabled"}
    assert body["max_tokens"] == 900
    assert "fallbacks" not in body


def test_a_rate_limit_is_tried_again_but_a_server_error_is_not() -> None:
    sleeps: list[float] = []
    seen, client = recording([error(429, headers={"retry-after": "3"}), error(529), message("fine")])
    done = Messages("claude", KEY, "claude-opus-5-5", http_client=client, sleep=sleeps.append).complete(
        "s", "u", max_tokens=10
    )
    assert done.text == "fine"
    assert len(seen) == 3
    assert sleeps == [3.0, 2.0]

    # A 500 may have run the model already: sending it again could bill twice.
    seen, client = recording([error(500), message()])
    with pytest.raises(ProviderError, match="HTTP 500"):
        Messages("claude", KEY, "claude-opus-5-5", http_client=client, sleep=sleeps.append).complete(
            "s", "u", max_tokens=10
        )
    assert len(seen) == 1


def test_busy_after_three_tries() -> None:
    seen, client = recording([error(429)])
    with pytest.raises(ProviderError, match="busy"):
        Messages("claude", KEY, "claude-opus-5-5", http_client=client, sleep=lambda s: None).complete(
            "s", "u", max_tokens=10
        )
    assert len(seen) == 3


@pytest.mark.parametrize(
    ("response", "wanted"),
    [
        (error(401), "refused the API key"),
        (
            error(
                400, {"type": "error", "error": {"type": "x", "message": "Your credit balance is too low"}}
            ),
            "no credit",
        ),
        (error(402), "no credit"),
        (error(404), "does not offer the model 'claude-opus-5-5'"),
        (message(stop="refusal"), "declined"),
        (message(stop="max_tokens"), "cut off"),
    ],
)
def test_failures_are_told_plainly_and_never_echo_the_key(response: httpx2.Response, wanted: str) -> None:
    _, client = recording([response])
    with pytest.raises(ProviderError) as exc:
        Messages("claude", KEY, "claude-opus-5-5", http_client=client, sleep=lambda s: None).complete(
            "s", "u", max_tokens=10
        )
    assert wanted in str(exc.value)
    assert KEY not in str(exc.value)


# --- ChatGPT (OpenAI's Responses API) ---


def openai(responses: list[httpx.Response]) -> tuple[list[httpx.Request], httpx.MockTransport]:
    seen: list[httpx.Request] = []

    def handle(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return responses[min(len(seen), len(responses)) - 1]

    return seen, httpx.MockTransport(handle)


def test_chatgpt_reads_the_message_blocks() -> None:
    seen, transport = openai(
        [
            httpx.Response(
                200,
                json={
                    "model": "gpt-5.5",
                    "status": "completed",
                    "output": [
                        {"type": "reasoning", "summary": []},
                        {
                            "type": "message",
                            "content": [
                                {"type": "output_text", "text": "Strong "},
                                {"type": "output_text", "text": "fit."},
                            ],
                        },
                    ],
                    "usage": {"input_tokens": 40, "output_tokens": 7},
                },
            )
        ]
    )
    done = Responses(KEY, "gpt-5.5", transport=transport).complete("sys", "user", max_tokens=1000)
    assert done == Completion(text="Strong fit.", model="gpt-5.5", input_tokens=40, output_tokens=7)
    [req] = seen
    assert str(req.url) == "https://api.openai.com/v1/responses"
    assert req.headers["authorization"] == f"Bearer {KEY}"
    body = json.loads(req.content)
    assert body == {
        "model": "gpt-5.5",
        "instructions": "sys",
        "input": "user",
        "max_output_tokens": 9000,
        "store": False,
    }


def test_chatgpt_without_credit_is_not_tried_again() -> None:
    seen, transport = openai(
        [httpx.Response(429, json={"error": {"code": "insufficient_quota", "message": KEY}})]
    )
    with pytest.raises(ProviderError, match="no credit") as exc:
        Responses(KEY, "gpt-5.5", transport=transport, sleep=lambda s: None).complete("s", "u", max_tokens=10)
    assert len(seen) == 1
    assert KEY not in str(exc.value)


@pytest.mark.parametrize(
    ("response", "wanted"),
    [
        (httpx.Response(401, json={"error": {"message": KEY}}), "refused the API key"),
        (httpx.Response(404, json={"error": {"code": "model_not_found"}}), "does not offer the model"),
        (httpx.Response(200, json={"status": "incomplete", "output": []}), "cut off"),
    ],
)
def test_chatgpt_failures(response: httpx.Response, wanted: str) -> None:
    _, transport = openai([response])
    with pytest.raises(ProviderError, match=wanted):
        Responses(KEY, "gpt-5.5", transport=transport, sleep=lambda s: None).complete("s", "u", max_tokens=10)


# --- Choosing the provider for a job ---


def test_connect_builds_the_chosen_provider() -> None:
    access = AiAccess(provider="deepseek", model="deepseek-v4-flash", sealedKey=SEALED)
    ai = connect(access, SECRET, 30)
    assert isinstance(ai, Messages)
    assert ai.provider == "deepseek"
    assert ai.api_key == KEY
    assert KEY not in repr(ai)
    with pytest.raises(ProviderError, match="cannot be opened"):
        connect(access, SECRET + "-other", 30)


class Fake:
    def __init__(self, reply: Completion | ProviderError) -> None:
        self.reply, self.calls = reply, 0

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        self.calls += 1
        if isinstance(self.reply, ProviderError):
            raise self.reply
        return self.reply


OK = Completion(text="OK", model="m", input_tokens=1, output_tokens=1)


def deps(server: Fake | None = None, company: Fake | None = None) -> Deps:
    return Deps(
        download=lambda u, n: b"",
        ocr=None,
        limits=DEFAULT_LIMITS,
        max_download_bytes=1,
        ai=server,
        connect=(lambda access: company) if company else None,
    )


def test_a_job_uses_the_companys_provider_and_else_the_servers() -> None:
    server, company = Fake(OK), Fake(OK)
    access = AiAccess(provider="claude", model="claude-opus-5-5", sealedKey=SEALED)
    job = AssistantAskJob(
        answerId="ask_01jbz6w0m0k8q9x2r3s4t5v6w7", kind="fit", question=None, sections=[], ai=access
    )
    assistant_ask.run(job, deps(server, company))
    assert (server.calls, company.calls) == (0, 1)
    assistant_ask.run(job.model_copy(update={"ai": None}), deps(server, company))
    assert (server.calls, company.calls) == (1, 1)


def test_a_company_key_without_the_secret_is_an_answered_error() -> None:
    access = AiAccess(provider="claude", model="claude-opus-5-5", sealedKey=SEALED)
    job = AssistantAskJob(
        answerId="ask_01jbz6w0m0k8q9x2r3s4t5v6w7", kind="fit", question=None, sections=[], ai=access
    )
    [f] = assistant_ask.run(job, deps(Fake(OK)))
    assert "HP_AI_KEYS_SECRET" in (f.payload.error or "")  # type: ignore[attr-defined]


@pytest.mark.parametrize(
    ("reply", "ok", "message"),
    [
        (OK, True, None),
        (ProviderError("the AI provider refused the API key"), False, "the AI provider refused the API key"),
    ],
)
def test_the_check_reports_back(reply: Completion | ProviderError, ok: bool, message: str | None) -> None:
    access = AiAccess(provider="openai", model="gpt-5.5", sealedKey="v1.sealed")
    [f] = ai_check.run(AiProviderCheckJob(provider="openai", access=access), deps(company=Fake(reply)))
    assert f.queue == "core"
    assert f.kind == JobKind.aiprovider_checked
    r = AiProviderCheckedJob.model_validate(f.payload.model_dump(mode="json"))
    assert (r.ok, r.message, r.sealedKey, r.provider.value) == (ok, message, "v1.sealed", "openai")


def test_each_request_closes_its_connections() -> None:
    _, client = recording([message("fine")])
    Messages("claude", KEY, "claude-opus-5-5", http_client=client).complete("s", "u", max_tokens=10)
    assert client.is_closed, "a worker must not keep a connection pool open per call"


# The worker tells the app whether the provider answered each job's last
# call, so people see a notice when the AI is down.


class RecordingQueue:
    def __init__(self) -> None:
        self.completed: list[list[FollowUp]] = []
        self.failed: list[str] = []
        self.enqueued: list[FollowUp] = []

    def complete(self, job: ClaimedJob, follow_ups: list[FollowUp]) -> None:
        self.completed.append(follow_ups)

    def fail(self, job: ClaimedJob, error: str) -> str:
        self.failed.append(error)
        return "pending"

    def enqueue(self, job: ClaimedJob, follow_up: FollowUp) -> None:
        self.enqueued.append(follow_up)


def check_job() -> ClaimedJob:
    access = {"provider": "deepseek", "model": "deepseek-chat", "sealedKey": "v1.sealed"}
    return ClaimedJob(
        id="job_01jbz6w0m0k8q9x2r3s4t5v6w7",
        tenant_id="tnt_01jbz6w0m0k8q9x2r3s4t5v6w7",
        kind="aiprovider.check",
        payload={"provider": "deepseek", "access": access},
        attempts=1,
        max_attempts=5,
        request_id="req_test",
    )


def reported(f: FollowUp) -> AiProviderReportedJob:
    assert (f.queue, f.kind) == ("core", JobKind.aiprovider_reported)
    return AiProviderReportedJob.model_validate(f.payload.model_dump(mode="json"))


def test_a_job_whose_provider_fails_tells_the_app_why() -> None:
    queue = RecordingQueue()
    down = ProviderError("the AI provider could not be reached")
    process(queue, check_job(), deps(company=Fake(down)))  # type: ignore[arg-type]
    assert queue.failed == []
    [_result, report] = queue.completed[0]
    r = reported(report)
    assert (r.provider, r.ok, r.error) == ("deepseek", False, "the AI provider could not be reached")


def test_a_job_whose_provider_answers_says_so_and_clears_the_notice() -> None:
    queue = RecordingQueue()
    process(queue, check_job(), deps(company=Fake(OK)))  # type: ignore[arg-type]
    r = reported(queue.completed[0][-1])
    assert (r.provider, r.ok, r.error) == ("deepseek", True, None)


def test_a_job_that_calls_no_provider_reports_nothing() -> None:
    log = ProviderLog()
    assert log.report() is None


def test_a_key_that_cannot_be_opened_is_reported_too() -> None:
    queue = RecordingQueue()
    process(queue, check_job(), deps())  # type: ignore[arg-type]  # no HP_AI_KEYS_SECRET: connect is None
    r = reported(queue.completed[0][-1])
    assert r.ok is False
    assert "HP_AI_KEYS_SECRET" in (r.error or "")


def test_a_job_that_dies_on_the_provider_still_tells_the_app(monkeypatch: pytest.MonkeyPatch) -> None:
    """A handler that lets the provider's error out fails the job (it is
    retried); the app is told at once, not after the last retry."""

    def run(payload: AiProviderCheckJob, deps: Deps) -> list[FollowUp]:
        ai = deps.provider(payload.access)
        assert ai is not None
        ai.complete("s", "u", max_tokens=1)
        return []

    monkeypatch.setitem(HANDLERS, JobKind.aiprovider_check, JobHandler(AiProviderCheckJob, run))
    queue = RecordingQueue()
    down = ProviderError("the AI provider did not answer in time")
    process(queue, check_job(), deps(company=Fake(down)))  # type: ignore[arg-type]
    assert queue.completed == []
    assert len(queue.failed) == 1
    [report] = queue.enqueued
    assert reported(report).error == "the AI provider did not answer in time"

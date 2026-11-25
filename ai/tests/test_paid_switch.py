"""IMP-001: the server's paid-AI switch, at the AI worker's end. Paused,
no paid call is made, through a company's key or the server's provider,
for jobs queued before it too; CVs are still read, by the rules. Fake
providers only: no test ever calls a paid AI."""

from __future__ import annotations

import dataclasses
from typing import Any

import psycopg
import pytest

from hiringplug_ai.contracts import (
    AiAccess,
    AiProviderCheckedJob,
    AiProviderCheckJob,
    AssistantAskJob,
    DocumentCvParsedJob,
    DocumentParseCvJob,
)
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, ProviderLog, ai_check, assistant_ask
from hiringplug_ai.jobs import parse_cv as parse_job
from hiringplug_ai.paid_switch import PaidAIPausedError, PaidSwitch
from hiringplug_ai.providers import Completion

from .test_queue_db import database  # noqa: F401 - the real database, as the AI role

OK = Completion(text="OK", model="m", input_tokens=1, output_tokens=1)
ACCESS = AiAccess(provider="claude", model="claude-opus-5-5", sealedKey="v1.synthetic")
DOC_ID = "doc_01jbz6w0m0k8q9x2r3s4t5v6w7"


class Fake:
    def __init__(self) -> None:
        self.calls = 0

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        self.calls += 1
        return OK


def deps(paused: bool, server: Fake, company: Fake) -> Deps:
    return Deps(
        download=lambda u, n: b"",
        ocr=None,
        limits=DEFAULT_LIMITS,
        max_download_bytes=1,
        ai=server,
        connect=lambda access: company,
        paid_paused=lambda: paused,
    )


def ask(access: AiAccess | None) -> AssistantAskJob:
    return AssistantAskJob(
        answerId="ask_01jbz6w0m0k8q9x2r3s4t5v6w7", kind="fit", question=None, sections=[], ai=access
    )


def test_paused_no_provider_is_called_neither_the_companys_nor_the_servers() -> None:
    server, company = Fake(), Fake()
    for access in (ACCESS, None):
        [answer] = assistant_ask.run(ask(access), deps(True, server, company))
        assert "paid AI is paused" in (answer.payload.error or "")  # type: ignore[attr-defined]
    assert (server.calls, company.calls) == (0, 0)
    with pytest.raises(PaidAIPausedError):
        deps(True, server, company).provider(ACCESS)
    assistant_ask.run(ask(ACCESS), deps(False, server, company))
    assert company.calls == 1, "back on: the company's key answers"


# Review of 1.47.0: the switch was read once per job, so a job of many
# calls (ranking a long list) went on calling for minutes after a pause.
# It is read before each call (cached for seconds), and a paused call is
# never noted as the provider failing.
def test_a_job_of_many_calls_stops_within_seconds_of_a_pause() -> None:
    company = Fake()
    paused = [False]
    log = ProviderLog()
    d = dataclasses.replace(deps(False, Fake(), company), paid_paused=lambda: paused[0], log=log)
    ai = d.provider(ACCESS)
    assert ai is not None
    ai.complete("s", "first", max_tokens=8)
    paused[0] = True
    with pytest.raises(PaidAIPausedError):
        ai.complete("s", "second", max_tokens=8)
    assert company.calls == 1, "the second call never reached the provider"
    assert log.ok is True, "a pause is not an outage"


# Review of 1.47.0: a key check queued before the pause, run during it,
# reported the key as failed ("Not working"). It reports "paused".
def test_a_key_check_during_a_pause_reports_paused_not_failed() -> None:
    payload = AiProviderCheckJob(provider="claude", access=ACCESS, checkId="chk_1", askedBy="usr_1")
    [result] = ai_check.run(payload, deps(True, Fake(), Fake()))
    assert isinstance(result.payload, AiProviderCheckedJob)
    assert result.payload.paused is True
    assert result.payload.ok is False
    [fine] = ai_check.run(payload, deps(False, Fake(), Fake()))
    assert isinstance(fine.payload, AiProviderCheckedJob)
    assert fine.payload.ok is True
    assert not fine.payload.paused


def test_paused_a_cv_is_still_read_by_the_rules() -> None:
    server, company = Fake(), Fake()
    text = "Nur Aisyah binti Rahman\nnur@example.com\nSKILLS\nExcel, SAP\n"
    payload = DocumentParseCvJob(documentId=DOC_ID, text=text, useAi=True, aiText=text, ai=ACCESS)
    [result] = parse_job.run(payload, deps(True, server, company))
    assert isinstance(result.payload, DocumentCvParsedJob)
    assert result.payload.profile is not None
    assert result.payload.profile.email is not None
    assert (result.payload.aiTokens or 0) == 0
    assert (server.calls, company.calls) == (0, 0)


class FakeConn:
    def __init__(self, answers: list[Any]) -> None:
        self.answers, self.queries, self.autocommit = answers, 0, False

    def execute(self, query: str) -> FakeConn:
        self.queries += 1
        answer = self.answers.pop(0)
        if isinstance(answer, Exception):
            raise answer
        self.row = (answer,)
        return self

    def fetchone(self) -> tuple[Any]:
        return self.row

    def close(self) -> None:
        pass


def test_the_switch_is_read_at_most_every_few_seconds() -> None:
    now = [0.0]
    conn = FakeConn([False, True])
    switch = PaidSwitch("postgres://x", ttl=5, clock=lambda: now[0], connect=lambda url: conn)
    assert switch.paused() is False
    now[0] = 4.9
    assert switch.paused() is False
    assert conn.queries == 1, "cached"
    now[0] = 5.0
    assert switch.paused() is True, "paused by the operator: seen within seconds"


def test_an_unreadable_switch_keeps_its_last_answer_and_is_paused_when_never_read() -> None:
    now = [0.0]
    conn = FakeConn([False, psycopg.OperationalError("down")])
    switch = PaidSwitch("postgres://x", ttl=1, clock=lambda: now[0], connect=lambda url: conn)
    assert switch.paused() is False
    now[0] = 2
    assert switch.paused() is False, "the last answer stands"
    never = PaidSwitch("postgres://x", connect=lambda url: FakeConn([psycopg.OperationalError("down")]))
    assert never.paused() is True, "never read: no paid call"


def test_hp_ai_paid_off_pauses_whatever_the_database_says() -> None:
    def no_database(url: str) -> Any:
        raise AssertionError("not asked")

    assert PaidSwitch("postgres://x", env_off=True, connect=no_database).paused() is True


@pytest.mark.db
def test_the_ai_role_reads_the_switch(database: dict[str, str]) -> None:  # noqa: F811
    switch = PaidSwitch(database["ai"], ttl=0)
    assert switch.paused() is False
    with psycopg.connect(database["admin"], autocommit=True) as admin:
        admin.execute("UPDATE ai_paid_control SET paused = true")
    try:
        assert switch.paused() is True
    finally:
        with psycopg.connect(database["admin"], autocommit=True) as admin:
            admin.execute("UPDATE ai_paid_control SET paused = false")

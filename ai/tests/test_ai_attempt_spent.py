"""A failed attempt's AI use is counted: the provider charged what it answered."""

from __future__ import annotations

import pytest

from hiringplug_ai.contracts import AiAttemptSpentJob, AiProviderCheckJob, JobKind
from hiringplug_ai.jobs import HANDLERS, Deps, ProviderLog, ReportGate
from hiringplug_ai.jobs import Handler as JobHandler
from hiringplug_ai.providers import Completion
from hiringplug_ai.queue import FollowUp
from hiringplug_ai.worker import process

from .test_ai_providers import Fake, RecordingQueue, ai_job, deps

SPENT = Completion(text="not what was asked for", model="m", input_tokens=1200, output_tokens=300)


@pytest.fixture
def answered_then_failed(monkeypatch: pytest.MonkeyPatch) -> None:
    """An AI job whose provider answers twice, then whose reply cannot be used."""

    def run(payload: AiProviderCheckJob, deps: Deps) -> list[FollowUp]:
        ai = deps.provider(payload.access)
        assert ai is not None
        ai.complete("s", "u", max_tokens=1)
        ai.complete("s", "u again", max_tokens=1)
        raise ValueError("the reply could not be read")

    monkeypatch.setitem(HANDLERS, JobKind.aiprovider_check, JobHandler(AiProviderCheckJob, run))


def spent(f: FollowUp) -> AiAttemptSpentJob:
    assert (f.queue, f.kind) == ("core", JobKind.ai_attempt_spent)
    return AiAttemptSpentJob.model_validate(f.payload.model_dump(mode="json"))


@pytest.mark.usefixtures("answered_then_failed")
def test_an_attempt_that_failed_after_the_ai_answered_tells_what_it_spent() -> None:
    queue = RecordingQueue()
    job = ai_job()
    process(queue, job, deps(company=Fake(SPENT)), gate=ReportGate())  # type: ignore[arg-type]
    assert queue.completed == []
    assert len(queue.failed) == 1, "the job is retried"
    reports = [spent(f) for f in queue.enqueued if f.kind == JobKind.ai_attempt_spent]
    assert len(reports) == 1
    r = reports[0]
    assert (r.jobId, r.jobKind, r.attempt) == (job.id, job.kind, job.attempts)
    assert (r.inputTokens, r.outputTokens) == (2400, 600), "both calls of the attempt"


def test_an_attempt_that_spent_nothing_tells_nothing() -> None:
    assert ProviderLog().spent_report("job_1", "aiprovider.check", 1) is None

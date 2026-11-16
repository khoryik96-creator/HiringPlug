from __future__ import annotations

from dataclasses import dataclass, field

from hiringplug_ai.contracts import AssistantEmailedJob, AssistantEmailJob, JobKind
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, assistant_email
from hiringplug_ai.providers import Completion, ProviderError

DRAFT_ID = "aem_01jbz6w0m0k8q9x2r3s4t5v6w7"


def job(**over: object) -> AssistantEmailJob:
    base: dict[str, object] = {
        "draftId": DRAFT_ID,
        "mode": "write",
        "recipient": "candidate",
        "agency": "Hyppies",
    }
    base.update(over)
    return AssistantEmailJob.model_validate(base)


@dataclass
class FakeProvider:
    reply: Completion | ProviderError
    calls: list[tuple[str, str]] = field(default_factory=list)

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        self.calls.append((system, user))
        if isinstance(self.reply, ProviderError):
            raise self.reply
        return self.reply


def deps(ai: FakeProvider | None) -> Deps:
    return Deps(download=lambda u, n: b"", ocr=None, limits=DEFAULT_LIMITS, max_download_bytes=1, ai=ai)


def result(follow_ups: list) -> AssistantEmailedJob:  # type: ignore[type-arg]
    [f] = follow_ups
    assert f.queue == "core"
    assert f.kind == JobKind.assistant_emailed
    return AssistantEmailedJob.model_validate(f.payload.model_dump(mode="json"))


def test_a_written_email_comes_back_as_subject_and_body() -> None:
    ai = FakeProvider(
        Completion(
            text="Subject: Interview for ⟦1⟧\n\nHi ⟦NAME⟧,\n\n\n\nCould we talk on Thursday?\n\nBest,\n⟦ME⟧",
            model="deepseek-chat",
            input_tokens=300,
            output_tokens=60,
        )
    )
    r = result(
        assistant_email.run(
            job(
                instruction="invite her to interview Thursday",
                jobTitle="Finance Manager",
                clientCompany="Maju Foods",
            ),
            deps(ai),
        )
    )
    assert r.subject == "Interview for ⟦1⟧"
    assert r.body == "Hi ⟦NAME⟧,\n\nCould we talk on Thursday?\n\nBest,\n⟦ME⟧"
    assert (r.model, r.inputTokens, r.outputTokens, r.error) == ("deepseek-chat", 300, 60, None)
    system, user = ai.calls[0]
    assert "copy every placeholder" in system
    assert "Write an email to a candidate" in user
    assert "invite her to interview Thursday" in user
    assert "About the job: Finance Manager at Maju Foods" in user
    assert "Hyppies" in user


def test_a_draft_is_improved_in_the_style_chosen() -> None:
    ai = FakeProvider(
        Completion(
            text="Subject: Update\n\nDear ⟦NAME⟧, thank you.", model="m", input_tokens=1, output_tokens=1
        )
    )
    result(
        assistant_email.run(
            job(mode="improve", style="grammar", draft="dear ⟦NAME⟧ thank you", recipient="client"), deps(ai)
        )
    )
    _, user = ai.calls[0]
    assert "Correct the spelling, grammar and punctuation only" in user
    assert "draft email to a client" in user
    assert "<<< The recruiter's draft >>>\ndear ⟦NAME⟧ thank you" in user


def test_a_reply_answers_their_email() -> None:
    ai = FakeProvider(
        Completion(text="Hi ⟦NAME⟧,\n\nThanks, noted.", model="m", input_tokens=1, output_tokens=1)
    )
    r = result(
        assistant_email.run(
            job(mode="reply", theirMessage="Can we move the call to 3pm?", instruction="yes, 3pm works"),
            deps(ai),
        )
    )
    assert r.subject is None, "no subject line: the email's own stays"
    assert r.body == "Hi ⟦NAME⟧,\n\nThanks, noted."
    _, user = ai.calls[0]
    assert "<<< Their email >>>\nCan we move the call to 3pm?" in user
    assert "yes, 3pm works" in user


def test_why_nothing_was_written_goes_back() -> None:
    r = result(
        assistant_email.run(
            job(instruction="x"), deps(FakeProvider(ProviderError("the AI provider timed out")))
        )
    )
    assert (r.body, r.error) == (None, "the AI provider timed out")
    r = result(assistant_email.run(job(instruction="x"), deps(None)))
    assert r.error is not None
    assert "switched off" in r.error
    r = result(
        assistant_email.run(
            job(instruction="x"),
            deps(FakeProvider(Completion(text="  ", model="m", input_tokens=5, output_tokens=0))),
        )
    )
    assert (r.error, r.inputTokens) == ("the AI wrote nothing", 5)


def test_the_subject_line_is_found_however_the_ai_writes_it() -> None:
    for text in (
        "Subject: Interview on Thursday\n\nHi ⟦NAME⟧,",
        "**Subject:** Interview on Thursday\n\nHi ⟦NAME⟧,",
        "Subject - Interview on Thursday\n\nHi ⟦NAME⟧,",
        "\n\n# Subject: Interview on Thursday\nHi ⟦NAME⟧,",
        "**Subject: Interview on Thursday**\n\nHi ⟦NAME⟧,",
    ):
        assert assistant_email.split(text) == ("Interview on Thursday", "Hi ⟦NAME⟧,"), text
    assert assistant_email.split("Hi ⟦NAME⟧,\n\nThe subject: your notice period.") == (
        None,
        "Hi ⟦NAME⟧,\n\nThe subject: your notice period.",
    )

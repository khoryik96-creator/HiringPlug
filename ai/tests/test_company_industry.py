from __future__ import annotations

from dataclasses import dataclass, field

from hiringplug_ai.contracts import CompanyIndustriedJob, CompanyIndustryJob, JobKind
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, company_industry
from hiringplug_ai.providers import Completion, ProviderError

PICK = "cip_01jbz6w0m0k8q9x2r3s4t5v6w7"
BANK = "industry_01jbz6w0m0k8q9x2r3s4t5v6w1"
LOGISTICS = "industry_01jbz6w0m0k8q9x2r3s4t5v6w2"


def job(**over: object) -> CompanyIndustryJob:
    base: dict[str, object] = {
        "pickId": PICK,
        "companyName": "Maju Logistik Sdn Bhd",
        "website": "https://maju.example.my",
        "about": "We move goods across the peninsula with 300 trucks.",
        "hint": "Truck Transportation",
        "choices": [
            {"id": BANK, "label": "Financial Services > Banking"},
            {"id": LOGISTICS, "label": "Industrial/Manufacturing > Logistics"},
        ],
    }
    base.update(over)
    return CompanyIndustryJob.model_validate(base)


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


def answered(text: str) -> tuple[CompanyIndustriedJob, FakeProvider]:
    ai = FakeProvider(Completion(text=text, model="deepseek-chat", input_tokens=120, output_tokens=2))
    out = company_industry.run(job(), deps(ai))
    assert len(out) == 1
    assert out[0].kind == JobKind.company_industried
    assert out[0].queue == "core"
    payload = out[0].payload
    assert isinstance(payload, CompanyIndustriedJob)
    return payload, ai


def test_the_ai_picks_from_the_numbered_list() -> None:
    got, ai = answered("2")
    assert got.choiceId is not None
    assert got.choiceId.root == LOGISTICS
    assert got.error is None
    assert (got.inputTokens, got.outputTokens, got.model) == (120, 2, "deepseek-chat")
    _, user = ai.calls[0]
    assert "1. Financial Services > Banking" in user
    assert "2. Industrial/Manufacturing > Logistics" in user
    assert "Truck Transportation" in user
    assert "300 trucks" in user


def test_none_or_an_answer_off_the_list_picks_nothing() -> None:
    for text in ("0", "7", "I am not sure", ""):
        got, _ = answered(text)
        assert got.choiceId is None, text
        assert got.error is None


def test_the_number_is_found_in_a_wordier_answer() -> None:
    got, _ = answered("The best fit is 1.")
    assert got.choiceId is not None
    assert got.choiceId.root == BANK


def test_a_provider_failure_or_no_ai_is_sent_back_as_the_reason() -> None:
    out = company_industry.run(job(), deps(FakeProvider(ProviderError("the AI provider timed out"))))
    failed = out[0].payload
    assert isinstance(failed, CompanyIndustriedJob)
    assert failed.choiceId is None
    assert failed.error == "the AI provider timed out"
    off = company_industry.run(job(), deps(None)).pop().payload
    assert isinstance(off, CompanyIndustriedJob)
    assert off.error is not None

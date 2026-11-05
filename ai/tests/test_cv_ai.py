from __future__ import annotations

import json
from dataclasses import dataclass, field

from hiringplug_ai.contracts import DocumentCvParsedJob, DocumentParseCvJob
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, cv_ai, parse_cv
from hiringplug_ai.parsing import parse_cv as rules
from hiringplug_ai.provider import Completion, ProviderError

FULL_CV = (
    "SITI AISYAH\nsiti@example.com | +60 12-345 6789\n\n"
    "EXPERIENCE\nSenior Accountant | Contoh Sdn Bhd\nJan 2020 - Present\n"
)
THIN_CV = "Profile\nI keep the books at Contoh Sdn Bhd as a Senior Accountant.\nReach me: siti@example.com\n"


@dataclass
class FakeAI:
    reply: object
    calls: list[tuple[str, str]] = field(default_factory=list)

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        self.calls.append((system, user))
        if isinstance(self.reply, ProviderError):
            raise self.reply
        return Completion(text=json.dumps(self.reply), model="m", input_tokens=400, output_tokens=60)


def test_core_found_asks_only_for_the_industry() -> None:
    profile = rules(FULL_CV)
    assert cv_ai.core_found(profile)
    assert profile.industry is None, "the rules know nothing of Contoh"
    ai = FakeAI({"industry": {"value": "Professional Services", "quote": "Contoh Sdn Bhd"}})
    got = cv_ai.fill(profile, FULL_CV, ai)
    [(system, _)] = ai.calls
    assert system == cv_ai.INDUSTRY_SYSTEM, "Kano's gate: no full read when the core is there"
    assert got.profile.industry is not None
    assert got.profile.industry.value == "Professional Services"
    assert got.profile.parser == "rules@1+ai"
    assert got.tokens == 460


def test_nothing_to_ask_asks_nothing() -> None:
    profile = rules(FULL_CV.replace("Contoh Sdn Bhd", "Maybank Berhad"))
    ai = FakeAI({})
    got = cv_ai.fill(profile, FULL_CV, ai)
    assert ai.calls == []
    assert got.tokens == 0


def test_a_thin_reading_is_filled_but_never_overwritten() -> None:
    profile = rules(THIN_CV)
    assert not cv_ai.core_found(profile)
    ai = FakeAI(
        {
            "fullName": {"value": "Siti Aisyah", "quote": "Siti Aisyah"},
            "email": {"value": "other@example.com", "quote": "other@example.com"},
            "currentTitle": {"value": "Senior Accountant", "quote": "Senior Accountant"},
            "currentCompany": {"value": "Contoh Sdn Bhd", "quote": "Contoh Sdn Bhd"},
            "industry": {"value": "Space Travel", "quote": "Contoh Sdn Bhd"},
            "phone": "not sourced",
        }
    )
    got = cv_ai.fill(profile, THIN_CV, ai).profile
    assert ai.calls[0][0] == cv_ai.FULL_SYSTEM
    assert got.fullName is not None
    assert got.fullName.value == "Siti Aisyah"
    assert got.email is not None
    assert got.email.value == "siti@example.com", "the rules' email stands"
    assert got.phone is None, "a value without its quote is dropped"
    assert got.industry is None, "only Kano's industries"
    job = got.experience[0]
    assert (job.title and job.title.value, job.company and job.company.value) == (
        "Senior Accountant",
        "Contoh Sdn Bhd",
    )


def test_an_unexpected_ai_error_keeps_the_rules_reading() -> None:
    class Broken:
        def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
            raise RuntimeError("bug")

    deps = Deps(download=lambda u, n: b"", ocr=None, limits=DEFAULT_LIMITS, max_download_bytes=1, ai=Broken())
    payload = {
        "documentId": "doc_01jbz6w0m0k8q9x2r3s4t5v6w7",
        "text": FULL_CV,
        "useAi": True,
        "aiText": FULL_CV,
    }
    [result] = parse_cv.run(DocumentParseCvJob.model_validate(payload), deps)
    assert isinstance(result.payload, DocumentCvParsedJob)
    assert result.payload.errorMessage is None
    assert result.payload.profile is not None
    assert result.payload.profile.fullName is not None


def test_a_failing_provider_leaves_the_rules_reading() -> None:
    profile = rules(THIN_CV)
    got = cv_ai.fill(profile, THIN_CV, FakeAI(ProviderError("down")))
    assert got.profile == profile
    assert got.tokens == 0


def test_the_job_uses_the_ai_only_when_go_allows_it() -> None:
    ai = FakeAI({"industry": {"value": "Professional Services", "quote": "Contoh Sdn Bhd"}})
    deps = Deps(download=lambda u, n: b"", ocr=None, limits=DEFAULT_LIMITS, max_download_bytes=1, ai=ai)
    payload = {"documentId": "doc_01jbz6w0m0k8q9x2r3s4t5v6w7", "text": FULL_CV}

    [off] = parse_cv.run(DocumentParseCvJob.model_validate(payload), deps)
    assert ai.calls == []
    assert isinstance(off.payload, DocumentCvParsedJob)
    assert off.payload.aiTokens == 0

    salaried = FULL_CV + "Expected salary: RM 8,500\n"
    ai_text = FULL_CV + "Expected [salary removed]\n"  # as Go sends it
    with_ai: dict[str, object] = {**payload, "text": salaried, "useAi": True}
    parse_cv.run(DocumentParseCvJob.model_validate(with_ai), deps)
    assert ai.calls == [], "without the salary-free copy the AI is not asked"

    [on] = parse_cv.run(DocumentParseCvJob.model_validate(with_ai | {"aiText": ai_text}), deps)
    result = DocumentCvParsedJob.model_validate(on.payload.model_dump(mode="json"))
    assert result.aiTokens == 460
    [(_, prompt)] = ai.calls
    assert "8,500" not in prompt, "salary never goes to the AI"
    assert result.profile is not None
    assert result.profile.industry is not None

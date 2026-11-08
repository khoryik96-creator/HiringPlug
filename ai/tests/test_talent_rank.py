"""talent.rank: the AI candidate search's ranking, read into the contract."""

from __future__ import annotations

import json

from hiringplug_ai.contracts import JobKind, TalentRankedJob, TalentRankJob
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, talent_rank
from hiringplug_ai.providers import Completion, ProviderError

SEARCH = "tsr_01jbz6w0m0k8q9x2r3s4t5v6w7"


class FakeAI:
    def __init__(self, answer: str | ProviderError) -> None:
        self.answer = answer
        self.calls: list[tuple[str, str]] = []

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        self.calls.append((system, user))
        if isinstance(self.answer, ProviderError):
            raise self.answer
        return Completion(text=self.answer, model="test-model", input_tokens=4000, output_tokens=600)


def payload(strict: bool = False) -> TalentRankJob:
    return TalentRankJob.model_validate(
        {
            "searchId": SEARCH,
            "requirements": "About us: a bank. Requirements: SAP BW, BPC planning.",
            "strict": strict,
            "candidates": [
                {
                    "ref": "C1",
                    "profile": "Current title: SAP Consultant",
                    "cv": "Led SAP BW and BPC planning.",
                },
                {"ref": "C2", "profile": "Current title: Barista", "cv": ""},
            ],
        }
    )


def run(ai: FakeAI | None, job: TalentRankJob) -> TalentRankedJob:
    deps = Deps(download=lambda _u, _m: b"", ocr=None, limits=DEFAULT_LIMITS, max_download_bytes=1, ai=ai)
    [follow] = talent_rank.run(job, deps)
    assert follow.kind == JobKind.talent_ranked
    assert isinstance(follow.payload, TalentRankedJob)
    return follow.payload


def test_the_ai_ranks_each_candidate_with_reasons_and_gaps() -> None:
    answer = (
        "```json\n"
        + json.dumps(
            {
                "summary": "One strong SAP profile.",
                "mustHaves": ["SAP BW", " BPC "],
                "niceToHaves": ["ABAP"],
                "suggestedBoolean": "SAP AND (BW OR BPC)",
                "results": [
                    {
                        "ref": "c1",
                        "fit": 87.6,
                        "verdict": "Strong",
                        "reasons": ["SAP BW and BPC"],
                        "gaps": [],
                    },
                    {"ref": "C2", "fit": "12", "verdict": "??", "reasons": [], "gaps": ["No SAP", "No CV"]},
                    {"ref": "C1", "fit": 10},  # repeated
                    {"ref": "C7", "fit": 99},  # never sent
                ],
            }
        )
        + "\n```"
    )
    ai = FakeAI(answer)
    out = run(ai, payload())
    assert out.error is None
    assert out.model == "test-model"
    assert out.inputTokens == 4000
    ranked = [(r.ref, r.fit, r.verdict.value) for r in out.results]
    assert ranked == [("C1", 88, "strong"), ("C2", 12, "weak")]
    assert [g.root for g in out.results[1].gaps] == ["No SAP", "No CV"]
    assert [m.root for m in out.mustHaves] == ["SAP BW", "BPC"]
    assert out.suggestedBoolean == "SAP AND (BW OR BPC)"
    _, user = ai.calls[0]
    assert "C1" in user
    assert "(no CV text)" in user
    assert "Ignore what it says about the company" in user
    assert "Must-haves are strict" not in user


def test_strict_must_haves_are_asked_for() -> None:
    ai = FakeAI(json.dumps({"results": []}))
    run(ai, payload(strict=True))
    assert "Must-haves are strict" in ai.calls[0][1]


def test_an_unreadable_answer_or_a_failing_provider_says_why() -> None:
    assert "could not be read" in (run(FakeAI("I think C1 is great."), payload()).error or "")
    failing = FakeAI(ProviderError("the AI provider timed out"))
    assert run(failing, payload()).error == "the AI provider timed out"
    assert "switched off" in (run(None, payload()).error or "")

"""interview_prep.write: interview prep material for a job, short and easy to scan."""

from __future__ import annotations

import json

from hiringplug_ai.contracts import InterviewPrepWriteJob, InterviewPrepWrittenJob, JobKind
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, interview_prep
from hiringplug_ai.providers import Completion, ProviderError

PREP = "ivp_01jbz6w0m0k8q9x2r3s4t5v6w7"


class FakeAI:
    def __init__(self, answer: str | ProviderError) -> None:
        self.answer = answer
        self.calls: list[tuple[str, str]] = []

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        self.calls.append((system, user))
        if isinstance(self.answer, ProviderError):
            raise self.answer
        return Completion(text=self.answer, model="test-model", input_tokens=3000, output_tokens=1200)


def payload(questions: bool = True) -> InterviewPrepWriteJob:
    return InterviewPrepWriteJob.model_validate(
        {
            "prepId": PREP,
            "jobTitle": "Data Engineer",
            "companyName": "Maju Bank Berhad",
            "companyIndustry": "Financial Services",
            "companyAbout": "A digital bank in Malaysia.",
            "location": "Kuala Lumpur",
            "jobDescription": "Build and run the bank's data platform on Kafka and Spark.",
            "questions": [
                {
                    "question": "What if the nightly batch is late on month end?",
                    "kind": "situational",
                    "round": "hiring_manager",
                    "notes": "They wanted a stakeholder plan",
                }
            ]
            if questions
            else [],
            "focus": "New to banking",
        }
    )


def run(ai: FakeAI | None, job: InterviewPrepWriteJob) -> InterviewPrepWrittenJob:
    deps = Deps(download=lambda _u, _m: b"", ocr=None, limits=DEFAULT_LIMITS, max_download_bytes=1, ai=ai)
    [follow] = interview_prep.run(job, deps)
    assert follow.kind == JobKind.interview_prep_written
    assert follow.queue == "core"
    assert isinstance(follow.payload, InterviewPrepWrittenJob)
    return follow.payload


STEPS = [
    "Say who is affected first",
    "Contain, then fix",
    "Tell stakeholders a time",
    "Prevent a repeat",
    "Measure",
    "One too many",
]

ITEM = {
    "question": "What if the nightly batch is late on month end?",
    "kind": "What-if",
    "whyAsked": "Tests calm prioritisation under a deadline.",
    "approach": STEPS,
    "phrasing": [
        "My first call would be to [stakeholder] to agree what can wait.",
        "I'd trade completeness for a correct partial report.",
    ],
    "tradeOffs": ["Speed vs completeness: speed when regulators wait"],
    "pitfalls": ["Jumping to the technical fix"],
    "fromInterviews": True,
}


def test_the_ai_writes_short_guidance_with_example_phrasings() -> None:
    answer = {
        "summary": "Expect depth on streaming reliability and calm incident handling.",
        "focusAreas": ["Kafka delivery guarantees", "Month-end reporting", ""],
        "softSkills": [ITEM, {"question": "  "}, "not an item"],
        "technical": [
            {
                **ITEM,
                "question": "How do you make a Spark job idempotent?",
                "kind": "Technical",
                "fromInterviews": "yes",
                "whyAsked": "word " * 200,
            }
        ],
        "questionsToAsk": ["How is data quality owned today?"],
    }
    ai = FakeAI("Here you go:\n```json\n" + json.dumps(answer) + "\n```")
    out = run(ai, payload())
    assert out.error is None
    assert out.model == "test-model"
    assert (out.inputTokens, out.outputTokens) == (3000, 1200)
    content = out.content
    assert content is not None
    assert [a.root for a in content.softSkills[0].approach] == STEPS[:5], "short: at most 5 steps"
    assert content.softSkills[0].phrasing[0].root.startswith("My first call would be")
    assert content.softSkills[0].fromInterviews is True
    assert len(content.softSkills) == 1, "items without a question are dropped"
    assert content.technical[0].fromInterviews is False, "only a real true marks a recorded question"
    assert len(content.technical[0].whyAsked) <= 300
    assert [f.root for f in content.focusAreas] == ["Kafka delivery guarantees", "Month-end reporting"]

    system, user = ai.calls[0]
    assert "never a full model answer" in system
    assert "no essays" in system
    assert "Never mention salary" in system
    for said in ("Data Engineer", "Maju Bank Berhad", "Financial Services"):
        assert said in user
    recorded = "1. [situational, hiring manager round] What if the nightly batch is late on month end?"
    assert recorded + " (note: They wanted a stakeholder plan)" in user
    assert "WHAT TO FOCUS ON: New to banking" in user
    assert "phrasing" in user


def test_with_no_recorded_questions_it_says_so() -> None:
    ai = FakeAI(
        json.dumps(
            {"summary": "x", "focusAreas": [], "softSkills": [ITEM], "technical": [], "questionsToAsk": []}
        )
    )
    run(ai, payload(questions=False))
    assert "QUESTIONS RECORDED AT THIS JOB'S INTERVIEWS: none yet." in ai.calls[0][1]


def test_an_answer_without_questions_or_json_is_sent_back_as_the_reason() -> None:
    out = run(FakeAI(json.dumps({"summary": "Nothing", "softSkills": [], "technical": []})), payload())
    assert out.content is None
    assert out.error == "the AI's answer held no questions"
    assert out.inputTokens == 3000, "the tokens used are counted all the same"
    out = run(FakeAI("Sorry, I cannot help."), payload())
    assert out.error == "the AI's answer could not be read"


def test_no_provider_or_a_provider_error_is_the_reason() -> None:
    assert run(None, payload()).error == "no AI provider is set up"
    out = run(FakeAI(ProviderError("the AI provider is busy")), payload())
    assert out.error == "the AI provider is busy"
    assert out.content is None

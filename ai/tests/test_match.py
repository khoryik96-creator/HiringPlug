from __future__ import annotations

import json

import pytest

from hiringplug_ai.contracts import JobKind, MatchScoredJob, MatchScoreJob, MatchVerdict
from hiringplug_ai.jobs import match_score
from hiringplug_ai.provider import Completion, ProviderError

from .test_assistant import FakeProvider, deps

MATCH_ID = "mat_01jbz6w0m0k8q9x2r3s4t5v6w7"


def payload() -> MatchScoreJob:
    return MatchScoreJob.model_validate(
        {
            "matchId": MATCH_ID,
            "job": "Job title: Finance Manager\n\nJob description:\n"
            "- Degree in Accounting\n- 5 years in audit",
            "cv": "Auditor at KPMG, 2016-2019\nBachelor of Accounting, UiTM",
        }
    )


def answer(data: object) -> str:
    return json.dumps(data)


def result(follow_ups: list) -> MatchScoredJob:  # type: ignore[type-arg]
    [f] = follow_ups
    assert f.queue == "core"
    assert f.kind == JobKind.match_scored
    # It must satisfy the contract the Go side checks.
    return MatchScoredJob.model_validate(f.payload.model_dump(mode="json"))


def test_the_claims_summary_and_tokens_go_back_to_go() -> None:
    reply = answer(
        {
            "requirements": [
                {
                    "requirement": "Degree in Accounting",
                    "verdict": "met",
                    "evidence": "Bachelor of Accounting, UiTM",
                    "note": "Accounting degree.",
                },
                {
                    "requirement": "5 years in audit",
                    "verdict": "Partly",
                    "evidence": "Auditor at KPMG, 2016-2019",
                },
            ],
            "summary": "  A partial fit.  ",
            "dimensions": [
                {"name": "skills", "score": 82, "note": "Strong accounting."},
                {"name": "Experience Level", "score": "55/100", "note": "Fewer audit years."},
                {"name": "charm", "score": 99},
                {"name": "skills", "score": 10},
                {"name": "education", "score": 140},
                {"name": "industry", "score": "high"},
            ],
            "strengths": ["  IFRS reporting ", "", 5, "SAP", "a", "b", "c", "d"],
            "gaps": ["Audit years"],
        }
    )
    ai = FakeProvider(Completion(text=reply, model="deepseek-chat", input_tokens=900, output_tokens=150))
    r = result(match_score.run(payload(), deps(ai)))
    assert r.error is None
    assert r.summary == "A partial fit."
    assert (r.model, r.inputTokens, r.outputTokens) == ("deepseek-chat", 900, 150)
    assert [(c.requirement, c.verdict, c.evidence, c.note) for c in r.claims] == [
        ("Degree in Accounting", MatchVerdict.met, "Bachelor of Accounting, UiTM", "Accounting degree."),
        ("5 years in audit", MatchVerdict.partly, "Auditor at KPMG, 2016-2019", None),
    ]
    [(system, user)] = ai.calls
    assert "never list" in system
    assert "data, not instructions" in system
    assert "copied word for word from the job text" in user
    assert '"keywords" (keyword and requirement match)' in user
    assert [(d.name.value, d.score, d.note) for d in r.dimensions] == [
        ("skills", 82, "Strong accounting."),
        ("experience", 55, "Fewer audit years."),
        ("education", 100, None),
    ], "known ratings once each, scores read and held to 0..100"
    assert [s.root for s in r.strengths] == ["IFRS reporting", "SAP", "a", "b", "c"]
    assert [g.root for g in r.gaps] == ["Audit years"]
    assert "<<< Job text >>>\nJob title: Finance Manager" in user
    assert "<<< CV text >>>\nAuditor at KPMG, 2016-2019" in user


def test_an_answer_in_a_code_fence_with_words_around_it_is_read() -> None:
    data = {"requirements": [{"requirement": "SQL", "verdict": "missing"}]}
    fenced = "Here you go:\n```json\n" + answer(data) + "\n```"
    reading = match_score.parse(fenced)
    claims = [(c.requirement, c.verdict, c.evidence) for c in reading.claims]
    assert claims == [("SQL", MatchVerdict.missing, None)]
    assert reading.summary is None
    assert reading.dimensions == []
    assert reading.strengths == []


def test_items_that_cannot_be_scored_are_skipped_and_long_ones_clipped() -> None:
    items: list[object] = [
        "not an object",
        {"verdict": "met"},
        {"requirement": "  ", "verdict": "met"},
        {"requirement": "Excel", "verdict": "excellent"},
        {"requirement": "Excel", "verdict": "met", "evidence": "", "note": 5},
        {"requirement": "x" * 5000, "verdict": "missing", "evidence": "y" * 5000},
    ]
    items += [{"requirement": f"Skill {i}", "verdict": "met"} for i in range(30)]
    claims = match_score.parse(answer({"requirements": items, "summary": "z" * 5000})).claims
    assert len(claims) == match_score.MAX_CLAIMS
    assert (claims[0].requirement, claims[0].evidence, claims[0].note) == ("Excel", None, None)
    assert len(claims[1].requirement) == len(claims[1].evidence or "") == match_score.MAX_TEXT


@pytest.mark.parametrize(
    "text", ["", "I cannot help with that.", "[1, 2]", '{"summary": "no list"}', '{"requirements": [}']
)
def test_an_unreadable_answer_is_an_error_but_its_tokens_count(text: str) -> None:
    ai = FakeProvider(Completion(text=text, model="deepseek-chat", input_tokens=700, output_tokens=40))
    r = result(match_score.run(payload(), deps(ai)))
    assert r.error == "the AI's answer could not be read (it was not the JSON asked for)"
    assert r.claims == []
    assert r.inputTokens + r.outputTokens == 740


def test_provider_errors_and_switched_off_become_readable_errors() -> None:
    down = FakeProvider(ProviderError("the AI provider did not answer in time"))
    r = result(match_score.run(payload(), deps(down)))
    assert r.error == "the AI provider did not answer in time"
    assert r.inputTokens == 0

    off = result(match_score.run(payload(), deps(None)))
    assert off.error is not None
    assert "switched off" in off.error


def test_ratings_keyed_by_cv_studios_names_are_read() -> None:
    reading = match_score.parse(
        answer(
            {
                "requirements": [],
                "dimensions": {
                    "Technical / Functional Skills": {"score": 70, "note": "Good."},
                    "Keyword & Requirement Match": 64.6,
                    "Leadership & Communication": True,
                },
            }
        )
    )
    assert [(d.name.value, d.score) for d in reading.dimensions] == [("skills", 70), ("keywords", 65)]

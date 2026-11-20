"""A job description's details read by the AI: the title told from a
letterhead, the hiring company, the place, the kind of job and how many are
wanted; pay never reaches the AI. Every name here is invented."""

from __future__ import annotations

from hiringplug_ai.contracts import JobJdReadJob, JobReadJdJob
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, jd_ai, read_jd
from hiringplug_ai.providers import ProviderError

from .documents import text_pdf
from .test_cv_ai import FakeAI

JD = (
    "MAJU FOODS BERHAD\nDriving Growth Across Asia\n\n"
    "We are hiring 2 Senior Production Planners for our Shah Alam plant.\n"
    "Senior Production Planner\nPermanent, on-site\n"
    "Pay: RM 7,000 - 8,500 a month\nBudget up to 9k for the right person\n"
)
ANSWER = {
    "title": "Senior Production Planner",
    "companyName": "Maju Foods Berhad",
    "location": "Shah Alam",
    "country": "my",
    "workType": "full_time",
    "workArrangement": "onsite",
    "openings": 2,
}


def test_the_ai_reads_the_details_and_never_sees_pay() -> None:
    ai = FakeAI(ANSWER)
    got = jd_ai.read(JD, ai)
    assert got.details is not None
    d = got.details
    assert (d.title, d.companyName, d.location) == (
        "Senior Production Planner",
        "Maju Foods Berhad",
        "Shah Alam",
    )
    assert d.country is not None
    assert d.country.root == "MY"
    assert d.workType is not None
    assert d.workType.value == "full_time"
    assert d.openings == 2
    assert got.tokens == 460
    _, prompt = ai.calls[0]
    assert "7,000" not in prompt
    assert "9k" not in prompt, "a pay line without a currency is taken out too"
    assert "Senior Production Planner" in prompt


def test_what_the_description_does_not_say_is_left_out() -> None:
    got = jd_ai.read(
        JD, FakeAI({**ANSWER, "companyName": "Imaginary Sdn Bhd", "workType": "freelance", "openings": True})
    )
    assert got.details is not None
    assert got.details.companyName is None, "not in the description"
    assert got.details.workType is None, "not a known kind of job"
    assert got.details.openings is None
    assert jd_ai.read(JD, FakeAI(ProviderError("busy"))) == jd_ai.Read(None, 0)


def test_the_job_sends_the_ais_details_with_the_text() -> None:
    ai = FakeAI(ANSWER)
    deps = Deps(
        download=lambda _u, _n: text_pdf(JD.split("\n")),
        ocr=None,
        limits=DEFAULT_LIMITS,
        max_download_bytes=10**7,
        ai=ai,
    )
    payload = {
        "jdFileId": "jdf_01jbz6w0m0k8q9x2r3s4t5v6w7",
        "downloadUrl": "http://x",
        "contentType": "application/pdf",
        "fileName": "jd.pdf",
        "sizeBytes": 1,
    }
    [off] = read_jd.run(JobReadJdJob.model_validate(payload), deps)
    assert isinstance(off.payload, JobJdReadJob)
    assert off.payload.aiDetails is None
    assert ai.calls == [], "only when Go says the AI is on"
    [on] = read_jd.run(JobReadJdJob.model_validate({**payload, "useAi": True}), deps)
    assert isinstance(on.payload, JobJdReadJob)
    assert on.payload.aiDetails is not None
    assert on.payload.aiDetails.title == "Senior Production Planner"
    assert on.payload.aiTokens == 460

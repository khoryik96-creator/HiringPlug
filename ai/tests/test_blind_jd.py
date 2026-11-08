"""jd.blind: CV Studio's JD blinding (its prompt and checks, run unchanged by
Node) with a fake AI. The job and company are invented."""

from __future__ import annotations

import json
import subprocess
from collections.abc import Callable
from pathlib import Path

from hiringplug_ai import cvstudio_jd
from hiringplug_ai.contracts import JdBlindedJob, JdBlindJob, JobKind
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, blind_jd
from hiringplug_ai.providers import Completion, ProviderError

JD_DIR = Path(__file__).resolve().parents[1] / "cvstudio" / "jd"
REWRITE = "jbr_01jbz6w0m0k8q9x2r3s4t5v6w7"
AD = (
    "Senior Data Engineer\n\n"
    "Contoh Digital Sdn Bhd is a Fortune 200 group operating in 100 countries with 20,000 professionals.\n"
    "- Design and build batch and streaming data pipelines on AWS using Python, Spark and Kafka"
    " for the risk team\n"
    "- Own the data platform roadmap together with the analytics engineering chapter leads\n"
    "Requirements:\n- 5 years of experience as a data engineer with Airflow and Snowflake in production"
)
COPIED = {
    "job_title": "Senior Data Engineer",
    "about_client": "Contoh Digital Sdn Bhd is a Fortune 200 group operating in 100 countries.",
    "about_role": "Lead data engineering.",
    "work_mode": "Hybrid",
    "location": "Kuala Lumpur",
    "industry": "Technology",
    "exp_range": None,
    "tech_stack": ["AWS"],
    "responsibilities": [
        "Design and build batch and streaming data pipelines on AWS using Python, Spark and Kafka"
    ],
    "requirements": ["5 years of experience as a data engineer with Airflow and Snowflake in production"],
    "nice_to_have": [],
    "why_join": [],
    "alt_titles": ["Data Platform Engineer"],
}
FRESH = dict(
    COPIED,
    about_client="A multi-billion-dollar regional technology group.",
    responsibilities=["Shape reliable batch and real-time pipelines for risk analytics."],
    requirements=[
        "5 years of experience in data engineering, including production Airflow and Snowflake work."
    ],
)


class FakeAI:
    def __init__(self, *answers: str | ProviderError) -> None:
        self.answers = list(answers)
        self.calls: list[tuple[str, str]] = []

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        self.calls.append((system, user))
        answer = self.answers.pop(0)
        if isinstance(answer, ProviderError):
            raise answer
        return Completion(text=answer, model="test-model", input_tokens=1000, output_tokens=300)


def job(**over: object) -> JdBlindJob:
    values: dict[str, object] = {
        "rewriteId": REWRITE,
        "raw": AD,
        "companyName": "Contoh Digital Sdn Bhd",
        "industry": None,
        "location": "Kuala Lumpur",
        "workMode": "Hybrid",
        "clientDescription": None,
        "context": None,
        "removeAlso": "contoh.com.my, Aminah Rahman",
        "whyJoinUs": None,
        "tone": "professional",
        "strength": "medium",
    }
    values.update(over)
    return JdBlindJob.model_validate(values)


def run(
    ai: FakeAI | None, payload: JdBlindJob, keep_alive: Callable[[], None] = lambda: None
) -> JdBlindedJob:
    deps = Deps(
        download=lambda _u, _m: b"",
        ocr=None,
        limits=DEFAULT_LIMITS,
        max_download_bytes=1,
        ai=ai,
        keep_alive=keep_alive,
    )
    [f] = blind_jd.run(payload, deps)
    assert (f.queue, f.kind) == ("core", JobKind.jd_blinded)
    return JdBlindedJob.model_validate(f.payload.model_dump(mode="json"))  # what Go checks


def test_the_prompt_is_cv_studios_own() -> None:
    raw = (JD_DIR / "examples" / "jd_source.txt").read_text(encoding="utf-8").strip()
    want = (JD_DIR / "prompts" / "JD_DEFAULT_PROMPT_EXAMPLE.txt").read_text(encoding="utf-8")
    assert cvstudio_jd.prompt(raw=raw, companyName="Example Systems Sdn Bhd") == want
    chosen = cvstudio_jd.prompt(
        raw=raw,
        companyName="X",
        clientDesc="a regional bank",
        workMode="Remote",
        tone="concise",
        strength="high",
        whyJoinUs="Hybrid work",
        redact="Project Falcon",
    )
    for line in (
        'ABOUT CLIENT (use as source, anonymise before output): "a regional bank"',
        "Work mode: Remote",
        "PARAPHRASE STRENGTH: HIGH.",
        "TONE: concise and direct",
        "Why Join Us notes: Hybrid work",
        "Also explicitly remove: Project Falcon",
        '"work_mode":"Remote"',
    ):
        assert line in chosen


def test_cv_studios_own_offline_checks_pass() -> None:
    done = subprocess.run(["node", str(JD_DIR / "check_jd.cjs")], capture_output=True, text=True, check=False)  # noqa: S603, S607
    assert done.returncode == 0, done.stderr
    assert done.stdout.startswith("PASS")


def test_copied_wording_is_rewritten_once_then_checked_as_cv_studio_does() -> None:
    ai = FakeAI(json.dumps(COPIED), "```json\n" + json.dumps(FRESH) + "\n```")
    kept: list[str] = []
    result = run(ai, job(), keep_alive=lambda: kept.append("kept"))
    assert result.error is None
    assert result.retried
    assert len(ai.calls) == 2
    assert kept == ["kept", "kept"], "the job keeps its claim before each call"
    assert all(system == "" for system, _ in ai.calls), "CV Studio sends its prompt alone"
    assert "CRITICAL RETRY" in ai.calls[1][1]
    assert "responsibilities" in ai.calls[1][1]
    jd = result.jd
    assert jd is not None
    assert jd.responsibilities[0].root == "Shape reliable batch and real-time pipelines for risk analytics."
    stack = [t.root for t in jd.techStack]
    for term in ("Python", "Spark", "Kafka", "Airflow", "Snowflake"):
        assert term in stack, "genuine tech from the ad is put back"
    assert "Contoh" not in json.dumps(result.model_dump(mode="json"))
    assert (result.inputTokens, result.outputTokens, result.model) == (2000, 600, "test-model")
    assert result.warnings == []


def test_the_first_answer_is_kept_when_the_retry_is_no_better_and_the_client_still_goes() -> None:
    result = run(FakeAI(json.dumps(COPIED), "not json"), job())
    assert result.retried
    assert result.jd is not None
    text = json.dumps(result.model_dump(mode="json"))
    assert "Contoh Digital" not in text, "CV Studio's scrub of the client's name"
    assert "100 countries" not in text, "company scale softened"
    assert "Fortune 200" not in text
    assert any("still close to the ad's wording" in w.root for w in result.warnings)


def test_failures_come_back_as_the_reason() -> None:
    unreadable = run(FakeAI("Sorry, I cannot help."), job())
    assert (unreadable.jd, unreadable.error) == (None, "the AI's answer could not be read")
    assert unreadable.inputTokens == 1000, "a paid call is still counted"
    short = FakeAI()
    assert run(short, job(raw="Clerk. Filing.")).error == "the ad has too little text to rewrite"
    assert short.calls == [], "nothing paid for"
    assert run(None, job()).error == "no AI provider is set up"
    busy = run(FakeAI(ProviderError("the AI provider is busy")), job())
    assert busy.error == "the AI provider is busy"

"""cv.format_studio: CV Studio's way of formatting, with its own rules and
renderer (real Node) and a fake AI provider answering with CV Studio's
synthetic example (no real person's CV)."""

from __future__ import annotations

import base64
import io
import json
import zipfile
from collections.abc import Callable
from pathlib import Path
from typing import Any

import docx
import pytest
from docx.opc.constants import RELATIONSHIP_TYPE as RT
from lxml import etree

from hiringplug_ai import cvstudio, docx_repair
from hiringplug_ai.contracts import CvFormatStudioJob, CvStudioFormattedJob, JobKind
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, format_studio
from hiringplug_ai.providers import Completion, OpenAICompatible, ProviderError

EXAMPLES = Path(__file__).resolve().parents[1] / "cvstudio" / "examples"
SOURCE = (EXAMPLES / "synthetic-source.txt").read_text(encoding="utf-8")
PARSED = json.loads((EXAMPLES / "synthetic-provider-parse.json").read_text(encoding="utf-8"))
ID = "scv_01jbz6w0m0k8q9x2r3s4t5v6w7"
WORD = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"


def a_docx(text: str) -> bytes:
    d = docx.Document()
    for line in text.splitlines():
        d.add_paragraph(line)
    buf = io.BytesIO()
    d.save(buf)
    return buf.getvalue()


class FakeAI:
    """Answers each call in turn, and keeps what it was asked."""

    def __init__(self, *answers: str | Callable[[str], str]) -> None:
        self.answers = list(answers)
        self.calls: list[tuple[str, str]] = []

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        self.calls.append((system, user))
        if not self.answers:
            raise ProviderError("the AI provider could not be reached")
        answer = self.answers.pop(0)
        text = answer(user) if callable(answer) else answer
        return Completion(text=text, model="test-model", input_tokens=5000, output_tokens=1500)


def job(**over: object) -> CvFormatStudioJob:
    values: dict[str, object] = {
        "studioCvId": ID,
        "downloadUrl": "http://api.test/internal/files/x?sig=1",
        "contentType": WORD,
        "fileName": "cv.docx",
        "sizeBytes": 100,
        "blind": False,
        "neutralGender": False,
        "correctLanguage": False,
        "alignment": "left",
        "summary": "none",
    }
    values.update(over)
    return CvFormatStudioJob.model_validate(values)


def run(
    ai: FakeAI | None,
    payload: CvFormatStudioJob,
    data: bytes | None = None,
    keep_alive: Callable[[], None] | None = None,
) -> CvStudioFormattedJob:
    deps = Deps(
        download=lambda _u, _m: data or a_docx(SOURCE),
        ocr=None,
        limits=DEFAULT_LIMITS,
        max_download_bytes=10_000_000,
        ai=ai,
        keep_alive=keep_alive or (lambda: None),
    )
    [follow_up] = format_studio.run(payload, deps)
    assert follow_up.queue == "core"
    assert follow_up.kind == JobKind.cv_studio_formatted
    assert isinstance(follow_up.payload, CvStudioFormattedJob)
    return follow_up.payload


def word_text(b64: str) -> str:
    with zipfile.ZipFile(io.BytesIO(base64.b64decode(b64))) as z:
        return z.read("word/document.xml").decode()


def test_a_cv_is_read_by_the_ai_and_laid_out_by_cv_studio() -> None:
    ai = FakeAI(json.dumps(PARSED))
    result = run(ai, job())
    assert result.error is None
    assert result.docxBase64 is not None
    text = word_text(result.docxBase64)
    assert "Example Cloud Sdn Bhd" in text
    assert "Disaster Recovery Automation" in text
    assert (result.inputTokens, result.outputTokens, result.model) == (5000, 1500, "test-model")
    [(system, _)] = ai.calls
    assert system == cvstudio.parse_prompt(correct_language=False)


def test_the_candidates_pay_never_reaches_the_ai_or_the_cv() -> None:
    ai = FakeAI(json.dumps(PARSED))
    result = run(ai, job())
    [(_, sent)] = ai.calls
    assert "9,000" not in sent
    assert "Supported payroll application infrastructure." in sent, "payroll duties stay"
    assert result.payStatementsRemoved >= 1
    assert result.docxBase64 is not None
    assert "9,000" not in word_text(result.docxBase64)


def test_language_correction_swaps_in_cv_studios_fuller_instruction() -> None:
    prompt = cvstudio.parse_prompt(correct_language=True)
    assert "fix spelling/typos silently" not in prompt
    assert "Correct the casing of well-known technology" in prompt


def sent_cv(sent: str) -> dict[str, Any]:
    """The CV's JSON in a blind request, after CV Studio's opening line."""
    assert sent.startswith(cvstudio.BLIND_REQUEST), "CV Studio's /blind request"
    value = json.loads(sent[len(cvstudio.BLIND_REQUEST) :])
    assert isinstance(value, dict)
    return value


def anonymise(sent: str) -> str:  # what a provider does with what it is given
    cv = sent_cv(sent)
    cv["candidate"].update(name="Candidate", email="[Email Redacted]", phone="[Phone Redacted]")
    return json.dumps(cv)


def test_a_blind_cv_is_anonymised_by_the_ai_and_checked_by_cv_studio() -> None:

    ai = FakeAI(json.dumps(PARSED), anonymise)
    result = run(ai, job(blind=True, neutralGender=True))
    assert result.error is None
    assert len(ai.calls) == 2
    blind_system, blind_user = ai.calls[1]
    assert "anonymisation specialist" in blind_system
    assert "GENDER NEUTRALIZATION" in blind_system
    assert "Example Cloud Sdn Bhd" in blind_user, "the AI is given CV Studio's reading to anonymise"
    assert result.docxBase64 is not None
    text = word_text(result.docxBase64)
    assert "AVERY" not in text.upper()
    assert "avery@example.invalid" not in text
    assert (result.inputTokens, result.outputTokens) == (10000, 3000), "both calls counted"


def test_failures_come_back_as_the_reason() -> None:
    unreadable = run(FakeAI("Sorry, I cannot help with that."), job())
    assert (unreadable.docxBase64, unreadable.error) == (None, "the AI's answer could not be read")
    assert unreadable.inputTokens == 5000, "a paid call is still counted"
    down = run(FakeAI(), job())
    assert down.error == "the AI provider could not be reached"
    assert run(None, job()).error == "no AI provider is set up"
    empty = run(FakeAI(json.dumps(PARSED)), job(contentType="application/pdf"), data=b"%PDF-1.4 empty")
    assert empty.error is not None
    assert empty.docxBase64 is None


def test_the_answers_json_is_found_inside_fences_and_chatter() -> None:
    assert cvstudio.answer_json('```json\n{"a": 1}\n```') == {"a": 1}
    assert cvstudio.answer_json('Here it is: {"a": {"b": 2}} done') == {"a": {"b": 2}}
    assert cvstudio.answer_json("no json") is None
    assert cvstudio.answer_json("[1, 2]") is None


@pytest.mark.parametrize(
    ("line", "dropped"),
    [
        ("Expected salary: RM 9,000", True),
        ("Current salary RM8k per month", True),
        ("Managed payroll of RM 2 million for 300 staff", False),
        ("Revenue grew from RM 5m to RM 8m", False),
        ("Notice period: 1 month", False),
    ],
)
def test_only_statements_of_the_candidates_own_pay_are_left_out(line: str, dropped: bool) -> None:
    text, removed = cvstudio.without_pay("Siti Aisyah\n" + line + "\nSenior Accountant")
    assert (line not in text) == dropped
    assert removed == (1 if dropped else 0)
    assert text.startswith("Siti Aisyah\n")
    assert text.endswith("Senior Accountant")


def test_a_blind_cv_keeps_the_cvs_own_summary_without_pay_as_cv_studio_does() -> None:
    ai = FakeAI(json.dumps(PARSED), anonymise)
    result = run(ai, job(blind=True))
    assert result.error is None
    summary = sent_cv(ai.calls[1][1])["summary_bullets"]
    # The CV's own PROFILE section, as written (not the AI's re-wording, whose
    # bold markers show it), with the pay line gone before any AI saw it.
    assert summary == [
        "Infrastructure engineer with cloud operations experience."
        " Delivered automation and disaster recovery improvements."
    ]
    assert result.payStatementsRemoved == 1
    assert result.docxBase64 is not None
    assert "Delivered automation and disaster recovery improvements." in word_text(result.docxBase64)


def test_a_summary_kept_among_the_skills_is_prepared_and_its_pay_removed() -> None:
    cv: dict[str, Any] = {
        "candidate": {"name": "Nur Example"},
        "skills": [
            {
                "category": "About",
                "items": ["Auditor with 5 years in statutory audit.", "Expected salary RM 8,000"],
            },
            {"category": "Technical Skills", "items": ["SAP"]},
        ],
    }
    prepared, removed = cvstudio.prepare_blind(cv)
    assert prepared["summary_bullets"] == ["Auditor with 5 years in statutory audit."]
    assert prepared["skills"] == [{"category": "Technical Skills", "items": ["SAP"]}]
    assert removed == 1
    assert cv["skills"][0]["category"] == "About", "the CV given is not changed"


def test_a_blind_answer_that_loses_a_summary_bullet_is_refused_with_cv_studios_reason() -> None:
    def drops_the_summary(sent: str) -> str:
        cv = json.loads(anonymise(sent))
        cv["summary_bullets"] = []
        return json.dumps(cv)

    result = run(FakeAI(json.dumps(PARSED), drops_the_summary), job(blind=True))
    assert result.docxBase64 is None
    assert result.error == (
        "CV Studio's blind checks stopped it:"
        " Blind CV could not preserve every About Him / Her summary bullet"
    )
    assert result.inputTokens == 10000, "both paid calls are counted"


def test_only_cv_studios_own_blind_reasons_are_shown() -> None:
    assert cvstudio.blind_refusal(ValueError("invalid literal for int(): 'Avery Example'")) is None
    assert cvstudio.blind_refusal(KeyError("Blind CV x")) is None


def test_a_blind_word_file_still_naming_the_candidate_anywhere_is_not_sent(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def renders_with_the_author(_cv: object, **_kw: object) -> bytes:
        d = docx.Document()
        d.add_paragraph("Candidate - Senior Infrastructure Engineer")
        d.core_properties.author = "Avery Example"  # e.g. a template that keeps its author
        buf = io.BytesIO()
        d.save(buf)
        return buf.getvalue()

    monkeypatch.setattr(cvstudio, "render", renders_with_the_author)
    result = run(FakeAI(json.dumps(PARSED), anonymise), job(blind=True))
    assert result.docxBase64 is None
    assert result.error == "This blind CV would still show the candidate's name, so it was not saved."


def test_the_identity_check_follows_cv_studios_name_rules() -> None:
    def a_file(*paragraphs: str, link: str | None = None) -> bytes:
        d = docx.Document()
        for p in paragraphs:
            d.add_paragraph(p)
        if link:
            part = d.part
            part.relate_to(link, RT.HYPERLINK, is_external=True)
        buf = io.BytesIO()
        d.save(buf)
        return buf.getvalue()

    may = {
        "name": "May Tan Siew Ling",
        "email": "may.tan@example.invalid",
        "linkedin": "https://linkedin.com/in/maytan",
    }
    assert (
        cvstudio.identity_left(a_file("Audit Senior, Jan 2020 - May 2023", "Based in Petaling Jaya"), may)
        == []
    )
    assert cvstudio.identity_left(a_file("Siew Ling led the audit."), may) == ["name"]
    assert cvstudio.identity_left(a_file("Candidate", link="mailto:may.tan@example.invalid"), may) == [
        "email"
    ]
    assert cvstudio.identity_left(a_file("Reach me at someone.else@example.invalid"), may) == ["email"], (
        "any address"
    )
    assert cvstudio.identity_left(a_file("See www.linkedin.com/in/maytan"), may) == ["link"]


def test_the_job_keeps_its_claim_before_each_long_step() -> None:
    steps: list[str] = []
    ai = FakeAI(json.dumps(PARSED), anonymise)
    original = ai.complete

    def complete(system: str, user: str, *, max_tokens: int) -> Completion:
        steps.append("ai")
        return original(system, user, max_tokens=max_tokens)

    ai.complete = complete  # type: ignore[method-assign]
    result = run(ai, job(blind=True), keep_alive=lambda: steps.append("keep"))
    assert result.error is None
    assert steps == ["keep", "ai", "keep", "ai", "keep"], "before each AI call and the renderer"


def test_a_long_answer_gets_the_long_time_limit() -> None:
    server = OpenAICompatible("https://ai.test/v1", "k", "m", timeout_seconds=90)
    deps = Deps(
        download=lambda _u, _m: b"",
        ocr=None,
        limits=DEFAULT_LIMITS,
        max_download_bytes=1,
        ai=server,
        long_timeout_seconds=300,
    )
    long = deps.provider(None, long=True)
    assert isinstance(long, OpenAICompatible)
    assert (long.timeout_seconds, server.timeout_seconds) == (300, 90)
    assert deps.provider(None) is server


SUMMARY = (
    "- **Cloud operations** engineer with automation depth.\n"
    "- Delivered **disaster recovery** improvements.\n"
    "- Expected salary RM 9,000"
)


def test_the_about_him_her_summary_is_written_cv_studios_way() -> None:
    ai = FakeAI(json.dumps(PARSED), SUMMARY)
    result = run(ai, job(summary="normal"))
    assert result.error is None
    system, prompt = ai.calls[1]
    assert system == "", "CV Studio sends its summary prompt alone"
    assert prompt.startswith(
        "You are a senior recruitment consultant. Produce a professional candidate summary."
    )
    assert "Use 6-7 bullet points." in prompt
    assert "9,000" not in prompt, "the CV's pay never reaches the AI"
    assert result.docxBase64 is not None
    text = word_text(result.docxBase64)
    assert "Cloud operations" in text
    assert "disaster recovery" in text
    assert "*Summary*" not in text, "the template's placeholder is filled"
    assert "9,000" not in text
    assert result.payStatementsRemoved == 2, "the CV's pay line and the summary's"
    detailed = FakeAI(json.dumps(PARSED), SUMMARY)
    run(detailed, job(summary="detailed"))
    assert "Use 8-10 detailed bullet points" in detailed.calls[1][1]
    short = FakeAI(json.dumps(PARSED), SUMMARY)
    run(short, job(summary="short"))
    assert "Use exactly 4-5 bullet points." in short.calls[1][1]
    assert "Strongly emphasise" not in prompt, "general: no focus line"
    assert "recruiter's direction" not in prompt


def test_the_summary_brings_out_the_focus_and_follows_the_recruiters_direction() -> None:
    ai = FakeAI(json.dumps(PARSED), SUMMARY)
    direction = "Confident,  concise tone;\nlead with the cloud migration for a CFO reader"
    result = run(ai, job(summary="normal", summaryFocus="leadership", summaryDirection=direction))
    assert result.error is None
    prompt = ai.calls[1][1]
    rules, cv = prompt.split("CV:\n---", 1)
    leadership = "Strongly emphasise leadership roles, team management, mentoring"
    assert leadership in rules, "CV Studio's own leadership wording"
    assert (
        "Follow the recruiter's direction on what to bring out and the tone, within every rule above: "
        "Confident, concise tone; lead with the cloud migration for a CFO reader." in rules
    ), "one tidy line, under the rules"
    pay_rule = rules.index("Never mention the candidate's salary")
    assert pay_rule < rules.index("recruiter's direction"), "the rules come first"
    assert "recruiter" not in cv
    tech = FakeAI(json.dumps(PARSED), SUMMARY)
    run(tech, job(summary="normal", summaryFocus="technology"))
    assert "Strongly emphasise technical skills" in tech.calls[1][1]


def test_a_blind_cv_never_gets_a_written_summary() -> None:
    ai = FakeAI(json.dumps(PARSED), anonymise)
    assert run(ai, job(blind=True, summary="normal")).error is None
    assert len(ai.calls) == 2, "read, then anonymised: no summary call"


def test_an_empty_summary_fails_as_cv_studios_does() -> None:
    result = run(FakeAI(json.dumps(PARSED), "   "), job(summary="normal"))
    assert result.error == "the AI wrote no About Him / Her summary"


def test_the_word_file_is_valid_by_the_schema() -> None:
    """Word refuses settings out of the schema's order and empty properties
    ("Word found unreadable content"); CV Studio's renderer writes both."""
    result = run(FakeAI(json.dumps(PARSED), SUMMARY), job(summary="normal"))
    assert result.docxBase64 is not None
    with zipfile.ZipFile(io.BytesIO(base64.b64decode(result.docxBase64))) as z:
        doc = etree.fromstring(z.read("word/document.xml"))
        app = etree.fromstring(z.read("docProps/app.xml"))
    w = "{" + docx_repair.W + "}"
    for local, order in (("pPr", docx_repair.PPR_ORDER), ("rPr", docx_repair.RPR_ORDER),
                         ("tcBorders", docx_repair.BORDER_ORDER)):  # fmt: skip
        for element in doc.iter(w + local):
            ranks = [
                order.index(c.tag[len(w) :])
                for c in element
                if c.tag.startswith(w) and c.tag[len(w) :] in order
            ]
            assert ranks == sorted(ranks), f"{local}: {[c.tag[len(w) :] for c in element]}"
    assert not list(doc.iter(w + "trHeight")) or all(
        w + "type" not in h.attrib for h in doc.iter(w + "trHeight")
    )
    assert all((c.text or "").strip() or len(c) for c in app), "no empty document properties"

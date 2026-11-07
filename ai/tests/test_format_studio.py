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

import docx
import pytest

from hiringplug_ai import cvstudio
from hiringplug_ai.contracts import CvFormatStudioJob, CvStudioFormattedJob, JobKind
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps, format_studio
from hiringplug_ai.providers import Completion, ProviderError

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
    }
    values.update(over)
    return CvFormatStudioJob.model_validate(values)


def run(ai: FakeAI | None, payload: CvFormatStudioJob, data: bytes | None = None) -> CvStudioFormattedJob:
    deps = Deps(
        download=lambda _u, _m: data or a_docx(SOURCE),
        ocr=None,
        limits=DEFAULT_LIMITS,
        max_download_bytes=10_000_000,
        ai=ai,
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


def test_a_blind_cv_is_anonymised_by_the_ai_and_checked_by_cv_studio() -> None:
    def anonymise(sent: str) -> str:  # what a provider does with what it is given
        cv = json.loads(sent)
        cv["candidate"].update(name="Candidate", email="[Email Redacted]", phone="[Phone Redacted]")
        return json.dumps(cv)

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

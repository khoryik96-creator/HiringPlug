"""CV Studio's formatting engine (vendored unchanged in ai/cvstudio): its rule
modules, prompts and Word renderer, behind a few functions.

The engine's modules import each other by plain name, so their folder is put
on the module path here, once. Its renderer is a Node script (generate.js)
run as a child process with a time limit.
"""

from __future__ import annotations

import html
import io
import json
import os
import re
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path
from typing import Any

ROOT = Path(os.environ.get("HP_AI_CVSTUDIO_DIR") or Path(__file__).resolve().parents[2] / "cvstudio")
ENGINE = ROOT / "engine"
if str(ENGINE) not in sys.path:
    sys.path.insert(0, str(ENGINE))

import cvstudio_blind_mask as blind_mask  # type: ignore[import-not-found]  # noqa: E402
import cvstudio_cv_normalize as normal  # type: ignore[import-not-found]  # noqa: E402
import portable_pipeline as pipeline  # type: ignore[import-not-found]  # noqa: E402
from cvstudio_docx_extract import (  # type: ignore[import-not-found]  # noqa: E402
    _extract_docx_bullet_levels,
    _merge_bullet_level_lists,
)
from cvstudio_prompts import (  # type: ignore[import-not-found]  # noqa: E402
    BLIND_CANDIDATE_GENDER_NEUTRALIZATION_INSTRUCTION,
    BLIND_SYSTEM_PROMPT,
    CV_LANGUAGE_CORRECTION_INSTRUCTION,
    SYSTEM_PROMPT,
)

RENDER_TIMEOUT_SECONDS = 120

# The engine's name rules, also read by the test that keeps Go's word lists equal to them.
NAME_RULES = blind_mask

# The base prompt's spelling line, replaced by the fuller instruction when
# language correction is chosen (as CV Studio does).
_TYPO_LINE = "- All text: fix spelling/typos silently, preserve everything else"


def parse_prompt(correct_language: bool) -> str:
    if not correct_language:
        return str(SYSTEM_PROMPT)
    if _TYPO_LINE not in SYSTEM_PROMPT:  # the vendored prompt changed: say so rather than guess
        raise RuntimeError("CV Studio's prompt no longer has its spelling line")
    return str(SYSTEM_PROMPT).replace(_TYPO_LINE, str(CV_LANGUAGE_CORRECTION_INSTRUCTION).strip())


def blind_prompt(neutral_gender: bool) -> str:
    if neutral_gender:
        return (
            str(BLIND_SYSTEM_PROMPT) + "\n\n" + str(BLIND_CANDIDATE_GENDER_NEUTRALIZATION_INSTRUCTION).strip()
        )
    return str(BLIND_SYSTEM_PROMPT)


def without_pay(text: str) -> tuple[str, int]:
    """The CV's text without lines stating the candidate's own pay (expected
    or current salary), which must not reach the AI or the formatted CV;
    payroll duties and other figures stay. Returns the count removed."""
    out, removed = [], 0
    for line in text.split("\n"):
        if line.strip() and normal._cv_strip_pay_from_summary_counted([line])[1]:
            removed += 1
            continue
        out.append(line)
    return "\n".join(out), removed


def prepare_source(text: str) -> str:
    return str(pipeline.prepare_source(text))


def docx_bullet_levels(data: bytes) -> list[Any]:
    """The Word file's own list depth (bullets under bullets), when it is one."""
    try:
        levels = _extract_docx_bullet_levels(data)
    except Exception:  # noqa: BLE001 - a damaged file: the rules infer the depth instead
        return []
    return levels if isinstance(levels, list) else []


def merge_levels(*lists: list[Any]) -> list[Any]:
    return list(_merge_bullet_level_lists(*[x for x in lists if isinstance(x, list)]))


def after_parse(
    parsed: dict[str, Any], source: str, *, correct_language: bool, keep_source_summary: bool = False
) -> dict[str, Any]:
    """CV Studio's checks of the AI's reading. For a blind CV the CV's own
    About / Summary section is kept as written (CV Studio's blind mode): the
    blind step then anonymises those same bullets, one for one."""
    return dict(
        pipeline.postprocess_parse(
            parsed, source, preserve_source_summary=keep_source_summary, correct_language=correct_language
        )
    )


# What CV Studio's /blind route sends before the CV's JSON.
BLIND_REQUEST = "Anonymise this parsed CV JSON:\n\n"


def prepare_blind(cv: dict[str, Any]) -> tuple[dict[str, Any], int]:
    """CV Studio's /blind preparation: the summary taken from an About /
    Summary group of skills when there is no other, then lines stating the
    candidate's pay removed from it before the AI sees it. Returns the
    prepared CV (the one the blind checks compare with) and the count removed."""
    prepared = dict(blind_mask._blind_prepare_summary_bullets(cv))
    removed = 0
    if isinstance(prepared.get("summary_bullets"), list):
        prepared["summary_bullets"], removed = normal._cv_strip_pay_from_summary_counted(
            prepared["summary_bullets"]
        )
    return prepared, int(removed)


def blind_request(prepared: dict[str, Any]) -> str:
    return BLIND_REQUEST + json.dumps(prepared, ensure_ascii=False)


def blind_refusal(exc: Exception) -> str | None:
    """CV Studio's own reason for refusing an anonymised answer (a fixed text,
    e.g. a summary bullet was lost), or None for any other error, whose text
    could quote the CV."""
    text = str(exc)
    if isinstance(exc, ValueError) and text.startswith("Blind CV "):
        return "CV Studio's blind checks stopped it: " + text
    return None


def after_blind(blinded: dict[str, Any], prepared: dict[str, Any]) -> dict[str, Any]:
    """CV Studio's five checks after the AI's anonymised answer, in its order,
    against the prepared CV it was given (never copying the original back)."""
    return dict(pipeline.postprocess_blind(blinded, prepared))


def render(data: dict[str, Any], *, alignment: str, bullet_levels: list[Any]) -> bytes:
    """The Word file, from CV Studio's renderer and template."""
    prepared = pipeline.prepare_export(data, alignment=alignment, bullet_levels=bullet_levels)
    with tempfile.TemporaryDirectory(prefix="hp-cvstudio-") as tmp:
        payload, output = Path(tmp) / "render-input.json", Path(tmp) / "cv.docx"
        payload.write_text(json.dumps(prepared, ensure_ascii=False), encoding="utf-8")
        done = subprocess.run(  # noqa: S603 - fixed program and script, our own files
            ["node", str(ENGINE / "generate.js"), str(payload), str(output)],  # noqa: S607 - node from PATH
            capture_output=True,
            timeout=RENDER_TIMEOUT_SECONDS,
            check=False,
        )
        if done.returncode != 0 or not output.exists():
            raise RuntimeError("CV Studio's renderer failed: " + done.stderr.decode(errors="replace")[-500:])
        return output.read_bytes()


def answer_json(text: str) -> dict[str, Any] | None:
    """The JSON object in an AI answer (fences and chatter around it ignored)."""
    clean = text.strip()
    if clean.startswith("```"):
        clean = clean.strip("`")
        clean = clean[clean.find("{") :] if "{" in clean else clean
    start, end = clean.find("{"), clean.rfind("}")
    if start < 0 or end <= start:
        return None
    try:
        value = json.loads(clean[start : end + 1])
    except ValueError:
        return None
    return value if isinstance(value, dict) else None


_PARAGRAPH = re.compile(rb"<w:p[ >].*?</w:p>", re.S)
_RUN_TEXT = re.compile(rb"<w:(?:t|delText|instrText)(?: [^>]*)?>([^<]*)</w:(?:t|delText|instrText)>")
_TARGET = re.compile(rb'Target="([^"]*)"')
_TAG = re.compile(rb"<[^>]*>")
_MAX_PART = 20_000_000


def word_text(docx: bytes) -> str:
    """A Word file's text as a reader sees it: each paragraph's runs joined
    (Word splits words across runs), its properties and every link's address."""
    out: list[str] = []
    with zipfile.ZipFile(io.BytesIO(docx)) as z:
        for info in z.infolist():
            if info.file_size > _MAX_PART:
                raise ValueError("a part of the Word file is too large")
            name, data = info.filename, z.read(info)
            if name.endswith(".rels"):
                out += [html.unescape(m.decode("utf-8", "replace")) for m in _TARGET.findall(data)]
            elif name.startswith("word/") and name.endswith(".xml"):
                for paragraph in _PARAGRAPH.findall(data):
                    out.append(
                        html.unescape(b"".join(_RUN_TEXT.findall(paragraph)).decode("utf-8", "replace"))
                    )
            elif name.endswith(".xml"):  # properties: author, title, company...
                out.append(html.unescape(_TAG.sub(b"\n", data).decode("utf-8", "replace")))
    return "\n".join(out)


# What CV Studio's sweep puts in place of each kind of identity.
_SWEEP_MARKERS = (
    ("name", re.compile(r"the candidate", re.I)),
    ("email", re.compile(re.escape("[Email Redacted]"))),
    ("phone", re.compile(re.escape("[Phone Redacted]"))),
    ("link", re.compile(re.escape("[Link Redacted]"))),
)


def identity_left(docx: bytes, candidate: dict[str, Any]) -> list[str]:
    """What of the candidate's identity a finished blind Word file still
    shows, by CV Studio's own sweep: whatever it would still replace in the
    file's text (body, headers, footers, properties, links) is left in it,
    named by the marker the sweep would put there."""
    text = word_text(docx)
    swept = blind_mask._blind_scrub_candidate_identity({"text": text}, {"candidate": dict(candidate)})["text"]
    if swept == text:
        return []
    kinds = [
        kind for kind, marker in _SWEEP_MARKERS if len(marker.findall(swept)) > len(marker.findall(text))
    ]
    return kinds or ["details"]

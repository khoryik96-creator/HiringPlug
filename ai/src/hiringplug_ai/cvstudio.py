"""CV Studio's formatting engine (vendored unchanged in ai/cvstudio): its rule
modules, prompts and Word renderer, behind a few functions.

The engine's modules import each other by plain name, so their folder is put
on the module path here, once. Its renderer is a Node script (generate.js)
run as a child process with a time limit.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

ROOT = Path(os.environ.get("HP_AI_CVSTUDIO_DIR") or Path(__file__).resolve().parents[2] / "cvstudio")
ENGINE = ROOT / "engine"
if str(ENGINE) not in sys.path:
    sys.path.insert(0, str(ENGINE))

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


def after_parse(parsed: dict[str, Any], source: str, *, correct_language: bool) -> dict[str, Any]:
    return dict(pipeline.postprocess_parse(parsed, source, correct_language=correct_language))


def after_blind(blinded: dict[str, Any], original: dict[str, Any]) -> dict[str, Any]:
    return dict(pipeline.postprocess_blind(blinded, original))


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

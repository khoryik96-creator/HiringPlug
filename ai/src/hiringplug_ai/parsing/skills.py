"""Skills from the CV's skills section(s): one label per item, never guessed from prose."""

from __future__ import annotations

import re

from .dates import find_range
from .layout import BULLET, Line
from .vocab import is_sentence

_SPLIT = re.compile(r"\s*(?:[,;|•·▪●]|\t|\s{3,}|\s+/\s+)\s*")
_LABEL = re.compile(r"^([A-Za-z][A-Za-z &/-]{1,30})\s*[:：]\s*")
# Lines that are personal facts or languages, not skills, even under a skills heading.
_NOT_SKILLS = re.compile(
    r"^(?:nationality|citizenship|notice|availability|address|date of birth|dob|age|gender|religion|race|"
    r"marital|salary|expected|current salary|languages?|bahasa|spoken|written|driving|licen[cs]e|"
    r"references?)\b",
    re.IGNORECASE,
)
_LEVELS = r"(?:basic|beginner|intermediate|advanced|expert|proficient|fluent|good|excellent|average|native)"
# "Excel (Advanced)", "Python - expert": the level goes, the skill stays. Other brackets stay.
_LEVEL = re.compile(rf"\s*(?:\((?:very\s+)?{_LEVELS}[^)]{{0,20}}\)|[-–:]\s*{_LEVELS}\b.*)$", re.IGNORECASE)


_LEVEL_ONLY = re.compile(
    r"^(?:very\s+)?(?:basic|beginner|intermediate|advanced|expert|proficient|fluent|good|excellent|average|"
    r"native|strong|fair|poor|working|professional)$",
    re.IGNORECASE,
)
_FILLER = {"and", "or", "&", "etc", "with", "of", "the", "others"}


def _split(text: str) -> list[str]:
    """Split on list separators, but not inside brackets: "AWS (EC2, EKS, S3)" stays one item."""
    items: list[str] = []
    depth, last = 0, 0
    for m in re.finditer(r"[()\[\]]|" + _SPLIT.pattern, text):
        mark = m.group(0)
        if mark in "([":
            depth += 1
        elif mark in ")]":
            depth = max(0, depth - 1)
        elif depth == 0:
            items.append(text[last : m.start()])
            last = m.end()
    items.append(text[last:])
    return items


def _keep(value: str) -> bool:
    if _LEVEL_ONLY.match(value) or value.lower() in _FILLER:
        return False
    if re.search(r"(.)\1{4,}", value):  # rating bars drawn with letters: "lllllll"
        return False
    # "original and resourceful": the tail of a sentence split at a comma
    return not (value[0].islower() and " and " in value)


def _skill_lines(lines: list[Line]) -> list[str]:
    """The skills section's lines, with lines wrapped inside brackets joined."""
    out: list[str] = []
    for line in lines:
        if line.section != "skills" or line.heading:
            continue
        text = BULLET.sub("", line.text)
        if out and out[-1].count("(") > out[-1].count(")"):
            out[-1] = f"{out[-1]} {text}"
        else:
            out.append(text)
    return out


def skills(lines: list[Line]) -> list[tuple[str, str]]:
    found: list[tuple[str, str]] = []
    seen: set[str] = set()
    for text in _skill_lines(lines):
        if _NOT_SKILLS.match(text) or is_sentence(text) or find_range(text):
            continue  # personal facts, prose, or a job line that ended up under the heading
        label = _LABEL.match(text)
        items = text[label.end() :] if label and len(label.group(1).split()) <= 3 else text
        for item in _split(items):
            quote = item.strip(" .-–")
            value = _LEVEL.sub("", quote).strip(" .-–")
            if (
                not value
                or len(value) > 60
                or len(value.split()) > 5
                or value.lower() in seen
                or not _keep(value)
            ):
                continue
            if not re.search(r"[^\W\d_]", value):
                continue
            seen.add(value.lower())
            found.append((value, quote))
            if len(found) == 100:
                return found
    return found

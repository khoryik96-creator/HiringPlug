"""Skills from the CV's skills section(s): one label per item, never guessed from prose."""

from __future__ import annotations

import re

from .layout import BULLET, Line

_SPLIT = re.compile(r"\s*(?:[,;|•·▪●]|\t|\s{3,}|\s+/\s+)\s*")
_LABEL = re.compile(r"^([A-Za-z][A-Za-z &/-]{1,30})\s*[:：]\s*")
# Lines that are personal facts or languages, not skills, even under a skills heading.
_NOT_SKILLS = re.compile(
    r"^(?:nationality|citizenship|notice|availability|address|date of birth|dob|age|gender|religion|race|"
    r"marital|salary|expected|current salary|languages?|bahasa|spoken|written|driving|licen[cs]e|"
    r"references?)\b",
    re.IGNORECASE,
)
_LEVEL = re.compile(
    r"\s*(?:\((?:[^)]{0,30})\)|[-–:]\s*(?:basic|beginner|intermediate|advanced|expert|proficient|fluent|good|"
    r"excellent)\b.*)$",
    re.IGNORECASE,
)


def skills(lines: list[Line]) -> list[tuple[str, str]]:
    found: list[tuple[str, str]] = []
    seen: set[str] = set()
    for line in lines:
        if line.section != "skills" or line.heading:
            continue
        text = BULLET.sub("", line.text)
        if _NOT_SKILLS.match(text):
            continue
        label = _LABEL.match(text)
        if label and len(label.group(1).split()) <= 3:
            text = text[label.end() :]
        for item in _SPLIT.split(text):
            quote = item.strip(" .-–")
            value = _LEVEL.sub("", quote).strip(" .-–")
            if not value or len(value) > 60 or len(value.split()) > 5 or value.lower() in seen:
                continue
            if not re.search(r"[^\W\d_]", value):
                continue
            seen.add(value.lower())
            found.append((value, quote))
            if len(found) == 100:
                return found
    return found

"""Education: qualification, institution and year, by their words (Bachelor,
Diploma, SPM ...; University, Kolej, Politeknik ...), in the education section."""

from __future__ import annotations

import re
from dataclasses import dataclass

from .dates import RANGE_RE, YEAR_RE, find_range
from .experience import trim
from .layout import BULLET, Line
from .vocab import INSTITUTION_RE, QUALIFICATION_RE, strip_duration

_SPLIT = re.compile(r"\s*\|\s*|\s+[-–—]\s+|\s+from\s+|\s+at\s+|,\s+(?=[A-Z0-9(])|\s{3,}")
_DANGLING = re.compile(r"\b(?:of|and|in|from|at|for|the)$|[&,]$", re.I)
_FINISHED = re.compile(r"^(?:finished|graduated|completed|expected)\b[:\s]*", re.I)


@dataclass
class School:
    qualification: str | None = None
    institution: str | None = None
    dates: str | None = None
    end_year: int | None = None


def _lines(lines: list[Line]) -> list[str]:
    out: list[str] = []
    for x in lines:
        if x.section != "education" or x.heading:
            continue
        text = BULLET.sub("", x.text)
        if out and _DANGLING.search(
            out[-1]
        ):  # "Bachelor's Degree ... from University of" + "Tenaga Nasional"
            out[-1] = f"{out[-1]} {text}"
        else:
            out.append(text)
    return out


def education(lines: list[Line]) -> list[School]:
    schools: list[School] = []
    current = School()

    def push() -> None:
        nonlocal current
        if current.qualification or current.institution:
            schools.append(current)
        current = School()

    for text in _lines(lines):
        if len(text) > 200:
            continue
        dates = find_range(text)
        years = [int(y) for y in YEAR_RE.findall(text)]
        for raw in _SPLIT.split(strip_duration(RANGE_RE.sub(" ", text))):
            piece = trim(_FINISHED.sub("", raw))
            if len(piece) < 2 or YEAR_RE.fullmatch(piece):
                continue
            if INSTITUTION_RE.search(piece) and not QUALIFICATION_RE.match(piece):
                if current.institution:
                    push()
                current.institution = piece
            elif QUALIFICATION_RE.search(piece) and len(piece.split()) <= 16:
                if current.qualification:
                    push()
                current.qualification = piece
        if dates or years:
            if current.dates and (current.qualification or current.institution) and current.end_year:
                push()
            current.dates = dates.quote if dates else str(years[-1])
            if dates and dates.current:
                current.end_year = None
            else:
                last = (dates.end or dates.start) if dates else str(max(years))
                current.end_year = int(last[:4]) if last else None
    push()
    return schools[:15]

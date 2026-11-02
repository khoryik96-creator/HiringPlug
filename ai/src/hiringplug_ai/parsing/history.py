"""Work history and education.

A job is anchored on a date range ("Jan 2020 – Present"). Its title and
company are on the same line ("Accountant | ABC Sdn Bhd | 2020 - 2022",
"Accountant at ABC Sdn Bhd") or on the short lines around it, in whatever
order the CV uses: company-signal words (Sdn Bhd, Pte Ltd, Berhad, ...) and
role words decide which is which (Kano's rules v147, v362, v403).
"""

from __future__ import annotations

import re
from dataclasses import dataclass

from .dates import RANGE_RE, YEAR_RE, DateRange, find_range
from .layout import BULLET, Line, is_bullet

COMPANY_RE = re.compile(
    r"\b(?:sdn\.?\s*bhd\.?|bhd\.?|berhad|pte\.?\s*ltd\.?|ltd\.?|limited|llc|llp|inc\.?|corp\.?|corporation|plc|"
    r"gmbh|co\.|company|group|bank|holdings?|technolog(?:y|ies)|solutions?|systems|consult(?:ing|ancy)|services|"
    r"industries|enterprise|resources|international|global|partners|associates|capital|logistics|"
    r"hospital|clinic|agency|firm)\b",
    re.IGNORECASE,
)
ROLE_RE = re.compile(
    r"\b(?:engineer|developer|programmer|architect|manager|lead|leader|analyst|consultant|director|specialist|"
    r"executive|administrator|admin|officer|designer|scientist|head|vp|cto|cfo|ceo|coo|accountant|assistant|"
    r"clerk|intern|internship|trainee|supervisor|technician|coordinator|associate|recruiter|auditor|"
    r"representative|advisor|adviser|nurse|teacher|lecturer|tutor|cashier|driver|chef|cook|operator|"
    r"controller|planner|buyer|merchandiser|tester|writer|editor|partner|founder|owner|secretary|"
    r"receptionist|agent|attendant|worker|staff|support|president|chairman|pharmacist|doctor|surgeon|"
    r"therapist|researcher|instructor|trainer|strategist|investigator|inspector|surveyor|draughtsman)\b",
    re.IGNORECASE,
)
_SPLIT_RE = re.compile(r"\s*\|\s*|\s+(?:at|@)\s+|\s+[-–—]\s+|\s*,\s+(?=[A-Z0-9])")
_DUTY_START = re.compile(
    r"^(?:responsible|manag(?:e|ed|ing)|assist(?:ed|ing)?|handl(?:e|ed|ing)|prepar(?:e|ed|ing)|develop(?:ed|ing)?|"
    r"led|lead\s+a|coordinat(?:e|ed|ing)|ensur(?:e|ed|ing)|perform(?:ed|ing)?|maintain(?:ed|ing)?|"
    r"key\s+(?:responsibilities|achievements)|achievements?|duties|client\s*:|project\s*:|reporting\s+to)",
    re.IGNORECASE,
)


@dataclass(frozen=True)
class Piece:
    text: str


@dataclass(frozen=True)
class Job:
    title: str | None
    company: str | None
    dates: DateRange


def _headline(text: str) -> bool:
    """A short line that can be a title or company: not a bullet, a sentence or a duty."""
    words = text.split()
    return (
        bool(text)
        and not is_bullet(text)
        and len(words) <= 12
        and len(text) <= 120
        and not text.endswith(".")
        and not _DUTY_START.match(text)
        and "@" not in text.replace(" @ ", "")
    )


def _trim(text: str) -> str:
    """Strip separators, and brackets only when unbalanced: "(Hons)" stays whole."""
    text = text.strip(" ,;:|-–—")
    while text and text[-1] in ")]" and text.count("(") + text.count("[") < text.count(")") + text.count("]"):
        text = text[:-1].strip()
    while text and text[0] in "([" and text.count("(") + text.count("[") > text.count(")") + text.count("]"):
        text = text[1:].strip()
    return text


def _pieces(text: str) -> list[str]:
    parts = [_trim(p) for p in _SPLIT_RE.split(text)]
    return [p for p in parts if len(p) >= 2 and not YEAR_RE.fullmatch(p)]


def _kind(text: str) -> str:
    company, role = bool(COMPANY_RE.search(text)), bool(ROLE_RE.search(text))
    if company and not role:
        return "company"
    if role and not company:
        return "title"
    if company and role:  # "Finance Manager, Bank X" is rare on one piece; prefer the legal suffix
        return "company" if re.search(r"sdn|bhd|berhad|pte|ltd|inc|llc|corp", text, re.I) else "title"
    return ""


def _assign(pieces: list[str], at_split: bool) -> tuple[str | None, str | None]:
    title = company = None
    unknown: list[str] = []
    for p in pieces:
        kind = _kind(p)
        if kind == "title" and title is None:
            title = p
        elif kind == "company" and company is None:
            company = p
        elif not kind:
            unknown.append(p)
    if at_split and len(pieces) == 2 and not (title and company):
        return pieces[0], pieces[1]  # "Accountant at Acme": title first
    for p in unknown:
        if title is None:
            title = p
        elif company is None:
            company = p
    return title, company


def experience(lines: list[Line]) -> list[Job]:
    section = [x for x in lines if x.section == "experience" and not x.heading]
    if not section:  # no heading: use date ranges outside education
        section = [x for x in lines if x.section in ("", "other") and not x.heading]
    anchors = [i for i, x in enumerate(section) if find_range(x.text)]
    jobs: list[Job] = []
    for n, i in enumerate(anchors):
        line = section[i].text
        dates = find_range(line)
        if dates is None:  # anchors were chosen by find_range
            continue
        rest = _trim(RANGE_RE.sub(" ", line, count=1))
        rest = BULLET.sub("", rest)
        pieces = _pieces(rest) if rest and _headline(rest) else []
        at_split = bool(re.search(r"\s(?:at|@)\s", rest))
        # Short lines just above (after the previous job's own lines) and just below.
        previous = anchors[n - 1] if n else -1
        above = [section[j].text for j in range(max(previous + 1, i - 2), i) if _headline(section[j].text)]
        below_end = anchors[n + 1] if n + 1 < len(anchors) else len(section)
        below = [section[j].text for j in range(i + 1, min(below_end, i + 3)) if _headline(section[j].text)]
        title, company = _assign(pieces, at_split)
        for options in (above[-2:], below[:2]):
            if title and company:
                break
            for text in options:
                at = bool(re.search(r"\s(?:at|@)\s", text))
                t, c = _assign(_pieces(text), at) if at else (None, None)
                kind = _kind(text)
                if at:
                    title, company = title or t, company or c
                elif kind == "title" and not title:
                    title = text
                elif kind == "company" and not company:
                    company = text
        if not (title or company):
            continue
        jobs.append(Job(title=title, company=company, dates=dates))
    return jobs[:30]


QUALIFICATION_RE = re.compile(
    r"\b(?:bachelor|bachelors|degree|ijazah|diploma|master|masters|mba|phd|ph\.d|doctor(?:ate)?|certificate|"
    r"sijil|spm|stpm|uec|a-levels?|o-levels?|igcse|foundation|asasi|matriculation|matrikulasi|b\.?sc|b\.?a\b|"
    r"b\.?eng|m\.?sc|m\.?eng|b\.?com|bba|llb|acca|cpa|cima|cfa|hnd|advanced\s+diploma|honours|hons)\b",
    re.IGNORECASE,
)
INSTITUTION_RE = re.compile(
    r"\b(?:universit(?:y|i|as)|college|kolej|institute|institut|polytechnic|politeknik|school|sekolah|academy|"
    r"akademi|uitm|ukm|usm|upm|utm|utar|unimas|ums|uum|mmu|apu|segi|inti|sunway|taylor'?s|monash|nottingham|"
    r"campus|smk|sjk|nus|ntu|smu)\b",
    re.IGNORECASE,
)


@dataclass
class School:
    qualification: str | None = None
    institution: str | None = None
    dates: str | None = None
    end_year: int | None = None


def education(lines: list[Line]) -> list[School]:
    section = [x for x in lines if x.section == "education" and not x.heading]
    schools: list[School] = []
    current = School()

    def push() -> None:
        nonlocal current
        if current.qualification or current.institution:
            schools.append(current)
        current = School()

    for line in section:
        text = BULLET.sub("", line.text)
        if len(text) > 160:
            continue
        dates = find_range(text)
        year = YEAR_RE.findall(text)
        for piece in _pieces(RANGE_RE.sub(" ", text)) or [text]:
            if INSTITUTION_RE.search(piece):
                if current.institution:
                    push()
                current.institution = piece
            elif QUALIFICATION_RE.search(piece) and len(piece.split()) <= 16:
                if current.qualification:
                    push()
                current.qualification = piece
        if dates or year:
            if current.dates and (current.qualification or current.institution):
                push()
            current.dates = dates.quote if dates else year[-1]
            last = (dates.end or dates.start) if dates and not dates.current else (year[-1] if year else None)
            current.end_year = int(last[:4]) if last and not (dates and dates.current) else None
    push()
    return schools[:15]

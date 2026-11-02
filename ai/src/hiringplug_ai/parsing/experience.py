"""Work history.

A job is anchored on its date range ("Jan 2020 – Present"). Its title and
company come from the rest of that line ("Accountant | ABC Sdn Bhd",
"Accountant at ABC Sdn Bhd", "ABC Sdn Bhd - Accountant (2020 - Present)"),
from labelled lines ("Company:", "Role:"), or from the short lines next to it.
Whether those lines come before or after the dates is decided once per CV,
from the first job: CVs keep one layout throughout.

Rules from Kano (company and role words, "Title at Company", company line
above a dated role) and from real CVs: durations such as "(2 years 3 months)",
places ("Kuala Lumpur, Malaysia", "India / Remote"), numbering ("1. Kuwait
Finance House"), "X client: Y" (X is the employer), and titles followed by a
duty ("Head of Sales - Lead the key account team").
"""

from __future__ import annotations

import re
from dataclasses import dataclass

from .dates import DateRange, find_range
from .layout import BULLET, Line, is_bullet
from .vocab import (
    COMPANY_RE,
    INSTITUTION_RE,
    LEGAL_SUFFIX_RE,
    QUALIFICATION_RE,
    ROLE_RE,
    is_place,
    is_sentence,
    strip_duration,
)


@dataclass(frozen=True)
class Job:
    title: str | None
    company: str | None
    dates: DateRange


_COMPANY_LABEL = re.compile(
    r"^(?:company(?:\s+name)?|employer|organi[sz]ation|syarikat)\s*[:：]\s*(.+)$", re.I
)
_TITLE_LABEL = re.compile(r"^(?:role|position|designation|job\s*title|title|jawatan)\s*[:：]\s*(.+)$", re.I)
_OTHER_LABEL = re.compile(
    r"^(?:client|customer|project(?:\s+name)?|duration|period|location|team|reporting\s+to|industry|"
    r"responsibilit\w*|achievements?|key\s+\w+|scope|environment|technolog\w*|tools?)\s*[:：]",
    re.I,
)
_NUMBERING = re.compile(r"^\(?\d{1,2}[.)]\s+")
_CLIENT = re.compile(
    r"\s*[-–(]?\s*\bclients?\s*[:：][^|]*", re.I
)  # "Coforge client :UOB | DBA" -> employer Coforge
_STRONG_SPLIT = re.compile(r"\s*\|\s*|\s+[-–—]\s+|\s+·\s+|\s*•\s*|\s{3,}")
_DASHED = re.compile(r"\s[-–—]\s")
_COMMA_SPLIT = re.compile(r",\s+(?=[A-Z0-9(])")
_AT = re.compile(r"\s+(?:at|@)\s+")
_DANGLING = re.compile(r"(?:\b(?:of|and|at|from|in|for|the)|[&,\-–])$", re.I)
_DUTY_START = re.compile(
    r"^(?:responsible|manag(?:e|ed|ing)|assist(?:ed|ing)?|handl(?:e|ed|ing)|prepar(?:e|ed|ing)|develop(?:ed|ing)?|"
    r"led|lead\s+(?:a|the)\b|coordinat(?:e|ed|ing)|ensur(?:e|ed|ing)|perform(?:ed|ing)?|maintain(?:ed|ing)?|"
    r"provid(?:e|ed|ing)|support(?:ed|ing)|implement(?:ed|ing)?|work(?:ed|ing)|key\s+(?:responsibilities|"
    r"achievements)|achievements?|duties|reporting\s+to)\b",
    re.IGNORECASE,
)


def trim(text: str) -> str:
    """Strip separators, a sentence's full stop ("consultant." but not "Inc."), and
    brackets only when unbalanced: "(Hons)" stays whole."""
    text = text.strip(" ,;:|-–—·")
    if re.search(r"[a-z]{5,}\.$", text):
        text = text[:-1]
    while text and text[-1] in ")]" and text.count("(") + text.count("[") < text.count(")") + text.count("]"):
        text = text[:-1].strip(" ,;:|-–—·")
    while text and text[0] in "([" and text.count("(") + text.count("[") > text.count(")") + text.count("]"):
        text = text[1:].strip(" ,;:|-–—·")
    return text


_URL = re.compile(r"(?:https?://|www\.)\S+|\S*linkedin\.com\S*|\S+@\S+\.\w+", re.IGNORECASE)


def _clean(text: str) -> str:
    text = _NUMBERING.sub("", BULLET.sub("", _URL.sub(" ", text)))
    text = re.sub(r"\(\s*\)", " ", strip_duration(text))
    return trim(re.sub(r"\s+", " ", text))


def headline(text: str) -> bool:
    """A short line that can hold a title or a company: not a bullet, a sentence or a duty."""
    words = text.split()
    return (
        bool(text)
        and not is_bullet(text)
        and len(words) <= 14
        and len(text) <= 140
        and not re.search(r"[a-z]{4,}\.$", text)  # ends like a sentence (but "Sdn. Bhd." is fine)
        and not _DUTY_START.match(text)
        and not _OTHER_LABEL.match(text)
        and not is_sentence(text)
    )


def kind(piece: str) -> str:
    """'company', 'title', '' (could be either) or 'skip' (a place, a sentence, a school)."""
    if not re.search(r"[^\W\d_]{2}", piece) or is_place(piece) or is_sentence(piece):
        return "skip"
    if LEGAL_SUFFIX_RE.search(piece):
        return "company"
    company, role = bool(COMPANY_RE.search(piece)), bool(ROLE_RE.search(piece))
    if not (company or role) and (INSTITUTION_RE.search(piece) or QUALIFICATION_RE.search(piece)):
        return "skip"
    if company and not role:
        return "company"
    if role:
        return "title"
    return ""


def _without_places(segment: str) -> str:
    """ "CIMB Bank, Kuala Lumpur" -> "CIMB Bank"."""
    parts = _COMMA_SPLIT.split(segment)
    kept = [p for p in parts if not is_place(_clean(p))]
    return _clean(", ".join(kept)) if kept else ""


Found = tuple[str | None, str | None, list[str]]


def from_text(text: str) -> Found:
    """Title, company, and names that could be either, from one line (or a
    line joined with its continuation)."""
    text = _CLIENT.sub(" ", text)
    if (m := list(_AT.finditer(text))) and kind(_clean(text[: m[-1].start()])) != "skip":
        head = _clean(text[: m[-1].start()])
        employer = _without_places(
            _STRONG_SPLIT.split(text[m[-1].end() :])[0]
        )  # "Acme · Bangalore" -> "Acme"
        return head or None, employer or None, []
    segments = [x for x in (_without_places(seg) for seg in _STRONG_SPLIT.split(text)) if len(x) >= 2]
    if len(segments) == 1 and kind(segments[0]) == "":  # one segment, maybe "Title, Company"
        segments = [x for x in (_clean(p) for p in _COMMA_SPLIT.split(segments[0])) if len(x) >= 2]
    title, company, unknown = _assign(segments)
    if company and title:
        title = _whole_title(text, title, company)
    elif not company and title and _DASHED.search(text):
        # "IT Engineer - Storage and Backup DevOps Engineer": dashes inside one title.
        whole = _clean(text)
        if all(kind(seg) == "title" for seg in segments) and kind(whole) == "title" and headline(whole):
            title, unknown = whole, []
    return title, company, unknown


def _assign(segments: list[str]) -> Found:
    title = company = None
    unknown: list[str] = []
    mixed: list[str] = []
    for seg in segments:
        k = kind(seg)
        if "," in seg and COMPANY_RE.search(seg) and ROLE_RE.search(seg):
            mixed.append(seg)
        if k == "company" and company is None:
            company = seg
        elif k == "title" and title is None:
            title = seg
        elif k == "":
            unknown.append(seg)
    for seg in mixed:  # "Integration Stream Lead, City Savings Bank", when nothing better was found
        parts = [_clean(p) for p in _COMMA_SPLIT.split(seg)]
        kinds = [kind(p) for p in parts]
        if company is None and "title" in kinds and "company" in kinds:
            title, company = parts[kinds.index("title")], parts[kinds.index("company")]
    return title, company, unknown


def _whole_title(text: str, title: str, company: str) -> str:
    """The whole rest of the line is the title when it reads as one:
    "Dell Technologies - Senior Advisor - Database Administrator"."""
    rest = text.replace(company, " ", 1)
    for seg in _STRONG_SPLIT.split(rest):
        if seg.strip() and is_place(_clean(seg)):
            rest = rest.replace(seg, " ", 1)
    rest = _clean(rest)
    whole = (
        title in rest
        and "|" not in rest
        and rest.count(",") <= title.count(",")
        and kind(rest) == "title"
        and headline(rest)
    )
    return rest if whole else title


def _labelled(window: list[str]) -> tuple[str | None, str | None]:
    title = company = None
    for text in window:
        if (m := _COMPANY_LABEL.match(text)) and not company:
            company = _clean(m.group(1)) or None
        elif (m := _TITLE_LABEL.match(text)) and not title:
            title = _clean(m.group(1)) or None
    return title, company


def _joined(texts: list[str]) -> list[str]:
    """Join wrapped lines: "Senior Advisor at Dell" + "Technologies", "University of" + "Malaya"."""
    out: list[str] = []
    for t in texts:
        if out and (_DANGLING.search(out[-1]) or (_AT.search(out[-1]) and len(t.split()) <= 2)):
            out[-1] = f"{out[-1]} {t}"
        else:
            out.append(t)
    return out


_OPEN_RANGE = re.compile(r"(?:\b(?:19|20)\d\d|\b[a-z]{3,9}\.?)\s*(?:[-–—]|\bto)\s*\(?$", re.IGNORECASE)


def _unwrap_dates(lines: list[tuple[str, str]]) -> list[tuple[str, str]]:
    """ "Dell Technologies | May 2020 –" + "Present": a date range broken over two lines."""
    out: list[tuple[str, str]] = []
    for text, sec in lines:
        if (
            out
            and _OPEN_RANGE.search(out[-1][0])
            and not find_range(out[-1][0])
            and find_range(f"{out[-1][0]} {text}")
        ):
            out[-1] = (f"{out[-1][0]} {text}", out[-1][1])
        else:
            out.append((text, sec))
    return out


def _clear_job(text: str) -> bool:
    """A dated line that is plainly a job wherever it sits: "DBA | Acme Sdn. Bhd. | 2009 - 2011"
    (a sidebar or a page break can put work history under another heading)."""
    dates = find_range(text)
    if not dates:
        return False
    title, company, _ = from_text(text.replace(dates.quote, " ", 1))
    return bool(title and company and (LEGAL_SUFFIX_RE.search(company) or ROLE_RE.search(title)))


def experience(lines: list[Line]) -> list[Job]:
    tagged = _unwrap_dates([(x.text, x.section) for x in lines if not x.heading and x.section != "education"])
    anchors = [
        i for i, (t, sec) in enumerate(tagged) if find_range(t) and (sec == "experience" or _clear_job(t))
    ]
    if not any(tagged[i][1] == "experience" for i in anchors):  # no dated work-history section
        tagged = [(t, sec) for t, sec in tagged if sec in ("", "other")]
        anchors = [i for i, (t, _) in enumerate(tagged) if find_range(t)]
    if not anchors:
        return []
    section = [t for t, _ in tagged]
    # Dates first (title and company below them) when the first job starts with its dates.
    first = anchors[0]
    before = [t for t, sec in tagged[:first] if sec == tagged[first][1]][-3:]
    dates_first = not any(headline(t) for t in before)
    jobs: list[Job] = []
    for n, i in enumerate(anchors):
        start = anchors[n - 1] + 1 if n else 0
        end = anchors[n + 1] if n + 1 < len(anchors) else len(section)
        # Context stays within the job line's own section.
        while start < i and tagged[start][1] != tagged[i][1]:
            start += 1
        end = next((j for j in range(i + 1, end) if tagged[j][1] != tagged[i][1]), end)
        job = _job(section, i, start, end, dates_first=dates_first)
        if job:
            jobs.append(job)
    return jobs[:30]


def _headlines(section: list[str], indexes: range) -> list[str]:
    """Consecutive short lines from the job line outwards, at most three."""
    out: list[str] = []
    for j in indexes:
        text = section[j]
        if not headline(text):
            # "Head of Sales - Lead and develop the key account team": the title before the duty.
            first = _STRONG_SPLIT.split(text)[0]
            if first == text or not headline(first) or kind(_clean(first)) != "title":
                break
            text = first
        out.append(text)
        if len(out) == 3:
            break
    return out


def _job(section: list[str], i: int, start: int, end: int, *, dates_first: bool) -> Job | None:
    dates = find_range(section[i])
    if dates is None:
        return None
    rest = section[i].replace(dates.quote, " ", 1)
    title = company = None
    unknown: list[str] = []
    if not _OTHER_LABEL.match(rest.strip()) and headline(_clean(rest) or "x"):
        title, company, unknown = from_text(rest)
    lt, lc = _labelled(section[max(start, i - 4) : min(end, i + 5)])
    title, company = title or lt, company or lc
    above = _joined(_headlines(section, range(i - 1, start - 1, -1))[::-1])[::-1]  # nearest first
    below = _joined(_headlines(section, range(i + 1, end)))
    for text in [*below, *above] if dates_first else [*above, *below]:
        if title and company:
            break
        t, c, u = from_text(text)
        title, company = title or t, company or c
        unknown += u
    # Names without a title or company word fill what is still missing:
    # "ExxonMobil IT | Jan 2025 - Present" under a line with the title.
    for name in unknown:
        if title is None and name != company:
            title = name
        elif company is None and name != title:
            company = name
    if not (title or company) or _is_school(title, company):
        return None
    return Job(title=title, company=company, dates=dates)


def _is_school(title: str | None, company: str | None) -> bool:
    text = f"{title or ''} {company or ''}"
    return bool(INSTITUTION_RE.search(text) or QUALIFICATION_RE.search(text)) and not (
        ROLE_RE.search(text) or LEGAL_SUFFIX_RE.search(text)
    )

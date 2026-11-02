"""Dates and date ranges as CVs write them: "Jan 2020 – Present", "03/2019 - 12/2021", "2015 to 2018"."""

from __future__ import annotations

import re
from dataclasses import dataclass

MONTHS = {
    "jan": 1,
    "feb": 2,
    "mar": 3,
    "apr": 4,
    "may": 5,
    "jun": 6,
    "jul": 7,
    "aug": 8,
    "sep": 9,
    "oct": 10,
    "nov": 11,
    "dec": 12,
    # Malay
    "januari": 1,
    "februari": 2,
    "mac": 3,
    "mei": 5,
    "julai": 7,
    "ogos": 8,
    "okt": 10,
    "disember": 12,
}
_MONTH = (
    r"(?:jan(?:uary|uari)?|feb(?:ruary|ruari)?|mar(?:ch)?|mac|apr(?:il)?|may|mei|jun(?:e)?|jul(?:y|ai)?"
    r"|aug(?:ust)?|ogos|sep(?:t(?:ember)?)?|oct(?:ober)?|okt(?:ober)?|nov(?:ember)?|dec(?:ember)?|disember)"
)
_YEAR = r"(?:19[5-9]\d|20\d\d)"
_DAY = r"(?:0?[1-9]|[12]\d|3[01])(?:st|nd|rd|th)?"
DATE = (
    # Jan 2020, 1 Dec 2025, Dec 1, 2025
    rf"(?:(?:{_DAY}\s+)?{_MONTH}\.?(?:\s+{_DAY})?[,\-]?\s*['’]?\s*{_YEAR}"
    rf"|{_YEAR}\s*[-/ ]\s*{_MONTH}\b\.?"  # 2022-Oct, 2025 June (year first, from CV Studio)
    rf"|(?:0?[1-9]|1[0-2])\s*[/.\-]\s*{_YEAR}"  # 03/2019
    rf"|{_YEAR}\s*[/.\-]\s*(?:0?[1-9]|1[0-2])(?!\d)"  # 2019-03
    rf"|{_YEAR})"
)
_NOW = (
    r"(?:(?:till|until|up\s?to|to)\s?(?:date|now|present|current)|todate|present(?:ly)?|current(?:ly)?|now"
    r"|ongoing|today|kini|sekarang|masa\s+kini|date)"
)
RANGE_RE = re.compile(
    rf"(?<![\w/])(?P<start>{DATE})\s*(?:[-–—~]+|\bto\b|\buntil\b|\btill\b|\bhingga\b"
    # "2022-Oct 2023-May": two year-first dates side by side
    rf"|(?<=[a-z.])\s+(?=(?:19|20)\d\d\s*[-/ ]\s*[a-z]))\s*(?P<end>{DATE}|{_NOW})(?![\w/])",
    re.IGNORECASE,
)
YEAR_RE = re.compile(rf"(?<!\d){_YEAR}(?!\d)")
_NOW_RE = re.compile(rf"^{_NOW}$", re.IGNORECASE)
_SEP = r"\s*(?:[-–—~]+|\bto\b|\buntil\b|\btill\b)\s*"
# "Dec - Jan 2026", "1 Jul - 31 Aug, 2026": both months share the year at the end.
_SHARED_YEAR_RE = re.compile(
    rf"(?<![\w/])(?:{_DAY}\s+)?(?P<sm>{_MONTH})\.?(?:\s+{_DAY})?{_SEP}(?:{_DAY}\s+)?(?P<em>{_MONTH})\.?"
    rf"(?:\s+{_DAY})?,?\s*(?P<ey>{_YEAR})(?![\w/])",
    re.IGNORECASE,
)
# "Jan '20 - Dec '21", "Sep 99 - May 01", "Jan 2020 to Dec 21": two-digit years after a month.
_SHORT_YEAR_RE = re.compile(
    rf"(?<![\w/])(?P<sm>{_MONTH})\.?\s*['’]?(?P<sy>\d{{2}}|{_YEAR})(?!\d){_SEP}"
    rf"(?:(?P<em>{_MONTH})\.?\s*['’]?(?P<ey>\d{{2}}|{_YEAR})(?!\d)|(?P<now>{_NOW}))(?![\w/])",
    re.IGNORECASE,
)


def _month(word: str) -> int:
    w = word.lower().rstrip(".")
    return MONTHS.get(w) or MONTHS.get(w[:3]) or 0


def _year(text: str) -> int:
    n = int(text)
    return n if n > 99 else (1900 + n if n > 50 else 2000 + n)


def _other_forms(text: str) -> DateRange | None:
    if m := _SHARED_YEAR_RE.search(text):
        sm, em, ey = _month(m.group("sm")), _month(m.group("em")), int(m.group("ey"))
        sy = ey - 1 if sm > em else ey  # "Dec - Jan 2026" starts in 2025
        return DateRange(m.group(0).strip(), f"{sy}-{sm:02d}", f"{ey}-{em:02d}", False)
    if m := _SHORT_YEAR_RE.search(text):
        start = f"{_year(m.group('sy'))}-{_month(m.group('sm')):02d}"
        if m.group("now"):
            return DateRange(m.group(0).strip(), start, None, True)
        end = f"{_year(m.group('ey'))}-{_month(m.group('em')):02d}"
        return DateRange(m.group(0).strip(), start, end, False) if end >= start else None
    return None


@dataclass(frozen=True)
class DateRange:
    quote: str
    start: str | None  # YYYY-MM or YYYY
    end: str | None  # None when current
    current: bool


def to_iso(token: str) -> str | None:
    """'Jan 2020' -> '2020-01'; '03/2019' -> '2019-03'; '2015' -> '2015'."""
    t = token.strip().lower()
    year_match = YEAR_RE.search(t)
    if not year_match:
        return None
    year = year_match.group(0)
    word = re.search(r"[a-z]+", re.sub(r"(?<=\d)(?:st|nd|rd|th)\b", "", t))
    if word:
        month = MONTHS.get(word.group(0)) or MONTHS.get(word.group(0)[:3])
        return f"{year}-{month:02d}" if month else year
    numbers = [int(n) for n in re.findall(r"\d+", t) if len(n) <= 2]
    if numbers and 1 <= numbers[0] <= 12:
        return f"{year}-{numbers[0]:02d}"
    return year


def find_range(text: str) -> DateRange | None:
    m = RANGE_RE.search(text)
    if not m:
        return _other_forms(text)
    end_raw = m.group("end")
    current = bool(_NOW_RE.match(end_raw.strip()))
    start = to_iso(m.group("start"))
    end = None if current else to_iso(end_raw)
    if start and end and end < start:
        return None  # "2021 - 2019" is not a period of time
    return DateRange(m.group(0).strip(), start, end, current)

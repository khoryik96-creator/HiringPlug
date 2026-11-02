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
DATE = (
    rf"(?:{_MONTH}\.?,?\s*['’]?\s*{_YEAR}"  # Jan 2020, Sept. '2019
    rf"|(?:0?[1-9]|1[0-2])\s*[/.\-]\s*{_YEAR}"  # 03/2019
    rf"|{_YEAR}\s*[/.\-]\s*(?:0?[1-9]|1[0-2])(?!\d)"  # 2019-03
    rf"|{_YEAR})"
)
_NOW = r"(?:present|current(?:ly)?|now|to\s*date|till\s*date|until\s*now|ongoing|today|kini|sekarang)"
RANGE_RE = re.compile(
    rf"(?<![\w/])(?P<start>{DATE})\s*(?:[-–—~]+|\bto\b|\buntil\b|\bhingga\b)\s*(?P<end>{DATE}|{_NOW})(?![\w/])",
    re.IGNORECASE,
)
YEAR_RE = re.compile(rf"(?<!\d){_YEAR}(?!\d)")
_NOW_RE = re.compile(rf"^{_NOW}$", re.IGNORECASE)


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
    word = re.match(r"[a-z]+", t)
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
        return None
    end_raw = m.group("end")
    current = bool(_NOW_RE.match(end_raw.strip()))
    start = to_iso(m.group("start"))
    end = None if current else to_iso(end_raw)
    if start and end and end < start:
        return None  # "2021 - 2019" is not a period of time
    return DateRange(m.group(0).strip(), start, end, current)

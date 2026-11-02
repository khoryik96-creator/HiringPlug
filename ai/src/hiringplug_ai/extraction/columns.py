"""Reading order for two-column pages.

CVs often have a sidebar (contacts, skills, languages) next to the main
column. PDF text comes out line by line across the whole page, so the two
columns interleave: "Phone  Senior Analyst, SAP FICO" and "+6017 877 1510
Mar 2023 - Present". Here a page is split at the gap between its columns,
found from word positions, and each column is read top to bottom.

A gap alone is not enough: right-aligned dates on a one-column CV leave a gap
too. A page is split only when almost no line crosses the gap and both sides
run down a large part of the page with a real share of its text. (The two
columns' lines rarely line up, since their fonts differ, so lines are not
compared side by side.) Lines that cross the gap (a full-width
name or heading) are kept in place, between column blocks.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any

BIN = 2.0  # points
MIN_GUTTER = 8.0  # points of empty space between the columns
MIN_ROWS = 8
MAX_CROSSING_SHARE = 0.1  # of rows with a word across the gap
MIN_SIDE_HEIGHT = 0.35  # of the text's height covered by each side
MIN_SIDE_CHARS = 0.05  # of the page's characters on each side
MIN_SIDE_WORDS = 15
MAX_DATE_SHARE = 0.5  # a side that is mostly dates is a date column of a one-column page

_DATE_WORD = re.compile(
    r"^(?:[\d/.\-\u2013\u2014()]+|jan\w*|feb\w*|mar\w*|apr\w*|may|jun\w*|jul\w*|aug\w*|sep\w*|oct\w*|nov\w*|"
    r"dec\w*|present|current|now|to|till|date)[,.]?$",
    re.IGNORECASE,
)


@dataclass
class _Row:
    top: float
    bottom: float
    words: list[dict[str, Any]]


def _rows(words: list[dict[str, Any]]) -> list[_Row]:
    rows: list[_Row] = []
    for w in sorted(words, key=lambda w: (round(w["top"]), w["x0"])):
        if rows and abs(rows[-1].top - w["top"]) <= 2:
            rows[-1].words.append(w)
            rows[-1].bottom = max(rows[-1].bottom, w["bottom"])
        else:
            rows.append(_Row(w["top"], w["bottom"], [w]))
    return rows


def find_gutter(words: list[dict[str, Any]], width: float) -> float | None:
    """The x position of the gap between two columns, or None for one column."""
    rows = _rows(words)
    if len(rows) < MIN_ROWS or width <= 0:
        return None
    total_chars = sum(len(w["text"]) for w in words) or 1
    best: tuple[float, float] | None = None  # (score, x)
    run_start: float | None = None
    x = width * 0.2
    while x <= width * 0.8:
        crossing = sum(1 for r in rows if any(w["x0"] < x < w["x1"] for w in r.words))
        if crossing / len(rows) <= MAX_CROSSING_SHARE:
            run_start = x if run_start is None else run_start
        else:
            best = _better(best, run_start, x, rows=rows, words=words, total_chars=total_chars)
            run_start = None
        x += BIN
    return (_better(best, run_start, x, rows=rows, words=words, total_chars=total_chars) or (0.0, None))[1]


def _better(
    best: tuple[float, float] | None,
    start: float | None,
    end: float,
    *,
    rows: list[_Row],
    words: list[dict[str, Any]],
    total_chars: int,
) -> tuple[float, float] | None:
    if start is None or end - start < MIN_GUTTER:
        return best
    mid = (start + end) / 2
    left = sum(len(w["text"]) for w in words if w["x1"] <= mid)
    right = sum(len(w["text"]) for w in words if w["x0"] >= mid)
    if min(left, right) / total_chars < MIN_SIDE_CHARS:
        return best
    for side in ([w for w in words if w["x1"] <= mid], [w for w in words if w["x0"] >= mid]):
        if side and sum(1 for w in side if _DATE_WORD.match(w["text"])) / len(side) > MAX_DATE_SHARE:
            return best
    left_rows = [r for r in rows if any(w["x1"] <= mid for w in r.words)]
    right_rows = [r for r in rows if any(w["x0"] >= mid for w in r.words)]
    if min(sum(1 for w in words if w["x1"] <= mid), sum(1 for w in words if w["x0"] >= mid)) < MIN_SIDE_WORDS:
        return best
    height = max(r.bottom for r in rows) - min(r.top for r in rows) or 1.0
    if min(_covered(left_rows), _covered(right_rows)) / height < MIN_SIDE_HEIGHT:
        return best
    score = end - start
    return (score, mid) if best is None or score > best[0] else best


def _covered(rows: list[_Row]) -> float:
    """How much of the page's height these rows run down (from the first to the last)."""
    return max(r.bottom for r in rows) - min(r.top for r in rows) if rows else 0.0


def page_text(page: Any) -> str:
    """The page's text, column by column when it has two columns."""
    words = page.extract_words()
    gutter = find_gutter(words, float(page.width))
    if gutter is None:
        return str(page.extract_text() or "")
    parts: list[str] = []
    block_top: float | None = None
    block_bottom = 0.0

    def flush() -> None:
        if block_top is None:
            return
        top, bottom = max(0.0, block_top - 1), min(float(page.height), block_bottom + 1)
        sides = [
            page.within_bbox((x0, top, x1, bottom)) for x0, x1 in ((0.0, gutter), (gutter, float(page.width)))
        ]
        # The column that starts higher first (it usually holds the name), else the wider one.
        sides.sort(
            key=lambda c: (round(min((w["top"] for w in c.extract_words()), default=bottom) / 6), -c.width)
        )
        for side in sides:
            text = side.extract_text()
            if text and text.strip():
                parts.append(text)

    for row in _rows(words):
        if any(w["x0"] < gutter < w["x1"] for w in row.words):  # a full-width line
            flush()
            block_top = None
            parts.append(" ".join(w["text"] for w in sorted(row.words, key=lambda w: w["x0"])))
            continue
        block_top = row.top if block_top is None else block_top
        block_bottom = row.bottom
    flush()
    return "\n".join(parts)

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
from itertools import pairwise
from typing import Any

from .tables import work_tables

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


def find_gutter(words: list[dict[str, Any]], width: float, rows: list[_Row] | None = None) -> float | None:
    """The x position of the gap between two columns, or None for one column."""
    rows = _rows(words) if rows is None else rows
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


def _letter_spaced(rows: list[_Row]) -> list[tuple[str, str]]:
    """Letter-spaced lines ("N O E L L E  K H O O"), read back as words: the
    text has one space between every letter, but the words' gap is wider than
    the letters'. Pairs of (as the text has it, as words)."""
    out: list[tuple[str, str]] = []
    for row in rows:
        run: list[dict[str, Any]] = []
        for w in [*sorted(row.words, key=lambda w: w["x0"]), None]:
            if w is not None and len(w["text"]) == 1 and w["text"].isalpha():
                run.append(w)
                continue
            if len(run) >= 4:
                gaps = [b["x0"] - a["x1"] for a, b in pairwise(run)]
                usual = sorted(gaps)[len(gaps) // 2]
                joined = run[0]["text"]
                for gap, letter in zip(gaps, run[1:], strict=False):
                    joined += (" " if gap > max(2 * usual, usual + 3) else "") + letter["text"]
                if " " in joined or len(joined) >= 4:
                    out.append((" ".join(x["text"] for x in run), joined))
            run = []
    return out


def _respaced(text: str, rows: list[_Row]) -> str:
    for spaced, joined in _letter_spaced(rows):
        text = re.sub(rf"(?<!\S){re.escape(spaced)}(?!\S)", joined, text, count=1)
    return text


def page_text(page: Any) -> str:
    """The page's text, column by column when it has two columns; a
    work-history table read row by row (tables.py) in its place: on a
    two-column page, within its column (a sidebar beside it is read as the
    other column, never inside the work history); else between the text
    above and below it, with text beside it before or after it."""
    tables = work_tables(page)
    if not tables:
        return _region_text(page)
    x0, top, x1, bottom = (float(v) for v in page.bbox)

    def in_table(w: dict[str, Any]) -> bool:
        return any(
            t.left - 1 <= w["x0"]
            and w["x1"] <= t.right + 1
            and t.top - 1 <= w["top"]
            and w["bottom"] <= t.bottom + 1
            for t in tables
        )

    # The columns are found on the whole page (without the tables): the strips
    # between tables are often too short to tell on their own.
    outside = [w for w in page.extract_words() if not in_table(w)]
    gutter = find_gutter(outside, x1, _rows(outside))
    if gutter is not None and all(t.left >= gutter - 1 or t.right <= gutter + 1 for t in tables):
        return _columns_with_tables(page, tables, gutter, outside)
    parts: list[str] = []
    for t in tables:
        if t.top - top > 1:
            parts.append(_region_text(_strip(page, (x0, top, x1, t.top)), gutter))
        beside = [
            _column_text(_strip(page, (a, max(t.top, top), b, t.bottom)))
            for a, b in ((x0, t.left), (t.right, x1))
        ]
        parts += [beside[0], t.text, beside[1]]  # text on its left first, on its right after
        top = max(top, t.bottom)
    if bottom - top > 1:
        parts.append(_region_text(_strip(page, (x0, top, x1, bottom)), gutter))
    return "\n".join(p for p in parts if p.strip())


def _columns_with_tables(page: Any, tables: list[Any], gutter: float, outside: list[dict[str, Any]]) -> str:
    """A two-column page whose work tables sit in one column: the lines across
    both columns above them (the name) first, then each column top to bottom,
    a table's rows in its place in its column."""
    x0, top, x1, bottom = (float(v) for v in page.bbox)
    first = min(t.top for t in tables)
    across = [
        r.bottom
        for r in _rows(outside)
        if r.bottom <= first and any(w["x0"] < gutter < w["x1"] for w in r.words)
    ]
    head = max(across, default=top)
    parts = [_region_text(_strip(page, (x0, top, x1, head)), gutter)] if head - top > 1 else []
    for a, b in ((x0, gutter), (gutter, x1)):
        y = head
        for t in [t for t in tables if a - 1 <= t.left and t.right <= b + 1]:
            parts += [_column_text(_strip(page, (a, y, b, t.top))), t.text]
            y = max(y, t.bottom)
        parts.append(_column_text(_strip(page, (a, y, b, bottom))))
    return "\n".join(p for p in parts if p.strip())


def _strip(page: Any, bbox: tuple[float, float, float, float]) -> Any:
    """A part of the page holding the characters whose middle is in it: a
    character on a border goes to one part only, never to two."""
    px0, ptop, px1, pbottom = (float(v) for v in page.bbox)
    a, t, b, u = max(bbox[0], px0), max(bbox[1], ptop), min(bbox[2], px1), min(bbox[3], pbottom)
    if b - a <= 1 or u - t <= 1:
        return None

    def inside(obj: dict[str, Any]) -> bool:
        if obj.get("object_type") != "char":
            return True
        cx, cy = (obj["x0"] + obj["x1"]) / 2, (obj["top"] + obj["bottom"]) / 2
        return bool(a <= cx < b and t <= cy < u)

    # Filtered before the crop, which would clip a character to the border.
    return page.filter(inside).crop((a, t, b, u), strict=False)


def _column_text(page: Any) -> str:
    """One column's text, as it stands (it is not split again)."""
    if page is None:
        return ""
    words = page.extract_words()
    return _respaced(str(page.extract_text() or ""), _rows(words))


def _region_text(page: Any, gutter: float | None = None) -> str:
    if page is None:
        return ""
    words = page.extract_words()
    rows = _rows(words)
    return _respaced(_page_text(page, words, rows, gutter), rows)


def _page_text(page: Any, words: list[dict[str, Any]], rows: list[_Row], gutter: float | None = None) -> str:
    # A gutter found on the whole page holds for a strip of it that crosses it.
    if gutter is None or not (float(page.bbox[0]) < gutter < float(page.bbox[2])) or not words:
        gutter = find_gutter(words, float(page.bbox[2]), rows)
    if gutter is None:
        return str(page.extract_text() or "")
    parts: list[str] = []
    block_top: float | None = None
    block_bottom = 0.0

    def flush() -> None:
        if block_top is None:
            return
        top, bottom = max(float(page.bbox[1]), block_top - 1), min(float(page.bbox[3]), block_bottom + 1)
        bounds = [(float(page.bbox[0]), gutter), (gutter, float(page.bbox[2]))]

        def first_top(side: tuple[float, float]) -> float:
            # From the page's words, already extracted: no second pass over the column.
            tops = [
                w["top"]
                for w in words
                if top <= w["top"] <= bottom and side[0] <= w["x0"] and w["x1"] <= side[1]
            ]
            return min(tops, default=bottom)

        # The column that starts higher first (it usually holds the name), else the wider one.
        bounds.sort(key=lambda side: (round(first_top(side) / 6), -(side[1] - side[0])))
        for x0, x1 in bounds:
            text = page.within_bbox((x0, top, x1, bottom)).extract_text()
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

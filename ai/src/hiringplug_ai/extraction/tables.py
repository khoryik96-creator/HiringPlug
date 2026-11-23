"""Work-history tables in PDFs.

Some CVs list their jobs as a table: Organization | Role | Location | Client |
Industry | Duration, each cell wrapped over a few lines. Read line by line,
the cells of a row interleave ("GSD KL - IT Shared Business Functional
Analyst - SAP Malaysia In-House Projects FMCG"), so no title or employer can
be told apart. Here a table whose header names a role and its dates (or its
employer) is read cell by cell, and each row is written as labelled lines,
the dates first: "Duration: 11/2019 to 03/2026", "Role: ...", "Organization: ...".
Every other table is left to the usual reading.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any

_ROLE = re.compile(r"^(?:role|position|designation|job\s*title|title|jawatan)$", re.I)
_ORG = re.compile(r"^(?:organi[sz]ation|company(?:\s+name)?|employer|syarikat)$", re.I)
_DATES = re.compile(r"^(?:duration|period|dates?|tenure|from\s*[-\u2013/]\s*to|year\(?s\)?)$", re.I)
# A cheap look at the page's words before asking pdfplumber for tables.
_HINT = re.compile(
    r"^(?:role|position|designation|title|jawatan|duration|period|dates?|tenure|years?|"
    r"organi[sz]ation|company|employer|syarikat)$",
    re.I,
)


@dataclass(frozen=True)
class WorkTable:
    left: float
    top: float
    right: float
    bottom: float
    text: str


def _plain(text: Any) -> str:
    return " ".join(str(text or "").split())


def _label(header: str) -> str:
    """The header as a label the work-history reader knows."""
    if _ROLE.match(header):
        return "Role"
    if _ORG.match(header):
        return "Organization"
    if _DATES.match(header):
        return "Duration"
    return header


def _overlap(a: tuple[float, float], b: tuple[float, float]) -> float:
    return max(0.0, min(a[1], b[1]) - max(a[0], b[0]))


def rows_as_lines(
    header: list[tuple[tuple[float, float], str]], rows: list[list[tuple[tuple[float, float], str]]]
) -> str:
    """Each row as labelled lines, the dates first; a cell goes under the
    header it overlaps most. Pure: unit tested."""
    out: list[str] = []
    for row in rows:
        values: dict[str, list[str]] = {}
        for span, text in row:
            if not text:
                continue
            best = max(header, key=lambda h: _overlap(span, h[0]))
            if _overlap(span, best[0]) <= 0:
                continue
            values.setdefault(_label(best[1]), []).append(text)
        if not values:
            continue
        order = ["Duration", "Role", "Organization"]
        labels = [k for k in order if k in values] + [k for k in values if k not in order]
        out.append("\n".join(f"{k}: {' '.join(values[k])}" for k in labels))
    return "\n\n".join(out)


def _is_work_header(cells: list[str]) -> bool:
    return any(_ROLE.match(c) for c in cells) and (
        any(_DATES.match(c) for c in cells) or any(_ORG.match(c) for c in cells)
    )


def work_tables(page: Any) -> list[WorkTable]:
    """The page's work-history tables, top to bottom, as labelled text."""
    if not any(_HINT.match(str(w.get("text", ""))) for w in page.extract_words()):
        return []
    found: list[WorkTable] = []
    for table in page.find_tables():
        texts = table.extract()
        if len(texts) < 2 or len(table.rows) != len(texts):
            continue
        cells = [_plain(c) for c in texts[0]]
        if not _is_work_header([c for c in cells if c]):
            continue
        header = [
            ((box[0], box[2]), text)
            for box, text in zip(table.rows[0].cells, cells, strict=False)
            if box is not None and text
        ]
        rows = [
            [
                ((box[0], box[2]), _plain(text))
                for box, text in zip(r.cells, t, strict=False)
                if box is not None
            ]
            for r, t in zip(table.rows[1:], texts[1:], strict=False)
        ]
        text = rows_as_lines(header, rows)
        if text:
            x0, top, x1, bottom = (float(v) for v in table.bbox)
            found.append(WorkTable(left=x0, top=top, right=x1, bottom=bottom, text=text))
    return sorted(found, key=lambda t: t.top)

"""A work-history table written row by row as text, as a Word table is read:
"Role | Organization | Client | Duration" over "Analyst | Contoh Berhad |
Sampel Sdn Bhd | 2019 - 2021". Each row is read by its header's columns, so
a client with a company's legal name is never taken for the employer."""

from __future__ import annotations

from .vocab import COLUMN_ORG_RE, COLUMN_ROLE_RE, TABLE_HEADER_RE

ROWS_LOOKED_AT = 40  # a table's rows above a row, at most, to find its header


def header_above(lines: list[str], i: int) -> str | None:
    """The header row of the table whose rows run up to line i, or None when
    line i is not in such a table."""
    for j in range(i - 1, max(-1, i - ROWS_LOOKED_AT), -1):
        text = lines[j].strip()
        if "|" in text and TABLE_HEADER_RE.match(text):
            return text
        if text and "|" not in text:
            return None
    return None


def by_columns(header: str | None, row: str) -> tuple[str, str] | None:
    """A row's (title, employer) cells, as written, by its header's columns;
    None when the row does not have the header's cells (its dates' cell left
    empty, not dropped) or names neither."""
    if header is None or "|" not in row:
        return None
    names = [c.strip() for c in header.strip(" |").split("|")]
    cells = [c.strip() for c in row.split("|")]
    if len(cells) != len(names):
        return None
    title = next((c for n, c in zip(names, cells, strict=True) if COLUMN_ROLE_RE.match(n)), "")
    company = next((c for n, c in zip(names, cells, strict=True) if COLUMN_ORG_RE.match(n)), "")
    return (title, company) if title or company else None

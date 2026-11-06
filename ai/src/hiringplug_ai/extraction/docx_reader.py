"""Word (.docx) text extraction that keeps a CV's structure.

Mechanism (from CV Studio):
- Headers come first: a candidate's name and contacts are often in the page header.
- List paragraphs keep a "• " marker, so bullet boundaries survive (Word stores
  numbering separately from the text, including numbering inherited from styles).
- Tables: a row of single-paragraph cells becomes "a | b | c"; cells holding
  several paragraphs (e.g. a list of duties) keep one line per paragraph.
- Layout tables: CV templates put the name, skills, employers and duties in
  tables inside table cells. A cell is read in document order (its paragraphs
  and nested tables as they come), a merged cell once, and text boxes anchored
  in a cell (often the dates beside an employer) stay beside it.
- Content controls, custom XML and tracked insertions/moves wrap paragraphs
  and tables without hiding them: they are read through (deleted text is not).
- Text boxes (summary panels, sidebars) are read in place; Word stores each one
  twice (a modern copy and a fallback), so exact duplicates are dropped.
- A layout nested more than 32 levels deep fails with a clear message rather
  than being read partly.
"""

from __future__ import annotations

import io
from importlib.metadata import version
from typing import Any

from .limits import Limits
from .result import Extraction
from .text import clean_text
from .zipsafety import validate_zip

_W = "{http://schemas.openxmlformats.org/wordprocessingml/2006/main}"
_BULLETS = (
    "\u2022",
    "\u25cf",
    "\u25aa",
    "\u25e6",
    "\u2023",
    "\u2043",
    "\u2219",
    "\u00b7",
    "-",
    "\u2013",
)  # bullet glyphs, hyphen and en dash


def extract_docx(data: bytes, limits: Limits) -> Extraction:
    validate_zip(data, limits, "Word document")
    import docx  # noqa: PLC0415
    from docx.table import Table  # noqa: PLC0415
    from docx.text.paragraph import Paragraph  # noqa: PLC0415

    document = docx.Document(io.BytesIO(data))
    reader = _Reader(Paragraph, Table)
    lines: list[str] = []
    try:
        seen_headers: set[str] = set()
        for section in document.sections:
            for header in (section.header, section.first_page_header, section.even_page_header):
                for line in reader.content_lines(header._element, header, 0):
                    if line not in seen_headers:
                        seen_headers.add(line)
                        lines.append(line)
        lines.extend(reader.content_lines(document.element.body, document, 0))
    except _TooDeepError:
        message = (
            "The Word document's layout is nested too deeply to read: save it as a simpler Word file or PDF."
        )
        return Extraction("failed", None, None, f"python-docx@{version('python-docx')}", error=message)

    text = clean_text("\n".join(lines))
    extractor = f"python-docx@{version('python-docx')}"
    if not text:
        return Extraction("failed", None, None, extractor, error="The Word document has no readable text.")
    return Extraction("extracted", text, None, extractor)


# Wrappers read through; deleted text ("del", "moveFrom") is not.
_CONTAINERS = {"sdt", "sdtContent", "customXml", "ins", "moveTo"}
_MAX_DEPTH = 32


class _TooDeepError(ValueError):
    pass


def _tag(element: Any) -> str:
    return str(element.tag).rsplit("}", 1)[-1]


def _blocks(element: Any, level: int) -> Any:
    """The paragraphs and tables in element, in order, through any wrapping containers."""
    if level > _MAX_DEPTH:
        raise _TooDeepError
    for child in element.iterchildren():
        if _tag(child) in _CONTAINERS:
            yield from _blocks(child, level + 1)
        else:
            yield child, level


class _Reader:
    """Reads bodies, headers and table cells in document order.

    python-docx's Paragraph and Table are passed in: the library is imported lazily.
    """

    def __init__(self, paragraph: Any, table: Any) -> None:
        self.paragraph, self.table = paragraph, table
        self.seen_boxes: set[tuple[str, ...]] = set()

    def content_lines(self, element: Any, parent: Any, depth: int) -> list[str]:
        out: list[str] = []
        for child, level in _blocks(element, depth):
            tag = _tag(child)
            if tag == "p":
                line = _paragraph_line(self.paragraph(child, parent))
                if line:
                    out.append(line)
                out.extend(_textbox_lines(child, self.seen_boxes))
            elif tag == "tbl":
                for row in self.table(child, parent).rows:
                    out.extend(self.row_lines(row, level + 1))
        return out

    def row_lines(self, row: Any, depth: int) -> list[str]:
        if depth > _MAX_DEPTH:
            raise _TooDeepError
        groups: list[list[str]] = []
        seen_cells: set[Any] = set()
        for cell in row.cells:  # a merged cell is listed once per column it spans: read it once
            if cell._tc in seen_cells:
                continue
            seen_cells.add(cell._tc)
            group: list[str] = []
            for line in self.content_lines(cell._tc, cell, depth):
                if not group or group[-1] != line:
                    group.append(line)
            if group:
                groups.append(group)
        if not groups:
            return []
        if all(len(g) == 1 for g in groups):
            return [" | ".join(g[0] for g in groups)]
        return [line for g in groups for line in g]


def _paragraph_line(paragraph: Any) -> str:
    text = " ".join(paragraph.text.split())
    if text and _numbering_id(paragraph) is not None and not text.startswith(_BULLETS):
        return "• " + text
    return text


def _numbering_id(paragraph: Any) -> str | None:
    """The list a paragraph belongs to, directly or through its style chain."""
    direct = paragraph._p.xpath("./w:pPr/w:numPr/w:numId/@w:val")
    if direct:
        return next((str(v) for v in direct if str(v) != "0"), None)
    style, seen = paragraph.style, set()
    while style is not None and style.style_id not in seen:
        seen.add(style.style_id)
        inherited = style.element.xpath("./w:pPr/w:numPr/w:numId/@w:val")
        if inherited:
            return next((str(v) for v in inherited if str(v) != "0"), None)
        style = style.base_style
    return None


def _textbox_lines(paragraph_element: Any, seen: set[tuple[str, ...]]) -> list[str]:
    out: list[str] = []
    for box in paragraph_element.iter(f"{_W}txbxContent"):
        group: list[str] = []
        for p in box.iter(f"{_W}p"):
            text = " ".join("".join(t.text or "" for t in p.iter(f"{_W}t")).split())
            if not text:
                continue
            numbered = p.find(f"{_W}pPr/{_W}numPr") is not None
            group.append("• " + text if numbered and not text.startswith(_BULLETS) else text)
        if group and tuple(group) not in seen:
            seen.add(tuple(group))
            out.extend(group)
    return out

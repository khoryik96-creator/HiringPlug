"""Word (.docx) text extraction that keeps a CV's structure.

Mechanism (from CV Studio):
- Headers come first: a candidate's name and contacts are often in the page header.
- List paragraphs keep a "• " marker, so bullet boundaries survive (Word stores
  numbering separately from the text, including numbering inherited from styles).
- Tables: a row of single-paragraph cells becomes "a | b | c"; cells holding
  several paragraphs (e.g. a list of duties) keep one line per paragraph.
- Text boxes (summary panels, sidebars) are read in place; Word stores each one
  twice (a modern copy and a fallback), so exact duplicates are dropped.
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
    lines: list[str] = []
    seen_headers: set[str] = set()
    for section in document.sections:
        for header in (section.header, section.first_page_header, section.even_page_header):
            for line in _container_lines(header):
                if line not in seen_headers:
                    seen_headers.add(line)
                    lines.append(line)

    seen_boxes: set[tuple[str, ...]] = set()
    for child in document.element.body.iterchildren():
        tag = child.tag.rsplit("}", 1)[-1]
        if tag == "p":
            line = _paragraph_line(Paragraph(child, document))
            if line:
                lines.append(line)
            lines.extend(_textbox_lines(child, seen_boxes))
        elif tag == "tbl":
            for row in Table(child, document).rows:
                lines.extend(_row_lines(row))

    text = clean_text("\n".join(lines))
    extractor = f"python-docx@{version('python-docx')}"
    if not text:
        return Extraction("failed", None, None, extractor, error="The Word document has no readable text.")
    return Extraction("extracted", text, None, extractor)


def _container_lines(container: Any) -> list[str]:
    out = [line for p in container.paragraphs if (line := _paragraph_line(p))]
    for table in container.tables:
        for row in table.rows:
            out.extend(_row_lines(row))
    return out


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


def _row_lines(row: Any, depth: int = 0) -> list[str]:
    groups: list[list[str]] = []
    seen: set[tuple[str, ...]] = set()
    for cell in row.cells:  # merged cells repeat; keep the first copy
        group: list[str] = []
        for paragraph in cell.paragraphs:
            line = _paragraph_line(paragraph)
            if line and (not group or group[-1] != line):
                group.append(line)
        # Tables inside a cell (layout tables, and old .doc files converted by
        # LibreOffice) hold contact details often enough to matter.
        if depth < 3:
            for table in cell.tables:
                for inner in table.rows:
                    group.extend(_row_lines(inner, depth + 1))
        if group and tuple(group) not in seen:
            seen.add(tuple(group))
            groups.append(group)
    if not groups:
        return []
    if all(len(g) == 1 for g in groups):
        return [" | ".join(g[0] for g in groups)]
    return [line for g in groups for line in g]


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

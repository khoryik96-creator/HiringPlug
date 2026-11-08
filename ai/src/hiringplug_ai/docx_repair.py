"""Make a Word file valid by the Office Open XML schema, so Word opens it
without "Word found unreadable content".

CV Studio's renderer (vendored unchanged) writes some paragraph and run
settings in an order the schema forbids (spacing after ind, keepNext after
jc...), table cell borders out of order, a row height with `w:type` for
`w:hRule`; and the privacy-cleaned template keeps empty document properties
(<Pages></Pages>), which Word refuses. Word is strict where LibreOffice is
lenient, so the files looked fine everywhere but in Word. Checked with
Microsoft's Open XML SDK validator: 61 errors before, none after.

Only order and those values change: never any text, so nothing a person
wrote or a check approved is altered.
"""

from __future__ import annotations

import io
import zipfile

from lxml import etree

W = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
_W = "{" + W + "}"

# The schema's order of a paragraph's settings (CT_PPr) and a run's (CT_RPr).
PPR_ORDER = [
    "pStyle", "keepNext", "keepLines", "pageBreakBefore", "framePr", "widowControl", "numPr",
    "suppressLineNumbers", "pBdr", "shd", "tabs", "suppressAutoHyphens", "kinsoku", "wordWrap",
    "overflowPunct", "topLinePunct", "autoSpaceDE", "autoSpaceDN", "bidi", "adjustRightInd", "snapToGrid",
    "spacing", "ind", "contextualSpacing", "mirrorIndents", "suppressOverlap", "jc", "textDirection",
    "textAlignment", "textboxTightWrap", "outlineLvl", "divId", "cnfStyle", "rPr", "sectPr", "pPrChange",
]  # fmt: skip
RPR_ORDER = [
    "rStyle", "rFonts", "b", "bCs", "i", "iCs", "caps", "smallCaps", "strike", "dstrike", "outline", "shadow",
    "emboss", "imprint", "noProof", "snapToGrid", "vanish", "webHidden", "color", "spacing", "w", "kern",
    "position", "sz", "szCs", "highlight", "u", "effect", "bdr", "shd", "fitText", "vertAlign", "rtl", "cs",
    "em", "lang", "eastAsianLayout", "specVanish", "oMath", "rPrChange",
]  # fmt: skip
BORDER_ORDER = ["top", "start", "left", "bottom", "end", "right", "insideH", "insideV", "tl2br", "tr2bl"]
PBDR_ORDER = ["top", "left", "bottom", "right", "between", "bar"]
_ORDERS = {"pPr": PPR_ORDER, "rPr": RPR_ORDER, "tcBorders": BORDER_ORDER, "tblBorders": BORDER_ORDER,
           "pBdr": PBDR_ORDER}  # fmt: skip

_WORD_PARTS = ("word/document.xml", "word/header", "word/footer", "word/footnotes.xml", "word/endnotes.xml")


def _reorder(parent: etree._Element, order: list[str]) -> None:
    rank = {name: i for i, name in enumerate(order)}
    children = list(parent)

    def key(pair: tuple[int, etree._Element]) -> tuple[int, int]:
        index, child = pair
        tag = child.tag if isinstance(child.tag, str) else ""
        local = tag[len(_W) :] if tag.startswith(_W) else None
        return (
            rank.get(local, len(order)) if local is not None else len(order),
            index,
        )  # others keep their place last

    ordered = [child for _, child in sorted(enumerate(children), key=key)]
    if ordered != children:
        for child in children:
            parent.remove(child)
        parent.extend(ordered)


def _repair_word_part(data: bytes) -> bytes:
    root = etree.fromstring(data, etree.XMLParser(remove_blank_text=False, huge_tree=True))
    for local, order in _ORDERS.items():
        for element in root.iter(_W + local):
            _reorder(element, order)
    for height in root.iter(_W + "trHeight"):  # w:type is not an attribute here; w:hRule is
        kind = height.attrib.pop(_W + "type", None)
        if kind and _W + "hRule" not in height.attrib and kind in ("exact", "atLeast", "auto"):
            height.set(_W + "hRule", kind)
    return bytes(etree.tostring(root, xml_declaration=True, encoding="UTF-8", standalone=True))


def _repair_properties(data: bytes) -> bytes:
    """Document properties with no value are left out (Word refuses empty ones)."""
    root = etree.fromstring(data)
    for child in list(root):
        if len(child) == 0 and not (child.text or "").strip():
            root.remove(child)
    return bytes(etree.tostring(root, xml_declaration=True, encoding="UTF-8", standalone=True))


def repair(docx: bytes) -> bytes:
    """The same Word file, valid by the schema (entries, order and compression kept)."""
    out = io.BytesIO()
    with zipfile.ZipFile(io.BytesIO(docx)) as source, zipfile.ZipFile(out, "w") as target:
        for info in source.infolist():
            data = source.read(info)
            name = info.filename
            if name.endswith(".xml") and name.startswith(_WORD_PARTS):
                data = _repair_word_part(data)
            elif name in ("docProps/app.xml", "docProps/core.xml"):
                data = _repair_properties(data)
            target.writestr(info, data)
    return out.getvalue()

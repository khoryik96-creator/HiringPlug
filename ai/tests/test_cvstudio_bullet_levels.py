"""A formatted CV's bullets nest as the original shows them: Word's list level
number is not how deep a bullet sits when a CV sets its indent by hand."""

from __future__ import annotations

import io

import docx
from docx.oxml import OxmlElement
from docx.oxml.ns import qn

from hiringplug_ai import cvstudio  # noqa: F401 - puts the engine's folder on the path

import cvstudio_docx_extract as engine  # type: ignore[import-not-found]  # isort: skip


def _bullet(d: docx.document.Document, text: str, level: int, left_twips: int | None) -> None:
    p = d.add_paragraph(text, style="List Bullet")
    ppr = p._p.get_or_add_pPr()
    num = ppr.find(qn("w:numPr"))
    if num is None:
        num = OxmlElement("w:numPr")
        ppr.append(num)
        num_id = OxmlElement("w:numId")
        num_id.set(qn("w:val"), "1")
        num.append(num_id)
    ilvl = num.find(qn("w:ilvl"))
    if ilvl is None:
        ilvl = OxmlElement("w:ilvl")
        num.insert(0, ilvl)
    ilvl.set(qn("w:val"), str(level))
    if left_twips is not None:
        ind = OxmlElement("w:ind")
        ind.set(qn("w:left"), str(left_twips))
        ppr.append(ind)


def test_bullets_nest_by_how_deep_they_sit_not_by_their_list_level() -> None:
    d = docx.Document()
    d.add_paragraph("Achievements")
    # Level 2 and 3 of a list drawn shallow by hand, their sub-points deeper.
    _bullet(d, "Completed two end to end implementations", 2, 709)
    _bullet(d, "Contoh Telco for Accounts Payable", 0, 993)
    _bullet(d, "Contoh Berhad for Asset Accounting", 0, 993)
    _bullet(d, "Completed four rollouts of the FICO module", 2, 709)
    d.add_paragraph("Support Role")
    _bullet(d, "Support team member for SAP FICO", 3, 709)
    _bullet(d, "Create and maintain cost centres", 0, 709)
    buf = io.BytesIO()
    d.save(buf)
    levels = {e["text"]: e["level"] for e in engine._extract_docx_bullet_levels(buf.getvalue())}
    assert levels == {
        "contoh telco for accounts payable": 1,
        "contoh berhad for asset accounting": 1,
    }, "the sub-points one level in; the rest (levels 2 and 3 drawn shallow) at the first level"

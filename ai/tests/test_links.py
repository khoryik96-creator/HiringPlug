from __future__ import annotations

import io

from docx import Document
from docx.opc.constants import RELATIONSHIP_TYPE
from reportlab.lib.pagesizes import A4
from reportlab.pdfgen import canvas

from hiringplug_ai.extraction import extract
from hiringplug_ai.extraction.links import clean, with_links
from hiringplug_ai.parsing import parse_cv


def linked_pdf() -> bytes:
    """A CV where "LinkedIn" is only a word with the address behind it."""
    out = io.BytesIO()
    c = canvas.Canvas(out, pagesize=A4)
    c.setFont("Helvetica", 12)
    for i, line in enumerate(["SITI AISYAH", "siti@example.com | LinkedIn | Portfolio", "Senior Accountant"]):
        c.drawString(60, 780 - i * 20, line)
    c.linkURL("https://my.linkedin.com/in/siti-aisyah-123", (230, 755, 290, 770), relative=0)
    c.linkURL("https://siti.example.com", (300, 755, 360, 770), relative=0)
    c.showPage()
    c.save()
    return out.getvalue()


def linked_docx() -> bytes:
    doc = Document()
    doc.add_paragraph("SITI AISYAH")
    p = doc.add_paragraph("Profile: ")
    rel = p.part.relate_to(
        "https://www.linkedin.com/in/siti-aisyah-123/", RELATIONSHIP_TYPE.HYPERLINK, is_external=True
    )
    link = p._p.makeelement(
        "{http://schemas.openxmlformats.org/wordprocessingml/2006/main}hyperlink",
        {
            "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id": rel,
        },
    )
    run = p.add_run("LinkedIn")._r
    link.append(run)
    p._p.append(link)
    out = io.BytesIO()
    doc.save(out)
    return out.getvalue()


def test_a_pdfs_hidden_linkedin_link_is_read_kano_style() -> None:
    result = extract(linked_pdf(), ocr=None)
    assert result.text is not None
    assert result.text.endswith(
        "Links in the file:\nhttps://my.linkedin.com/in/siti-aisyah-123\nhttps://siti.example.com"
    )
    profile = parse_cv(result.text)
    assert profile.linkedinUrl is not None
    assert profile.linkedinUrl.value == "https://my.linkedin.com/in/siti-aisyah-123"


def test_a_word_files_hidden_linkedin_link_is_read() -> None:
    result = extract(linked_docx(), ocr=None)
    assert result.text is not None
    assert "Links in the file:\nhttps://www.linkedin.com/in/siti-aisyah-123/" in result.text
    profile = parse_cv(result.text)
    assert profile.linkedinUrl is not None


def test_links_already_shown_and_format_addresses_are_left_out() -> None:
    assert (
        with_links("Find me at linkedin.com/in/siti", ["https://www.linkedin.com/in/siti"])
        == "Find me at linkedin.com/in/siti"
    )
    assert clean("http://schemas.openxmlformats.org/x") == ""
    assert clean("mailto:siti@example.com") == ""
    assert clean("lnkd.in/abc)") == "https://lnkd.in/abc"

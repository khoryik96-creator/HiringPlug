from __future__ import annotations

import io

from docx import Document
from docx.oxml import parse_xml

from hiringplug_ai.extraction import extract

W = 'xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"'
V = 'xmlns:v="urn:schemas-microsoft-com:vml"'


def build_cv() -> bytes:
    doc = Document()
    doc.sections[0].header.paragraphs[0].text = "Lim Wei Jie | +65 9123 4567"
    doc.add_paragraph("PROFESSIONAL EXPERIENCE")
    doc.add_paragraph("Led the month-end close", style="List Bullet")
    doc.add_paragraph("Implemented SAP S/4HANA", style="List Bullet")
    table = doc.add_table(rows=2, cols=2)
    table.cell(0, 0).text = "Company"
    table.cell(0, 1).text = "Period"
    table.cell(1, 0).text = "Acme Sdn Bhd"
    duties = table.cell(1, 1)
    duties.text = "2019 - 2024"
    duties.add_paragraph("Managed a team of 5")
    # A text box (summary panel). Word keeps a modern copy and a fallback copy.
    box = (
        f"<w:p {W} {V}><w:r><w:pict><v:shape><v:textbox><w:txbxContent>"
        "<w:p><w:r><w:t>ABOUT THE CANDIDATE</w:t></w:r></w:p>"
        "<w:p><w:r><w:t>Chartered accountant, 8 years</w:t></w:r></w:p>"
        "</w:txbxContent></v:textbox></v:shape></w:pict></w:r></w:p>"
    )
    doc.element.body.insert(0, parse_xml(box))
    doc.element.body.insert(1, parse_xml(box))  # the duplicate fallback copy
    out = io.BytesIO()
    doc.save(out)
    return out.getvalue()


def test_word_cv_keeps_header_bullets_tables_and_text_boxes() -> None:
    result = extract(build_cv(), ocr=None)
    assert result.status == "extracted"
    assert result.extractor.startswith("python-docx@")
    assert result.text is not None
    lines = result.text.split("\n")
    assert lines[0] == "Lim Wei Jie | +65 9123 4567", "page headers come first: names live there"
    assert lines.count("ABOUT THE CANDIDATE") == 1, "a text box's fallback copy is dropped"
    assert "Chartered accountant, 8 years" in lines
    assert "• Led the month-end close" in lines, "list paragraphs keep a bullet marker"
    assert "Company | Period" in lines, "a row of one-line cells stays one line"
    assert "2019 - 2024" in lines, "multi-paragraph cells keep one line each"
    assert "Managed a team of 5" in lines


def test_an_empty_word_document_fails_clearly() -> None:
    out = io.BytesIO()
    Document().save(out)
    result = extract(out.getvalue(), ocr=None)
    assert result.status == "failed"
    assert result.error == "The Word document has no readable text."


def test_tables_inside_table_cells_are_read() -> None:
    # Layout tables nest contact details in a cell (old .doc files converted by
    # LibreOffice look like this too).
    doc = Document()
    outer = doc.add_table(rows=1, cols=2)
    outer.cell(0, 0).paragraphs[0].text = "Nur Aisyah binti Rahman"
    inner = outer.cell(0, 1).add_table(rows=2, cols=2)
    inner.cell(0, 0).text, inner.cell(0, 1).text = "Email", "nur@example.com"
    inner.cell(1, 0).text, inner.cell(1, 1).text = "Mobile", "012-000 0001"
    buffer = io.BytesIO()
    doc.save(buffer)
    text = extract(buffer.getvalue(), ocr=None).text or ""
    assert "Email | nur@example.com" in text
    assert "Mobile | 012-000 0001" in text

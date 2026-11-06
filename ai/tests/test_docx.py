from __future__ import annotations

import io

import pytest
from docx import Document
from docx.oxml import OxmlElement, parse_xml

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


# Layout-table CVs (from CV Studio's nested-table fix): the name, skills,
# employers and duties sit in tables inside table cells, the dates in text
# boxes anchored beside their employer. All fictional.
def _layout_cv() -> bytes:
    doc = Document()
    outer = doc.add_table(rows=1, cols=2)
    outer.cell(0, 0).text = ""
    body = outer.cell(0, 1)
    body.add_table(rows=1, cols=1).cell(0, 0).text = "Example Candidate"
    body.add_paragraph("Hybrid Cloud Engineer with enterprise platform experience.")
    skills = body.add_table(rows=2, cols=1)
    skills.cell(0, 0).text = "SKILLS"
    skills.cell(1, 0).text = "Linux, VMware, AWS and automation"
    body.add_paragraph("WORK EXPERIENCE")
    job = body.add_table(rows=1, cols=2)
    date = job.cell(0, 0).paragraphs[0]
    for _ in range(2):  # Word's modern copy and its fallback copy
        box, p, r, t = (OxmlElement(x) for x in ("w:txbxContent", "w:p", "w:r", "w:t"))
        t.text = "Sep 2024 to Present"
        r.append(t)
        p.append(r)
        box.append(p)
        run, pict = OxmlElement("w:r"), OxmlElement("w:pict")
        pict.append(box)
        run.append(pict)
        date._p.append(run)
    details = job.cell(0, 1).add_table(rows=1, cols=1).cell(0, 0)
    details.text = "Example Cloud Services"
    details.add_paragraph("Hybrid Cloud Engineer")
    details.add_paragraph("Maintain production Linux clusters.", style="List Bullet")
    body.add_paragraph("EDUCATION")
    body.add_table(rows=1, cols=1).cell(0, 0).text = "Example University | Engineering degree"
    out = io.BytesIO()
    doc.save(out)
    return out.getvalue()


def _assert_layout_read(text: str) -> None:
    order = [
        "Example Candidate",
        "Hybrid Cloud Engineer with",
        "SKILLS",
        "Linux, VMware",
        "WORK EXPERIENCE",
        "Sep 2024 to Present",
        "Example Cloud Services",
        "Maintain production Linux clusters.",
        "EDUCATION",
        "Example University",
    ]
    positions = [text.index(s) for s in order]
    assert positions == sorted(positions), "read in document order: the date beside its employer"
    assert text.count("Sep 2024 to Present") == 1, "the text box's fallback copy is not read twice"
    assert "• Maintain production Linux clusters." in text


def test_layout_tables_are_read_in_order_with_their_date_boxes() -> None:
    result = extract(_layout_cv(), ocr=None)
    assert result.status == "extracted"
    assert result.text is not None
    _assert_layout_read(result.text)


@pytest.mark.parametrize("tag", ["sdt", "customXml", "ins", "moveTo"])
def test_content_controls_and_tracked_insertions_are_read_through(tag: str) -> None:
    doc = Document(io.BytesIO(_layout_cv()))
    date = doc.element.body.xpath(".//w:tc/w:p[w:r/w:pict/w:txbxContent]")[0]
    employer = doc.element.body.xpath(".//w:tbl")[4]
    for child in (date, employer):
        parent, wrapper = child.getparent(), OxmlElement("w:" + tag)
        holder = OxmlElement("w:sdtContent") if tag == "sdt" else wrapper
        if holder is not wrapper:
            wrapper.append(holder)
        parent.replace(child, wrapper)
        holder.append(child)
    out = io.BytesIO()
    doc.save(out)
    result = extract(out.getvalue(), ocr=None)
    assert result.text is not None
    _assert_layout_read(result.text)


def test_deleted_revisions_are_not_read() -> None:
    doc = Document()
    cell = doc.add_table(rows=1, cols=1).cell(0, 0)
    cell.text = "Kept text"
    gone = cell.add_paragraph("Deleted text")._p
    wrapper = OxmlElement("w:del")
    gone.getparent().replace(gone, wrapper)
    wrapper.append(gone)
    out = io.BytesIO()
    doc.save(out)
    assert extract(out.getvalue(), ocr=None).text == "Kept text"


def test_a_layout_nested_too_deeply_fails_clearly() -> None:
    doc = Document()
    cell = doc.add_table(rows=1, cols=1).cell(0, 0)
    cell.text = "Fictional CV text"
    child = cell.paragraphs[0]._p
    for _ in range(34):
        parent, wrapper, content = child.getparent(), OxmlElement("w:sdt"), OxmlElement("w:sdtContent")
        wrapper.append(content)
        parent.replace(child, wrapper)
        content.append(child)
        child = wrapper
    out = io.BytesIO()
    doc.save(out)
    result = extract(out.getvalue(), ocr=None)
    assert result.status == "failed"
    assert "nested too deeply" in (result.error or "")

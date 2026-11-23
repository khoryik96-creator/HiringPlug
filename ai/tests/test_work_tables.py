"""Work history laid out as a table (Organization | Role | ... | Duration),
each cell wrapped over two lines: every job keeps its own title, employer and
dates. All text is invented."""

from __future__ import annotations

import io

from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import getSampleStyleSheet
from reportlab.platypus import Paragraph, SimpleDocTemplate, Spacer, Table, TableStyle

from hiringplug_ai.extraction import extract
from hiringplug_ai.extraction.tables import rows_as_lines
from hiringplug_ai.parsing import parse_cv

ROWS = [
    (
        "Contoh Berhad GSD KL - IT Shared Services",
        "Regional IT Senior Business Functional Analyst - SAP FICO/FSCM",
        "Malaysia",
        "In-House Projects",
        "FMCG",
        "11/2019 to 03/2026",
    ),
    (
        "Sampel Pharma Pvt Ltd - Global Technology Centre",
        "Regional IT Operations Business Functional Analyst - SAP MM/SD",
        "India",
        "In-House projects",
        "Pharmaceutical",
        "06/2017 to 11/2019",
    ),
    (
        "Demo Engineering & Business Solutions",
        "SAP Functional Lead Consultant / SAP Project Lead - SD",
        "India",
        "Demo Commercial Vehicles",
        "Automotive",
        "11/2014 to 05/2017",
    ),
]


def table_cv() -> bytes:
    """A CV whose jobs are a ruled table under "IT Experience", after an
    "Experience Summary" of bullets and the education."""
    styles = getSampleStyleSheet()
    small = styles["BodyText"].clone("small", fontSize=7, leading=8)
    story = [
        Paragraph("NUR AINA EXAMPLE", styles["Title"]),
        Paragraph("aina@example.com", styles["BodyText"]),
        Paragraph("Experience Summary", styles["Heading2"]),
        Paragraph("• 16 years of SAP consulting across finance and logistics.", styles["BodyText"]),
        Paragraph("• Led S/4HANA conversions and regional rollouts.", styles["BodyText"]),
        Paragraph("Educational Qualification", styles["Heading2"]),
        Paragraph("• Bachelor of Engineering, Universiti Contoh (2001 to 2005)", styles["BodyText"]),
        Paragraph("IT Experience", styles["Heading2"]),
        Spacer(1, 6),
    ]
    head = ["Organization", "Role", "Location", "Client", "Industry", "Duration"]
    data = [head] + [[Paragraph(c, small) for c in row] for row in ROWS]
    table = Table(data, colWidths=[95, 110, 55, 75, 60, 60])
    table.setStyle(
        TableStyle([("GRID", (0, 0), (-1, -1), 0.5, "black"), ("VALIGN", (0, 0), (-1, -1), "TOP")])
    )
    story.append(table)
    buf = io.BytesIO()
    SimpleDocTemplate(buf, pagesize=A4).build(story)
    return buf.getvalue()


def test_each_row_is_read_as_its_own_job() -> None:
    text = extract(table_cv(), ocr=None).text or ""
    assert (
        "Duration: 11/2019 to 03/2026\nRole: Regional IT Senior Business Functional Analyst - SAP FICO/FSCM"
        in text
    )
    jobs = parse_cv(text).model_dump()["experience"]
    got = [(j["title"]["value"], j["company"]["value"], j["dates"]["value"]) for j in jobs]
    assert got == [(role, org, dates) for org, role, _, _, _, dates in ROWS]


def test_a_row_becomes_labelled_lines_dates_first() -> None:
    header = [((0.0, 10.0), "Organization"), ((10.0, 20.0), "Position"), ((20.0, 30.0), "Period")]
    rows = [
        [((0.0, 10.0), "Contoh Berhad"), ((10.0, 20.0), "Analyst"), ((20.0, 30.0), "2019 - 2021")],
        [((0.0, 10.0), "")],
    ]
    assert rows_as_lines(header, rows) == "Duration: 2019 - 2021\nRole: Analyst\nOrganization: Contoh Berhad"


def test_a_field_named_experience_heading_starts_the_work_history() -> None:
    text = (
        "NUR AINA EXAMPLE\n\nEducation\nBachelor of Engineering (2001 to 2005)\n\nSAP Experience\n"
        "Duration: 2019 - 2021\nRole: Analyst\nOrganization: Contoh Berhad\n"
    )
    jobs = parse_cv(text).model_dump()["experience"]
    assert [(j["title"]["value"], j["company"]["value"]) for j in jobs] == [("Analyst", "Contoh Berhad")]

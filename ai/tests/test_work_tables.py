"""Work history laid out as a table (Organization | Role | ... | Duration),
each cell wrapped over two lines: every job keeps its own title, employer and
dates. All text is invented."""

from __future__ import annotations

import io

from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import getSampleStyleSheet
from reportlab.pdfgen import canvas
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


def test_a_title_or_skill_ending_in_experience_is_not_a_heading() -> None:
    text = (
        "NUR AINA EXAMPLE\n\nWork Experience\nHead of Customer Experience\n"
        "Contoh Berhad\nJan 2019 - Dec 2021\n\n"
        "Skills\nUser Experience\n10+ Years Experience\n"
    )
    jobs = parse_cv(text).model_dump()["experience"]
    assert jobs[0]["title"]["value"] == "Head of Customer Experience"


def test_an_empty_cell_never_takes_the_row_above() -> None:
    text = (
        "NUR AINA EXAMPLE\n\nIT Experience\n"
        "Duration: 11/2019 to 03/2026\nRole: Analyst\nOrganization: Contoh Berhad\n\n"
        "Duration: 06/2017 to 11/2019\nOrganization: Sampel Holdings\n"
    )
    jobs = parse_cv(text).model_dump()["experience"]
    assert jobs[1]["company"]["value"] == "Sampel Holdings"
    assert jobs[1]["title"] is None, "not the first row's Analyst"


def sidebar_table_cv(head: list[str]) -> bytes:
    """A work table on the right, a skills sidebar on the left of it."""
    buf = io.BytesIO()
    c = canvas.Canvas(buf, pagesize=A4)
    c.setFont("Helvetica", 9)
    y = 800
    for line in ["NUR AINA EXAMPLE", "aina@example.com", "IT Experience"]:
        c.drawString(40, y, line)
        y -= 14
    for i, line in enumerate(["SKILLS", "SAP FICO", "Python", "LANGUAGES", "Malay", "English", "Mandarin"]):
        c.drawString(40, 740 - i * 14, line)
    xs = [180, 300, 450, 560]
    rows = [
        head,
        ["Contoh Berhad", "Finance Analyst", "2019 - 2021"],
        ["Sampel Holdings", "Accountant", "2016 - 2019"],
    ]
    for r, row in enumerate(rows):
        top = 750 - r * 30
        for k, cell in enumerate(row):
            c.drawString(xs[k] + 4, top - 18, cell)
        c.line(xs[0], top, xs[-1], top)
    c.line(xs[0], 750 - len(rows) * 30, xs[-1], 750 - len(rows) * 30)
    for x in xs:
        c.line(x, 750, x, 750 - len(rows) * 30)
    c.save()
    return buf.getvalue()


def test_text_beside_a_work_table_is_kept() -> None:
    text = extract(sidebar_table_cv(["Organization", "Role", "Duration"]), ocr=None).text or ""
    assert "Duration: 2019 - 2021\nRole: Finance Analyst\nOrganization: Contoh Berhad" in text
    for word in ("SAP FICO", "Python", "Mandarin"):
        assert word in text, "the sidebar beside the table"


def test_a_company_job_title_dates_table_is_read_as_a_table() -> None:
    text = extract(sidebar_table_cv(["Company", "Job Title", "Dates"]), ocr=None).text or ""
    assert "Duration: 2016 - 2019\nRole: Accountant\nOrganization: Sampel Holdings" in text


def test_a_heading_naming_any_field_of_work_starts_the_work_history() -> None:
    for heading in ("Relevant Work Experience", "Sales Experience", "HR Experience", "Oil & Gas Experience"):
        text = (
            f"NUR AINA EXAMPLE\n\nSkills\nNegotiation\nCRM\n\n{heading}\n"
            "Sales Manager\nContoh Berhad\nJan 2019 - Dec 2021\n"
        )
        jobs = parse_cv(text).model_dump()["experience"]
        assert [(j["title"]["value"], j["company"]["value"]) for j in jobs] == [
            ("Sales Manager", "Contoh Berhad")
        ], heading


def test_a_certificates_table_is_not_read_as_jobs() -> None:
    text = extract(sidebar_table_cv(["Title", "Issuer", "Date"]), ocr=None).text or ""
    assert "Role:" not in text
    assert "Duration:" not in text


def two_column_table_cv() -> bytes:
    """A sidebar (its heading above the table) on the left; on the right the
    work table under its heading, and more of the main column below it."""
    buf = io.BytesIO()
    c = canvas.Canvas(buf, pagesize=A4)
    c.setFont("Helvetica", 9)
    c.drawString(200, 810, "NUR AINA EXAMPLE    aina@example.com    +60 12 345 6789    Kuala Lumpur")
    side = [
        "SKILLS",
        "SAP FICO",
        "Python",
        "Power BI",
        "Treasury",
        "Budgeting",
        "Forecasting",
        "IFRS 9",
        "LANGUAGES",
        "Malay",
        "English",
        "Mandarin",
        "HOBBIES",
        "Running",
        "Reading",
        "Chess",
        "Cooking",
    ]
    for i, line in enumerate(side):
        c.drawString(40, 780 - i * 22, line)
    c.drawString(200, 780, "IT Experience")
    xs = [200, 320, 450, 560]
    rows = [
        ["Organization", "Role", "Duration"],
        ["Contoh Berhad", "Finance Analyst", "2019 - 2021"],
        ["Sampel Holdings", "Accountant", "2016 - 2019"],
    ]
    for r, row in enumerate(rows):
        top = 765 - r * 30
        for k, cell in enumerate(row):
            c.drawString(xs[k] + 4, top - 18, cell)
        c.line(xs[0], top, xs[-1], top)
    c.line(xs[0], 765 - len(rows) * 30, xs[-1], 765 - len(rows) * 30)
    for x in xs:
        c.line(x, 765, x, 765 - len(rows) * 30)
    main = [
        "Education",
        "Bachelor of Accounting, Universiti Contoh (2012 - 2016)",
        "Projects",
        "Led the move of the general ledger to S/4HANA for three companies",
        "Built monthly cash forecasts used by the treasury team",
        "Automated bank reconciliations, saving two days each month",
        "Trained twelve users on the new reporting dashboards",
    ]
    for i, line in enumerate(main):
        c.drawString(200, 640 - i * 22, line)
    c.save()
    return buf.getvalue()


def test_a_sidebar_beside_a_work_table_stays_out_of_the_work_history() -> None:
    text = extract(two_column_table_cv(), ocr=None).text or ""
    assert text.count("Mandarin") == 1
    work = text[text.index("IT Experience") : text.index("Education")]
    for word in ("SAP FICO", "Mandarin", "Running"):
        assert word not in work, word
    assert text.index("SKILLS") < text.index("SAP FICO") < text.index("LANGUAGES")
    cv = parse_cv(text).model_dump()
    jobs = cv["experience"]
    assert [(j["title"]["value"], j["company"]["value"]) for j in jobs] == [
        ("Finance Analyst", "Contoh Berhad"),
        ("Accountant", "Sampel Holdings"),
    ]
    assert "SAP FICO" in [s["value"] for s in cv["skills"]]


def test_a_row_with_no_role_or_employer_never_takes_the_row_above() -> None:
    text = (
        "NUR AINA EXAMPLE\n\nIT Experience\n"
        "Duration: 11/2019 to 03/2026\nRole: Analyst\nOrganization: Contoh Berhad\nLocation: Malaysia\n\n"
        "Duration: 06/2017 to 11/2019\nLocation: India\nIndustry: Pharmaceutical\n"
    )
    jobs = parse_cv(text).model_dump()["experience"]
    assert [j["company"]["value"] for j in jobs if j["company"]].count("Contoh Berhad") == 1

"""A work history in tables, one job per row: an "IT Experience" table
(Organization | Role | Location | Client | Industry | Duration) with cells
wrapped over two lines and a last row cut by an inner rule, then an older
"Manufacturing Experience" table in another column order. Each row is one
job with its own title and employer, a header row is never either, and the
newest row is the current job. All names are invented; the layout copies a
real CV."""

from __future__ import annotations

import io
from collections.abc import Sequence

from docx import Document
from reportlab.lib.pagesizes import A4
from reportlab.pdfgen import canvas

from hiringplug_ai.extraction import extract
from hiringplug_ai.extraction.tables import nested_merged
from hiringplug_ai.parsing import parse_cv

IT_HEAD = ["Organization", "Role", "Location", "Client", "Industry", "Duration"]
# Each cell as its lines; the last row's second lines fall below an inner rule.
IT_ROWS = [
    [
        ["Contoh Berhad GSD KL -", "IT Shared Services"],
        ["Regional IT Senior Business", "Functional Analyst - SAP FICO"],
        ["Malaysia"],
        ["In-House Projects"],
        ["FMCG"],
        ["11/2019 to", "03/2026"],
    ],
    [
        ["Sampel Pharma Pvt Ltd -", "Global Technology Centre"],
        ["Regional IT Operations", "Analyst - SAP MM/SD"],
        ["India"],
        ["In-House projects"],
        ["Pharmaceutical"],
        ["06/2017 to", "11/2019"],
    ],
    [
        ["Demo Infotech", "(DEMO MINDTREE)"],
        ["SAP Functional", "Consultant - MM/WM"],
        ["India"],
        ["Contoh Semiconductors"],
        ["Semiconductors"],
        ["09/2010 to", "03/2013"],
    ],
]
OLD_HEAD = ["Organization", "Location", "Role", "Industry", "Duration"]
OLD_ROWS = [
    ["Contoh Valves & Controls", "India", "Purchasing Engineer", "Engineering", "01/2010 to 08/2010"],
    ["Sampel Tooling Asia Pvt Ltd", "India", "Assembly Supervisor", "Machinery", "04/2007 to 11/2007"],
]
WANT = [
    (
        "Regional IT Senior Business Functional Analyst - SAP FICO",
        "Contoh Berhad GSD KL - IT Shared Services",
        "11/2019 to 03/2026",
    ),
    (
        "Regional IT Operations Analyst - SAP MM/SD",
        "Sampel Pharma Pvt Ltd - Global Technology Centre",
        "06/2017 to 11/2019",
    ),
    ("SAP Functional Consultant - MM/WM", "Demo Infotech (DEMO MINDTREE)", "09/2010 to 03/2013"),
    ("Purchasing Engineer", "Contoh Valves & Controls", "01/2010 to 08/2010"),
    ("Assembly Supervisor", "Sampel Tooling Asia Pvt Ltd", "04/2007 to 11/2007"),
]


def _ruled(
    c: canvas.Canvas, top: float, xs: Sequence[float], rows: list[list[list[str]]], *, cut_last: bool
) -> float:
    """A ruled table drawn line by line; returns its bottom. Every row is two
    lines high; with cut_last, an inner rule (short of the borders, as Word
    draws a cell's paragraph border) cuts the last row in two."""
    c.line(xs[0], top, xs[-1], top)
    y = top
    for n, row in enumerate(rows):
        height = 24 if any(len(cell) > 1 for cell in row) or n else 14
        for k, cell in enumerate(row):
            for line_no, line in enumerate(cell):
                c.drawString(xs[k] + 2, y - 9 - 11 * line_no, line)
        if cut_last and n == len(rows) - 1:
            c.line(xs[1], y - 12, xs[-2], y - 12)
        y -= height
        c.line(xs[0], y, xs[-1], y)
    for x in xs:
        c.line(x, top, x, y)
    return y


def two_tables_cv() -> bytes:
    buf = io.BytesIO()
    c = canvas.Canvas(buf, pagesize=A4)
    c.setFont("Helvetica", 7)
    lines = [
        "+60 12 000 0009",
        "NUR AINA EXAMPLE",
        "aina@example.com",
        "Kuala Lumpur, Malaysia",
        "Professional Overview",
        "• Regional IT lead and SAP finance analyst across APAC and Europe.",
        "Experience Summary",
        "• 15 years of SAP consulting, S/4HANA conversions and regional rollouts.",
        "Academic Background",
        "• Bachelor of Mechanical Engineering, Universiti Contoh (2001 to 2005)",
        "IT Experience",
    ]
    for i, line in enumerate(lines):
        c.drawString(40, 810 - i * 12, line)
    # Thin edge columns, as the real table has, so an inner rule stops short of the borders.
    xs = [40, 46, 150, 270, 330, 420, 490, 560, 566]
    it_rows = [[[""], *[[h] for h in IT_HEAD], [""]], *[[[""], *row, [""]] for row in IT_ROWS]]
    y = _ruled(c, 670, xs, it_rows, cut_last=True)
    after = [
        "Contoh Berhad - Global Shared IT Service",
        "Project 1 SAP S/4HANA - Program Pulse",
        "• Led the conversion of finance from ECC to S/4HANA for twelve markets.",
        "Manufacturing Experience",
    ]
    for i, line in enumerate(after):
        c.drawString(40, y - 16 - i * 12, line)
    old = [[[h] for h in OLD_HEAD], *[[[cell] for cell in row] for row in OLD_ROWS]]
    y = _ruled(c, y - 16 - len(after) * 12, [40, 160, 230, 350, 440, 566], old, cut_last=False)
    for i, line in enumerate(["Responsibilities", "• Material planning and supplier development."]):
        c.drawString(40, y - 16 - i * 12, line)
    c.save()
    return buf.getvalue()


def jobs_of(text: str) -> list[tuple[str | None, str | None, str | None]]:
    return [
        (
            j.title.value if j.title else None,
            j.company.value if j.company else None,
            j.dates.value if j.dates else None,
        )
        for j in parse_cv(text).experience
    ]


def test_each_row_of_both_tables_is_one_job_with_its_own_title_and_employer() -> None:
    text = extract(two_tables_cv(), ocr=None).text or ""
    assert "Duration: 09/2010 to 03/2013" in text, "a row cut by an inner rule stays one row"
    assert jobs_of(text) == WANT


def test_the_newest_row_is_the_current_job() -> None:
    """The app takes the current title and company from the job that ended
    last (else the first listed): the IT table's first row, never the older
    table further down."""
    jobs = parse_cv(extract(two_tables_cv(), ocr=None).text or "").experience
    assert all(j.startDate and j.endDate for j in jobs), "every row has both dates"
    latest = max(jobs, key=lambda j: str(j.endDate))  # "2026-03": ISO months sort as text
    assert latest is jobs[0]
    assert (latest.title and latest.title.value, latest.company and latest.company.value) == WANT[0][:2]


def test_a_row_inside_a_row_stays_that_rows() -> None:
    first = [((0.0, 10.0), "Demo Infotech"), ((10.0, 20.0), "09/2010 to")]
    inner = [((0.0, 10.0), "(DEMO MINDTREE)"), ((10.0, 20.0), "03/2013")]
    after = [((0.0, 10.0), "Contoh Berhad"), ((10.0, 20.0), "2014 - 2016")]
    rows = [((100.0, 130.0), first), ((118.0, 130.0), inner), ((130.0, 150.0), after)]
    assert nested_merged(rows) == [first + inner, after]


def test_a_tables_header_row_read_as_a_line_is_neither_a_title_nor_an_employer() -> None:
    """A CV read before work tables were read cell by cell: each row on one line."""
    text = (
        "NUR AINA EXAMPLE\n\nManufacturing Experience\nOrganization Location Role Industry Duration\n"
        "Contoh Valves & Controls India Purchasing Engineer Engineering 01/2010 to 08/2010\n"
        "Sampel Tooling Asia Pvt Ltd India Assembly Supervisor Machinery 04/2007 to 11/2007\n"
        "Responsibilities\n• Material planning and supplier development.\n"
    )
    found = jobs_of(text)
    assert len(found) == 2
    for title, company, _ in found:
        assert title != "Organization Location Role Industry Duration"
        assert company != "Organization Location Role Industry Duration"
        assert title != "Responsibilities"


# A Word table: its client has a company's legal name, its employer does not.
WORD_HEAD = ["Role", "Organization", "Location", "Client", "Duration"]
WORD_ROWS = [
    [
        "SAP Functional Consultant",
        "Demo Infotech",
        "India",
        "Contoh Semiconductors Sdn Bhd",
        "09/2010 to 03/2013",
    ],
    ["Purchasing Engineer", "Contoh Valves & Controls", "Kulim", "Sampel Pumps Bhd", "01/2010 to 08/2010"],
]


def test_a_word_table_is_read_by_its_columns() -> None:
    doc = Document()
    doc.add_paragraph("NUR AINA EXAMPLE")
    doc.add_paragraph("Work Experience")
    table = doc.add_table(rows=1 + len(WORD_ROWS), cols=len(WORD_HEAD))
    for r, row in enumerate([WORD_HEAD, *WORD_ROWS]):
        for k, cell in enumerate(row):
            table.cell(r, k).text = cell
    buf = io.BytesIO()
    doc.save(buf)
    text = extract(buf.getvalue(), ocr=None).text or ""
    assert "Role | Organization | Location | Client | Duration" in text
    assert jobs_of(text) == [(role, org, dates) for role, org, _, _, dates in WORD_ROWS], "never the client"

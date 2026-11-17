"""Two-column pages are read column by column; one-column pages are not split,
even when their dates are aligned on the right. All text is invented."""

from __future__ import annotations

import io

from reportlab.lib.pagesizes import A4
from reportlab.pdfgen import canvas

from hiringplug_ai.extraction import extract

SIDEBAR = [
    "CONTACT",
    "Mobile",
    "+6012-000 0001",
    "Email",
    "aina@example.com",
    "SKILLS",
    "SAP FICO",
    "Excel",
    "Power BI",
    "LANGUAGES",
    "English",
    "Bahasa Malaysia",
    "Mandarin",
    "REFERENCES",
    "On request",
]
MAIN = [
    "NUR AINA BINTI KAMAL",
    "Senior Analyst, SAP FICO",
    "WORK EXPERIENCE",
    "Senior Analyst, SAP FICO - Contoh IT (M) Sdn. Bhd.",
    "Mar 2023 - Present",
    "Supported month-end closing and change requests for the finance team.",
    "Analyst, SAP FICO - Contoh Business Solutions Sdn. Bhd.",
    "Nov 2021 - Mar 2023",
    "Guided the S/4HANA migration for clients.",
    "EDUCATION",
    "Universiti Contoh Malaysia",
    "BSc. (Hons) Actuarial Science, 2017 - 2021",
]


def two_column_pdf() -> bytes:
    buffer = io.BytesIO()
    c = canvas.Canvas(buffer, pagesize=A4)
    c.setFont("Helvetica", 9)
    for i, line in enumerate(SIDEBAR):  # a narrow sidebar with its own line spacing
        c.drawString(40, 790 - i * 23, line)
    c.setFont("Helvetica", 11)
    for i, line in enumerate(MAIN):
        c.drawString(210, 800 - i * 31, line)
    c.save()
    return buffer.getvalue()


def test_a_sidebar_and_a_main_column_are_read_one_after_the_other() -> None:
    text = extract(two_column_pdf(), ocr=None).text or ""
    lines = text.splitlines()
    for column in (SIDEBAR, MAIN):  # each column's lines, in order and unmixed
        start = lines.index(column[0])
        assert lines[start : start + len(column)] == column


def test_right_aligned_dates_keep_a_one_column_page_whole() -> None:
    buffer = io.BytesIO()
    c = canvas.Canvas(buffer, pagesize=A4)
    c.setFont("Helvetica", 11)
    rows = [
        ("Regional Sales Manager", "06/2014 - 11/2015"),
        ("Contoh Consumer Goods", ""),
        ("Trade Marketing Manager", "09/2013 - 05/2014"),
        ("Contoh Foods Berhad", ""),
    ] * 4
    for i, (left, right) in enumerate(rows):
        c.drawString(60, 790 - i * 24, left)
        if right:
            c.drawRightString(540, 790 - i * 24, right)
    c.save()
    text = extract(buffer.getvalue(), ocr=None).text or ""
    assert "Regional Sales Manager 06/2014 - 11/2015" in text


def test_a_letter_spaced_name_is_read_as_words() -> None:
    """A designed CV prints "N O E L L E  K H O O": each letter is its own word
    in the PDF, the words' gap wider than the letters'."""
    buffer = io.BytesIO()
    c = canvas.Canvas(buffer, pagesize=A4)
    c.setFont("Helvetica-Bold", 22)
    x = 200.0
    for word in ("AINA", "KAMAL"):
        for letter in word:
            c.drawString(x, 780, letter)
            x += c.stringWidth(letter, "Helvetica-Bold", 22) + 6
        x += 12
    c.setFont("Helvetica", 11)
    c.drawString(200, 750, "P R O D U C T")  # one space between letters: one word
    c.drawString(200, 720, "Summary of a career")
    c.save()
    text = extract(buffer.getvalue(), ocr=None).text or ""
    assert "AINA KAMAL" in text.splitlines()

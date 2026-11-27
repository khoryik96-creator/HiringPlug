"""A PDF that stores no spaces between words (Word's "Save as PDF" with some
fonts): words are told apart only by a 2.5-point gap at a 9-point size, under
pdfplumber's fixed 3-point tolerance, and were read as one word
("DatabaseAdministrator · HongLeongBank(PetalingJaya,Malaysia)")."""

from __future__ import annotations

import io

import pdfplumber
from reportlab.lib.pagesizes import A4
from reportlab.pdfgen import canvas

from hiringplug_ai.extraction import extract
from hiringplug_ai.extraction.spacing import word_options

LINES = [
    "NURUL AINA BINTI KAMAL",
    "Database Administrator",
    "Hong Leong Bank (Petaling Jaya, Malaysia)",
    "I am basically working as an IT professional with more than 15 years of experience",
]


def spaceless_pdf(gap: float = 2.5, size: int = 9) -> bytes:
    """Every word drawn on its own, with no space character anywhere."""
    buffer = io.BytesIO()
    c = canvas.Canvas(buffer, pagesize=A4)
    c.setFont("Helvetica", size)
    y = 780.0
    for line in LINES:
        x = 60.0
        for word in line.split(" "):
            c.drawString(x, y, word)
            x += c.stringWidth(word, "Helvetica", size) + gap
        y -= 14
    c.showPage()
    c.save()
    return buffer.getvalue()


def test_a_pdf_without_spaces_is_read_as_words() -> None:
    text = extract(spaceless_pdf(), ocr=None).text or ""
    for line in LINES:
        assert line in text, text


def test_a_pdf_with_spaces_reads_as_before() -> None:
    """A page that does store its spaces keeps pdfplumber's usual reading."""
    buffer = io.BytesIO()
    c = canvas.Canvas(buffer, pagesize=A4)
    c.setFont("Helvetica", 9)
    for i, line in enumerate(LINES):
        c.drawString(60, 780 - 14 * i, line)
    c.showPage()
    c.save()
    text = extract(buffer.getvalue(), ocr=None).text or ""
    for line in LINES:
        assert line in text, text


def test_a_column_without_spaces_on_a_page_with_them_reads_as_before() -> None:
    """The choice is the page's: a crop holding one word per line (a sidebar)
    has no space character, yet its page stores them, so it is read as usual."""
    buffer = io.BytesIO()
    c = canvas.Canvas(buffer, pagesize=A4)
    c.setFont("Helvetica", 9)
    for i, word in enumerate(["Python", "SQL", "Excel"]):
        c.drawString(40, 780 - 14 * i, word)
    c.drawString(300, 780, "Database Administrator")
    c.showPage()
    c.save()
    with pdfplumber.open(io.BytesIO(buffer.getvalue())) as pdf:
        page = pdf.pages[0]
        sidebar = page.within_bbox((0, 0, 200, page.height))
        assert not any(ch["text"] == " " for ch in sidebar.chars)
        assert word_options(sidebar) == word_options(page) == {}

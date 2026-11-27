"""A PDF that stores no spaces between words (Word's "Save as PDF" with some
fonts): words are told apart only by a 2.5-point gap at a 9-point size, under
pdfplumber's fixed 3-point tolerance, and were read as one word
("DatabaseAdministrator · HongLeongBank(PetalingJaya,Malaysia)")."""

from __future__ import annotations

import io

from reportlab.lib.pagesizes import A4
from reportlab.pdfgen import canvas

from hiringplug_ai.extraction import extract

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

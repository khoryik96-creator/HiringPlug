"""Test documents, generated in code so no real CV (with real personal data)
ever needs to be committed."""

from __future__ import annotations

import io
import zipfile

import pypdfium2
from pypdf import PdfReader, PdfWriter
from reportlab.lib.pagesizes import A4
from reportlab.pdfgen import canvas


def text_pdf(*pages: list[str], font_size: int = 12) -> bytes:
    buffer = io.BytesIO()
    c = canvas.Canvas(buffer, pagesize=A4)
    for lines in pages:
        c.setFont("Helvetica", font_size)
        for i, line in enumerate(lines):
            c.drawString(60, 780 - i * (font_size + 8), line)
        c.showPage()
    c.save()
    return buffer.getvalue()


def scanned_pdf(pdf: bytes, rotate: int = 0, dpi: int = 200) -> bytes:
    """Rasterize every page (like a scanner would) into an image-only PDF."""
    document = pypdfium2.PdfDocument(pdf)
    images = [
        document[i].render(scale=dpi / 72).to_pil().convert("RGB").rotate(rotate, expand=True)
        for i in range(len(document))
    ]
    out = io.BytesIO()
    images[0].save(out, format="PDF", resolution=dpi, save_all=True, append_images=images[1:])
    return out.getvalue()


def join_pdfs(*pdfs: bytes) -> bytes:
    writer = PdfWriter()
    for pdf in pdfs:
        for page in PdfReader(io.BytesIO(pdf)).pages:
            writer.add_page(page)
    out = io.BytesIO()
    writer.write(out)
    return out.getvalue()


CV_LINES = [
    "Nur Aisyah binti Rahman",
    "Senior Accountant, Maybank Berhad",
    "aisyah.rahman@example.com +60 12 345 6789",
    "Experience: 2016 - 2024 Audit Associate at Deloitte",
    "Skills: IFRS reporting, SAP, consolidation, C++",
]


def zip_bytes(files: dict[str, bytes], compression: int = zipfile.ZIP_DEFLATED) -> bytes:
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w", compression) as z:
        for name, data in files.items():
            z.writestr(name, data)
    return out.getvalue()

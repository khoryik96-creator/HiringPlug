"""Pictures of a CV are read with OCR (Tesseract), as CV Studio does."""

from __future__ import annotations

import io

import pypdfium2
import pytest
from PIL import Image

from hiringplug_ai.extraction import TesseractOcr, extract
from hiringplug_ai.extraction.sniff import sniff

from .documents import text_pdf

CV = [
    "Siti Aisyah binti Rahman",
    "Senior Auditor at Contoh Audit",
    "Statutory audit of listed groups",
    "SAP FICO and MFRS reporting",
]


def picture(fmt: str, scale: float = 1.0, rotate: int = 0, exif_turn: bool = False) -> bytes:
    page = pypdfium2.PdfDocument(text_pdf(CV, font_size=14))[0]
    image = page.render(scale=scale * 150 / 72).to_pil().convert("RGB").rotate(rotate, expand=True)
    out = io.BytesIO()
    if exif_turn:  # a phone photo stored sideways, with "turn me" in its EXIF
        exif = Image.Exif()
        exif[0x0112] = 6  # rotate 90 clockwise to display
        image.rotate(90, expand=True).save(out, format=fmt, exif=exif)
    else:
        image.save(out, format=fmt)
    return out.getvalue()


def test_pictures_are_told_by_their_bytes() -> None:
    for fmt in ("PNG", "JPEG", "TIFF", "BMP", "WEBP"):
        assert sniff(picture(fmt, scale=0.2)) == "image", fmt


@pytest.mark.ocr
def test_a_picture_of_a_cv_is_read() -> None:
    for fmt in ("PNG", "JPEG", "WEBP"):
        result = extract(picture(fmt), ocr=TesseractOcr())
        assert result.status == "extracted", (fmt, result.error)
        assert "Siti Aisyah" in (result.text or ""), fmt
        assert "MFRS" in (result.text or ""), fmt
        assert result.extractor.startswith("tesseract")


@pytest.mark.ocr
def test_a_small_screenshot_is_enlarged_and_a_phone_photo_turned_upright() -> None:
    small = extract(picture("PNG", scale=0.45), ocr=TesseractOcr())
    assert "Statutory audit" in (small.text or ""), small.text
    turned = extract(picture("JPEG", exif_turn=True), ocr=TesseractOcr())
    assert "Senior Auditor" in (turned.text or ""), turned.text


def test_a_picture_without_ocr_waits_for_it_and_a_damaged_one_says_so() -> None:
    assert extract(picture("PNG", scale=0.2), ocr=None).status == "needs_ocr"
    broken = extract(b"\x89PNG\r\n\x1a\n" + b"\x00" * 50, ocr=TesseractOcr())
    assert broken.status == "failed"
    blank = io.BytesIO()
    Image.new("RGB", (800, 1000), "white").save(blank, format="PNG")
    assert extract(blank.getvalue(), ocr=TesseractOcr()).status == "failed"


def test_a_text_starting_with_bm_is_not_a_picture() -> None:
    assert sniff(b"BMW Group Malaysia, Data Engineer since 2019, Kuala Lumpur") != "image"

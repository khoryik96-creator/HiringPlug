"""Reading a picture of a CV (a phone photo, a scan saved as PNG or JPG)
with OCR, as CV Studio does, and better: the picture is turned upright as the
camera recorded, read in greys, enlarged when small, and read turned when it
lies sideways."""

from __future__ import annotations

from .limits import Limits
from .ocr import OcrEngine
from .result import Extraction


def extract_image(data: bytes, limits: Limits, ocr: OcrEngine | None) -> Extraction:
    if ocr is None or not ocr.available():
        return Extraction(
            "needs_ocr",
            None,
            1,
            "none",
            error="This is a picture: its text can be read once OCR (Tesseract) is set up.",
        )
    text = ocr.read_image(data, limits)
    if not text or not text.strip():
        return Extraction("failed", None, 1, ocr.name, error="No text could be read from the picture.")
    warnings = []
    if len(text.split()) < 40:
        warnings.append(
            "Little text was read from the picture: check it, or ask for the CV as a PDF or Word file."
        )
    return Extraction("extracted", text.strip(), 1, ocr.name, warnings=warnings)

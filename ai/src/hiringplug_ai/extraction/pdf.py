"""PDF text extraction with page-by-page OCR routing.

Mechanism (from CV Studio, where it was tuned on real CVs):

1. Read every page's text layer with pdfplumber.
2. Decide per page whether that text is trustworthy:
   - "unextractable": the layer is glyph noise (fonts without a Unicode map);
   - "sparse-visual": little text, but the page is mostly an image (a scan) or
     dense vector outlines (text converted to shapes).
   A short page with no visual evidence is NOT routed: brief contact or skills
   pages are real, and OCR could damage emails, phone numbers or "C++".
3. OCR only the routed pages; keep every other page's exact text.
4. Per page, keep the exact text unless OCR recovered clearly more letters.
5. Never lose content silently: pages OCR could not read become warnings
   (visibly blank pages excepted).
"""

from __future__ import annotations

import io
from dataclasses import dataclass
from importlib.metadata import version
from typing import Any

from .limits import DocumentRejectedError, Limits
from .ocr import OcrEngine, OcrPages
from .result import Extraction
from .text import clean_text, letter_count, looks_unextractable

ENOUGH_LETTERS = 400  # a page with this much text is used as-is
IMAGE_COVERAGE_FLOOR = 0.20  # share of the page covered by images that suggests a scan
VECTOR_SHAPES_FLOOR = 300  # this many curves/rects may be outlined text...
VECTOR_BANDS_FLOOR = 3  # ...if they span at least this many of 12 horizontal bands


@dataclass
class _Page:
    text: str
    ocr_reason: str  # "" = use the text layer as-is


def extract_pdf(data: bytes, limits: Limits, ocr: OcrEngine | None) -> Extraction:
    import pdfplumber  # noqa: PLC0415

    with pdfplumber.open(io.BytesIO(data)) as pdf:
        if len(pdf.pages) > limits.max_pdf_pages:
            raise DocumentRejectedError(
                f"The PDF has {len(pdf.pages)} pages; the limit is {limits.max_pdf_pages}."
            )
        pages = []
        for page in pdf.pages:
            text = page.extract_text() or ""
            pages.append(_Page(text=text, ocr_reason=page_ocr_reason(page, text)))

    # A renderable PDF that exposes no text at all: try OCR on every page.
    if pages and not any(p.text.strip() for p in pages):
        for p in pages:
            p.ocr_reason = p.ocr_reason or "empty-document"

    extractor = f"pdfplumber@{version('pdfplumber')}"
    routed = [n for n, p in enumerate(pages, 1) if p.ocr_reason]
    if not routed:
        return Extraction("extracted", clean_text(merge_pages(pages, {})), len(pages), extractor)

    if ocr is None or not ocr.available():
        return _without_ocr(pages, routed, extractor)

    result = ocr.read_pdf_pages(data, routed, limits)
    text = clean_text(merge_pages(pages, result.texts))
    unread = unread_pages(pages, result, routed)
    extractor = f"{extractor}+{ocr.name}"
    if not text:
        return Extraction(
            "failed",
            None,
            len(pages),
            extractor,
            error="OCR could not read this document. It may be blank, too low-quality or rotated; "
            "try a clearer copy or a text-based PDF or Word file.",
        )
    warnings = []
    if unread:
        warnings.append(
            f"OCR could not read {_pages_label(unread)}. Text from the other pages was kept, "
            "but this document may be incomplete; review it or upload a clearer copy."
        )
    return Extraction("extracted", text, len(pages), extractor, warnings)


def _without_ocr(pages: list[_Page], routed: list[int], extractor: str) -> Extraction:
    usable = [n for n, p in enumerate(pages, 1) if not p.ocr_reason and p.text.strip()]
    if usable:
        text = clean_text("\n\n".join(p.text.strip() for p in pages if not p.ocr_reason and p.text.strip()))
        return Extraction(
            "extracted",
            text,
            len(pages),
            extractor,
            [
                f"{_pages_label(routed).capitalize()} {'looks' if len(routed) == 1 else 'look'} scanned, "
                "but OCR is not available; only the other pages were read."
            ],
        )
    return Extraction(
        "needs_ocr",
        None,
        len(pages),
        extractor,
        ["This looks like a scanned document. Enable OCR to read it."],
    )


def page_ocr_reason(page: Any, text: str) -> str:
    """Why this page needs OCR, or "" when its text layer can be used."""
    if looks_unextractable(text):
        return "unextractable"
    if letter_count(text) >= ENOUGH_LETTERS:
        return ""
    if image_coverage(page) >= IMAGE_COVERAGE_FLOOR or has_outlined_text(page):
        return "sparse-visual"
    return ""


def image_coverage(page: Any) -> float:
    """Approximate share of the page covered by embedded images (0..1)."""
    try:
        area = float(page.width) * float(page.height)
    except (TypeError, ValueError, AttributeError):
        return 0.0
    if area <= 0:
        return 0.0
    covered = 0.0
    for image in page.images or []:
        try:
            covered += abs(float(image["x1"]) - float(image["x0"])) * abs(
                float(image["bottom"]) - float(image["top"])
            )
        except (KeyError, TypeError, ValueError):
            continue
    return min(1.0, covered / area)


def has_outlined_text(page: Any) -> bool:
    """True when dense vector shapes spread over the page like lines of text."""
    shapes = list(page.curves or []) + list(page.rects or [])
    if len(shapes) < VECTOR_SHAPES_FLOOR:
        return False
    height = float(page.height or 0)
    if height <= 0:
        return False
    bands: set[int] = set()
    for shape in shapes:
        try:
            first = max(0, min(11, int(float(shape["top"]) / height * 12)))
            last = max(0, min(11, int(float(shape["bottom"]) / height * 12)))
        except (KeyError, TypeError, ValueError):
            continue
        bands.update(range(min(first, last), max(first, last) + 1))
    return len(bands) >= VECTOR_BANDS_FLOOR


def choose_page_text(page: _Page, ocr_text: str) -> str:
    """The text to keep for one page: exact layer, OCR, or whichever has clearly more content."""
    exact = page.text.strip()
    if not page.ocr_reason:
        return exact
    ocr_text = ocr_text.strip()
    if page.ocr_reason == "unextractable" or not exact:
        return ocr_text
    exact_letters, ocr_letters = letter_count(exact), letter_count(ocr_text)
    return ocr_text if ocr_letters >= max(exact_letters + 10, int(exact_letters * 1.5)) else exact


def merge_pages(pages: list[_Page], ocr_texts: dict[int, str]) -> str:
    chosen = (choose_page_text(p, ocr_texts.get(n, "")) for n, p in enumerate(pages, 1))
    return "\n\n".join(text for text in chosen if text)


def unread_pages(pages: list[_Page], result: OcrPages, routed: list[int]) -> list[int]:
    """Routed pages that contributed nothing, excluding pages proven blank."""
    return [
        n
        for n in routed
        if n not in result.blank and not choose_page_text(pages[n - 1], result.texts.get(n, ""))
    ]


def _pages_label(numbers: list[int]) -> str:
    noun = "page" if len(numbers) == 1 else "pages"
    return f"{noun} {', '.join(str(n) for n in numbers)}"

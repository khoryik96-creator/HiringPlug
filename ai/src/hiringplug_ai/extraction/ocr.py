"""OCR for scanned pages: render with PDFium, read with Tesseract.

Bounded like everything else: at most `max_ocr_pages`, a per-page timeout,
an overall deadline, and a pixel limit per rendered page. Pages that are
visibly blank are reported as blank, not as "unreadable".

Sideways or upside-down scans: when a page reads as (almost) no words, it is
read again turned 90, 180 and 270 degrees, and the clearly best reading wins.
"""

from __future__ import annotations

import io
import re
import shutil
import time
from dataclasses import dataclass, field
from typing import Any, Protocol

from .limits import DocumentRejectedError, Limits


@dataclass
class OcrPages:
    texts: dict[int, str] = field(default_factory=dict)  # page number (1-based) -> text
    blank: set[int] = field(default_factory=set)
    skipped: list[int] = field(default_factory=list)  # not attempted: page or time limits


class OcrEngine(Protocol):
    @property
    def name(self) -> str: ...

    def available(self) -> bool: ...

    def read_pdf_pages(self, data: bytes, pages: list[int], limits: Limits) -> OcrPages: ...

    def read_image(self, data: bytes, limits: Limits) -> str | None: ...


# Pictures taken are these (Pillow's names); others are refused.
IMAGE_FORMATS = frozenset({"PNG", "JPEG", "MPO", "TIFF", "BMP", "WEBP"})
# A picture narrower than this is enlarged first: Tesseract reads small text
# badly (a phone screenshot of a CV is often under 1,000 pixels wide).
MIN_OCR_WIDTH = 1600


class TesseractOcr:
    """Tesseract via pytesseract. `languages` like "eng" or "eng+msa"."""

    def __init__(self, languages: str = "eng", binary: str = "tesseract") -> None:
        self.languages = languages
        self.binary = binary

    @property
    def name(self) -> str:
        try:
            import pytesseract  # noqa: PLC0415 - optional at import time

            return f"tesseract@{pytesseract.get_tesseract_version()}"
        except Exception:  # noqa: BLE001 - version is informational only
            return "tesseract"

    def available(self) -> bool:
        return shutil.which(self.binary) is not None

    def read_pdf_pages(self, data: bytes, pages: list[int], limits: Limits) -> OcrPages:
        import pypdfium2  # noqa: PLC0415
        import pytesseract  # noqa: PLC0415

        pytesseract.pytesseract.tesseract_cmd = self.binary
        result = OcrPages()
        deadline = time.monotonic() + limits.ocr_total_deadline_seconds
        document = pypdfium2.PdfDocument(data)
        try:
            for position, number in enumerate(pages):
                if position >= limits.max_ocr_pages or time.monotonic() > deadline:
                    result.skipped.extend(pages[position:])
                    break
                image = _render(document, number, limits)
                if image is None:
                    result.skipped.append(number)
                    continue
                if _visually_blank(image):
                    result.blank.add(number)
                    continue
                result.texts[number] = self._read_upright(pytesseract, image, limits, deadline)
        finally:
            document.close()
        return result

    def read_image(self, data: bytes, limits: Limits) -> str | None:
        """A picture's text; None when it is blank. Turned upright as the
        camera recorded (EXIF), in greys, enlarged when small; a sideways
        picture is also read turned. Raises DocumentRejectedError for a
        picture that is not one of IMAGE_FORMATS or is too large."""
        import pytesseract  # noqa: PLC0415
        from PIL import Image, ImageOps  # noqa: PLC0415

        pytesseract.pytesseract.tesseract_cmd = self.binary
        Image.MAX_IMAGE_PIXELS = limits.max_image_pixels
        try:
            opened = Image.open(io.BytesIO(data))
            kind = str(opened.format or "").upper()
            if kind not in IMAGE_FORMATS:
                raise DocumentRejectedError("Pictures can be PNG, JPG, TIFF, BMP or WEBP.")
            width, height = opened.size
            if width * height > limits.max_image_pixels:
                raise DocumentRejectedError("The picture is too large to read.")
            opened.seek(0)  # a multi-page TIFF: its first page
            image = ImageOps.exif_transpose(opened).convert("L")
        except (Image.DecompressionBombError, OSError) as exc:
            raise DocumentRejectedError("The picture is damaged or too large to read.") from exc
        if image.width < MIN_OCR_WIDTH:
            scale = MIN_OCR_WIDTH / image.width
            if image.width * scale * image.height * scale <= limits.max_image_pixels:
                image = image.resize((MIN_OCR_WIDTH, round(image.height * scale)), Image.Resampling.LANCZOS)
        if _visually_blank(image):
            return None
        deadline = time.monotonic() + limits.ocr_total_deadline_seconds
        return self._read_upright(pytesseract, image, limits, deadline)

    def _read_upright(self, pytesseract: Any, image: Any, limits: Limits, deadline: float) -> str:
        def read(img: Any) -> str:
            return str(
                pytesseract.image_to_string(img, lang=self.languages, timeout=limits.ocr_page_timeout_seconds)
            )

        best = read(image)
        best_score = word_score(best)
        if best_score >= ROTATION_RETRY_BELOW:
            return best
        for angle in (90, 180, 270):
            if time.monotonic() > deadline:
                break
            text = read(image.rotate(angle, expand=True, fillcolor=255))
            score = word_score(text)
            if score >= max(ROTATION_RETRY_BELOW, best_score * 2):
                best, best_score = text, score
        return best


# A page reading as fewer real-looking words than this is retried rotated.
ROTATION_RETRY_BELOW = 8
_WORD = re.compile(r"^[A-Za-z][a-z]{2,}[,.:;]?$")


def word_score(text: str) -> int:
    """How many tokens look like real words; OCR of a sideways page scores near zero."""
    return sum(1 for token in text.split() if _WORD.match(token))


def _render(document: Any, number: int, limits: Limits) -> Any | None:
    page = document[number - 1]
    try:
        width, height = page.get_size()
        scale = limits.ocr_dpi / 72
        if width * scale * height * scale > limits.max_image_pixels:
            return None
        return page.render(scale=scale).to_pil().convert("L")
    finally:
        page.close()


def _visually_blank(image: Any) -> bool:
    low, _ = image.getextrema()
    return bool(low >= 245)

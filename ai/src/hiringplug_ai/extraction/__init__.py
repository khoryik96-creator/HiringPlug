"""Reading CVs: turn an uploaded file into plain text, safely.

    extract(data, ocr=TesseractOcr()) -> Extraction

The file type is decided from the bytes. Problems with the document itself
(damaged, unsafe, unreadable) produce a "failed" Extraction with a message a
recruiter can act on; they are not exceptions, because retrying would not help.
"""

from __future__ import annotations

import dataclasses
import logging
from collections.abc import Callable

from .doc_reader import extract_doc
from .docx_reader import extract_docx
from .image import extract_image
from .limits import DEFAULT_LIMITS, DocumentRejectedError, Limits
from .links import docx_links, with_links
from .ocr import OcrEngine, TesseractOcr
from .pdf import extract_pdf
from .result import Extraction
from .sniff import sniff

__all__ = ["DEFAULT_LIMITS", "Extraction", "Limits", "OcrEngine", "TesseractOcr", "extract"]

log = logging.getLogger(__name__)


UNSUPPORTED = (
    "Only PDF, Word (.docx or .doc), text (.txt, .rtf, .odt) and picture "
    "(PNG, JPG, TIFF, BMP, WEBP) files can be read."
)


def extract(data: bytes, *, ocr: OcrEngine | None, limits: Limits = DEFAULT_LIMITS) -> Extraction:
    kind = sniff(data)
    readers: dict[str, Callable[[], Extraction]] = {
        "pdf": lambda: _add_links(extract_pdf(data, limits, ocr)),  # links read with the text
        "docx": lambda: _add_links(extract_docx(data, limits), lambda: docx_links(data)),
        "doc": lambda: extract_doc(data, limits),
        "image": lambda: extract_image(data, limits, ocr),
    }
    read = readers.get(kind)
    if read is None:
        return Extraction("failed", None, None, "none", error=UNSUPPORTED)
    try:
        return read()
    except DocumentRejectedError as exc:
        return Extraction("failed", None, None, kind, error=str(exc))
    except Exception:
        # A damaged file can make the parsing libraries raise almost anything.
        log.warning("document could not be parsed", exc_info=True, extra={"document_kind": kind})
        return Extraction("failed", None, None, kind, error="The file is damaged or not a valid document.")


def _add_links(result: Extraction, read: Callable[[], list[str]] | None = None) -> Extraction:
    """Kano's way: links hidden behind words ("LinkedIn") are added to the text."""
    if result.status != "extracted" or not result.text:
        return result
    try:
        links = read() if read else result.links
    except Exception:  # the text stands without them
        log.warning("links could not be read", exc_info=True)
        return result
    return dataclasses.replace(result, text=with_links(result.text, links), links=links)

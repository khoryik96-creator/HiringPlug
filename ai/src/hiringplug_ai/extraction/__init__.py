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
from .limits import DEFAULT_LIMITS, DocumentRejectedError, Limits
from .links import docx_links, pdf_links, with_links
from .ocr import OcrEngine, TesseractOcr
from .pdf import extract_pdf
from .result import Extraction
from .sniff import sniff

__all__ = ["DEFAULT_LIMITS", "Extraction", "Limits", "OcrEngine", "TesseractOcr", "extract"]

log = logging.getLogger(__name__)


def extract(data: bytes, *, ocr: OcrEngine | None, limits: Limits = DEFAULT_LIMITS) -> Extraction:
    kind = sniff(data)
    try:
        if kind == "pdf":
            return _add_links(extract_pdf(data, limits, ocr), data, pdf_links)
        if kind == "docx":
            return _add_links(extract_docx(data, limits), data, docx_links)
        if kind == "doc":
            return extract_doc(data, limits)
    except DocumentRejectedError as exc:
        return Extraction("failed", None, None, kind, error=str(exc))
    except Exception:
        # A damaged file can make the parsing libraries raise almost anything.
        log.warning("document could not be parsed", exc_info=True, extra={"document_kind": kind})
        return Extraction("failed", None, None, kind, error="The file is damaged or not a valid document.")
    return Extraction(
        "failed", None, None, "none", error="Only PDF and Word (.docx or .doc) files can be read."
    )


def _add_links(result: Extraction, data: bytes, read: Callable[[bytes], list[str]]) -> Extraction:
    """Kano's way: links hidden behind words ("LinkedIn") are added to the text."""
    if result.status != "extracted" or not result.text:
        return result
    try:
        links = read(data)
    except Exception:  # the text stands without them
        log.warning("links could not be read", exc_info=True)
        return result
    return dataclasses.replace(result, text=with_links(result.text, links))

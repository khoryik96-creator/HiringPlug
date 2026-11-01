"""Safety limits for reading untrusted documents.

CVs come from strangers. A crafted file can try to exhaust memory or CPU (zip
bombs, enormous images, thousands of pages), so every stage is bounded. The
defaults follow CV Studio's proven limits.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Limits:
    max_pdf_pages: int = 80
    max_ocr_pages: int = 30
    max_image_pixels: int = 60_000_000
    max_zip_entries: int = 2000
    max_zip_expanded_bytes: int = 160 * 1024 * 1024
    max_zip_single_entry_bytes: int = 64 * 1024 * 1024
    ocr_dpi: int = 220
    ocr_page_timeout_seconds: int = 35
    ocr_total_deadline_seconds: int = 180


DEFAULT_LIMITS = Limits()


class DocumentRejectedError(ValueError):
    """The document is unsafe or unreadable. The message is safe to show to users."""

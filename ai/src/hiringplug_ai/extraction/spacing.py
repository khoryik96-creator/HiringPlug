"""How far apart two letters must be to start a new word, per page.

pdfplumber joins letters closer than 3 points (its default). Most PDFs also
store the spaces between words, so that is enough. Some (Word's "Save as
PDF" with certain fonts) store no space at all: words are told apart only by
the gap, here 2.5 points at a 9-point size, and were read as one word
("DatabaseAdministrator"). On a page with no space at all the gap is judged
against the letters' size instead: a word gap is far wider than the gap
between letters of a word (under half a point at that size)."""

from __future__ import annotations

from typing import Any

# A gap wider than this share of the letters' size starts a new word, on a
# page that stores no spaces (1.35 points at 9 points).
GAP_RATIO = 0.15


def word_options(page: Any) -> dict[str, Any]:
    """The pdfplumber options for reading this page's words and text."""
    chars = page.chars
    if chars and not any(c.get("text") == " " for c in chars):
        return {"x_tolerance_ratio": GAP_RATIO}
    return {}

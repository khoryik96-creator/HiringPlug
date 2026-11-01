"""Clean-up of text extraction artefacts.

Conservative on purpose: it fixes known extractor noise and never rewrites
real words, so names, emails, phone numbers and terms like C++ survive intact.
"""

from __future__ import annotations

import re

# Some PDFs (Word/Canva exports) expose common ligatures as "(cid:N)" glyph
# codes. These mappings were observed in real CVs (via CV Studio):
#   Accoun(cid:415)ng -> Accounting, so(cid:332)ware -> software.
_CID_LIGATURES = {"332": "ft", "414": "tf", "415": "ti", "425": "tt", "427": "tti"}
_CID = re.compile(r"\(cid:(\d+)\)")
_SPACES = re.compile(r"[ \t\u00a0]+")
_BLANK_LINES = re.compile(r"\n{3,}")


def clean_text(text: str) -> str:
    """Remove NULs and control noise, fix known ligature codes, tidy whitespace."""
    text = _CID.sub(lambda m: _CID_LIGATURES.get(m.group(1), " "), text)
    text = "".join(ch for ch in text if ch in "\n\t" or ord(ch) >= 0x20)  # drops NUL and controls
    lines = [
        _SPACES.sub(" ", line).strip() for line in text.replace("\r\n", "\n").replace("\r", "\n").split("\n")
    ]
    return _BLANK_LINES.sub("\n\n", "\n".join(lines)).strip()


def looks_unextractable(text: str) -> bool:
    """True when a PDF text layer is glyph noise rather than words.

    Fonts without a Unicode map render fine but extract as "(cid:N)" tokens,
    control codes or replacement characters. Such a page must be OCR'd even
    though its text layer is not empty. Script-agnostic: real non-Latin text
    (CJK, Arabic, Tamil...) has none of these artefacts, so it is never
    misrouted to OCR.
    """
    sample = text[:8000]
    total = len(sample)
    if total < 40 or not sample.strip():
        return False
    cid_tokens = _CID.findall(sample)
    cid_chars = sum(len(m.group(0)) for m in _CID.finditer(sample))
    if len(cid_tokens) >= 8 and cid_chars > total * 0.30:
        return True
    control = sum(1 for ch in sample if ord(ch) < 0x20 and ch not in "\t\n\r\f")
    if control > total * 0.10:
        return True
    return sample.count("�") > total * 0.10


def letter_count(text: str) -> int:
    return sum(1 for ch in _CID.sub("", text) if ch.isalpha())

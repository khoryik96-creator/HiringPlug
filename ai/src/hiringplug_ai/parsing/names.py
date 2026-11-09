"""The candidate's name.

Candidates come from the top of the CV, in several shapes: a line on its own,
the start of a line shared with contacts ("SOH YONG HUAT 013-2899564") or a
title ("TAN PEI YIN Senior Oracle Engineer"), and labelled ("Name: ...").
Each must look like a person's name (CV Studio's rules: no digits, "@",
commas, address words or places) and not like a job title or heading.
The best candidate shares words with the email address or LinkedIn link
("siewnee84tan@..." -> "TAN SIEW NEE"); otherwise the first one wins.
"""

from __future__ import annotations

import re

from .layout import Line, is_bullet, section_of
from .vocab import COMPANY_RE, PLACES, ROLE_RE

_PARTICLES = {
    "bin",
    "binti",
    "bt",
    "bte",
    "a/l",
    "a/p",
    "al",
    "el",
    "de",
    "da",
    "van",
    "der",
    "von",
    "dos",
    "del",
}
_ADDRESS = re.compile(
    r"\b(?:jalan|jln|lorong|lrg|taman|kampung|kg|bandar|persiaran|lebuh|block|blok|street|road|avenue|floor|"
    r"tingkat|apartment|residenc[ey]|condominium|postcode|poskod|curriculum|vitae|resume|profile|summary|"
    r"objective|address|mobile|phone|email|nationality|personal|details|experience|education|skills|contact|"
    # labels a CV puts before a value ("LOCATION Penang", "Based in Kuala Lumpur")
    r"location|based|residing|citizenship|gender|sex|age|birth|dob|religion|race|marital|status|languages?|"
    r"salary|notice|availability|available|expected|current|references?|hobbies|interests|linkedin|github|"
    r"website|portfolio|passport|nric|ic)\b",
    re.IGNORECASE,
)
_WORD = re.compile(r"^[^\W\d_](?:[^\W\d_]|['’.\-/])*$")
_HONORIFIC = re.compile(
    r"^(?:(?:ts|dr|ir|mr|ms|mrs|mdm|madam|en|encik|pn|puan|cik|prof|dato'?|datuk|datin|tan\s+sri|sr|hj|haji)"
    r"\.?\s+)+",
    re.IGNORECASE,
)
_POST_NOMINALS = re.compile(r",\s*[A-Z][\w®&.\- ,]*$")
_NICKNAME = re.compile(r"\s*\([^)]{1,30}\)")
_LABEL = re.compile(
    r"^(?:(?:full\s+)?name|contact|curriculum\s+vitae\s+(?:for|of)|resume\s+of|cv\s+of)\b\s*[:：]?\s*", re.I
)
_CONTACT_SPLIT = re.compile(
    r"\s*(?:[|•·,]|\s(?=\+?\(?\d)|\b(?:phone|mobile|email|e-mail|tel|hp|h/p)\b\s*[:：]?)", re.IGNORECASE
)
# "TAN PEI YIN Senior ...", "THAM AH MENG (JORDAN) Curriculum Vitae", "TERRY GOH 60142002101"
_CAPS_RUN = re.compile(r"^((?:[A-Z][A-Z'’.\-/]*\s+){1,5}[A-Z][A-Z'’.\-/]*)(?=\s+(?:[A-Z]?[a-z]|\(|\d)|\s*$)")
_OCR_I = re.compile(r"(?<=[^\W\d_])[\]!](?=\s|$)")


def _ocr_fixed(text: str) -> str:
    """OCR reads a final "I" as "]" or "!" ("BINT]"): read it back as a letter.
    The Go grounding check treats these characters as "i" too."""
    return _OCR_I.sub(lambda m: "I" if m.string[: m.start()].split()[-1].isupper() else "i", text)


def _value(text: str) -> str:
    """The name as the profile should show it: no honorifics, letters after
    the name or nickname, ALL CAPS in Title Case."""
    text = _NICKNAME.sub("", _POST_NOMINALS.sub("", _HONORIFIC.sub("", _ocr_fixed(text.strip()))))
    text = re.sub(r"\s+", " ", text).strip(" ,.")
    if text.isupper():
        return " ".join(w if w in {"A/L", "A/P"} else w.capitalize() for w in text.split())
    return text


def _plausible(value: str, supported: bool) -> bool:
    words = value.split()
    if not (2 <= len(words) <= 6 or (len(words) == 1 and supported)) or len(value) > 60:
        return False
    if any(not _WORD.match(w) for w in words) or _ADDRESS.search(value) or ROLE_RE.search(value):
        return False
    if COMPANY_RE.search(value):
        return False
    if all(w.lower().strip(".,") in PLACES for w in words) or section_of(value):
        return False
    # "Dynamic and result-oriented": a capitalised start, then ordinary lowercase words.
    return not (words[0][0].isupper() and any(w.islower() and w.lower() not in _PARTICLES for w in words[1:]))


def _hints(email: str | None, linkedin: str | None, file_name: str | None = None) -> str:
    """The letters of the email's name, the LinkedIn link's and the file's
    ("TUCK WAIFOONG-SEEK-Profile.pdf"): names found in them win."""
    local = (email or "").split("@")[0]
    slug = (linkedin or "").rstrip("/").rsplit("/", 1)[-1]
    stem = re.sub(r"\.[A-Za-z0-9]{1,5}$", "", file_name or "")
    return re.sub(r"[^a-z]", "", f"{local} {slug} {stem}".lower())


def _overlap(value: str, hints: str) -> int:
    words = [re.sub(r"[^a-z]", "", w) for w in value.lower().split() if w not in _PARTICLES]
    return sum(1 for w in words if len(w) >= 3 and w in hints)


def _shapes(text: str) -> list[str]:
    text = _LABEL.sub("", text).strip()
    out = [text, _CONTACT_SPLIT.split(text)[0]]
    if m := _CAPS_RUN.match(text):
        out.append(m.group(1))
    return [s.strip() for s in out if s.strip()]


def name(
    lines: list[Line], email: str | None = None, linkedin: str | None = None, file_name: str | None = None
) -> tuple[str, str] | None:
    hints = _hints(email, linkedin, file_name)
    best: tuple[int, int, str, str] | None = None  # (score, -position, value, quote)
    labelled = next(
        (i for i, x in enumerate(lines[:60]) if re.match(r"^(?:full\s+)?name\s*[:：]?\s*$", x.text, re.I)),
        None,
    )
    for pos, line in enumerate(lines[:40]):
        if line.heading or is_bullet(line.text):
            continue
        for quote in _shapes(line.text):
            value = _value(quote)
            supported = bool(hints) and _overlap(value, hints) > 0
            if not _plausible(value, supported):
                continue
            # Inside skills or work history only with the email's or LinkedIn's backing
            # (a sidebar can put those sections above the name).
            if line.section in ("skills", "experience", "education") and not supported:
                continue
            score = 3 * _overlap(value, hints) + (2 if labelled is not None and pos == labelled + 1 else 0)
            score += 1 if pos < 5 else 0
            if best is None or (score, -pos) > best[:2]:
                best = (score, -pos, value, quote)
            break
    return (best[2], best[3]) if best else None

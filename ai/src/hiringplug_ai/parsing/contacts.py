"""Contact details and personal facts: email, phone, LinkedIn, name, location,
nationality and notice period.

Rules come from Kano's CV scanner, where each was added after a real CV broke
the simpler version (noted next to the rule).
"""

from __future__ import annotations

import re

from .layout import Line

EMAIL_RE = re.compile(r"[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}", re.IGNORECASE)
LINKEDIN_RE = re.compile(
    r"(?:https?://)?(?:[a-z]{2,3}\.)?(?:linkedin\.com/(?:in|pub)/[^\s)|,;]+|lnkd\.in/[^\s)|,;]+)", re.I
)
URL_RE = re.compile(r"(?:https?://|www\.)\S+|\S+\.(?:com|net|org|io|my|sg)/\S*", re.IGNORECASE)
# [ \t] rather than \s: a phone match must not cross a line ("...-123\n0122223333"
# was once read as "123 0122223333"). {6,} so 8-digit Singapore mobiles match.
PHONE_RE = re.compile(r"\+?\(?\d[\d \t().-]{6,}\d")
PHONE_LABEL_RE = re.compile(
    r"(?:tel(?:ephone)?|mobile|hp|h/p|handphone|phone|contact(?:\s*(?:no|number))?|no\.?\s*tel)\b[^\d+]{0,12}",
    re.I,
)
YEAR_RANGE_RE = re.compile(r"\b(?:19|20)\d{2}\s*(?:[-–—]|to)\s*(?:19|20)\d{2}\b", re.IGNORECASE)


def mask(text: str, pattern: re.Pattern[str]) -> str:
    """Blank out matches, keeping positions, so later patterns do not read inside them."""
    return pattern.sub(lambda m: " " * len(m.group(0)), text)


def email(lines: list[Line]) -> tuple[str, str] | None:
    for line in lines:
        if m := EMAIL_RE.search(line.text):
            return m.group(0).lower(), m.group(0)
    return None


def linkedin(lines: list[Line]) -> tuple[str, str] | None:
    for line in lines:
        if m := LINKEDIN_RE.search(line.text):
            quote = m.group(0).rstrip(".")
            url = quote if quote.lower().startswith("http") else "https://" + quote
            return url, quote
    return None


def _phone_ok(candidate: str) -> bool:
    digits = re.sub(r"\D", "", candidate)
    if not 8 <= len(digits) <= 16:
        return False
    if YEAR_RANGE_RE.search(candidate):  # employment years such as 1999-2001
        return False
    # A bare 8-digit run is only a phone if it looks like a Singapore mobile;
    # otherwise it is a reference number or a packed date (20240115).
    return not (len(digits) == 8 and not candidate.strip().startswith("+") and digits[0] not in "89")


_LABEL_ONLY = re.compile(rf"^{PHONE_LABEL_RE.pattern}\s*[:：]?\s*$", re.IGNORECASE)


def phone(lines: list[Line]) -> tuple[str, str] | None:
    """In order: a labelled number near the top ("Mobile: ...", or "Mobile" on
    the line above, as sidebars put it); any number near the top; a labelled
    number further down (a sidebar read after the main column); the first
    international (+) number. Near-top numbers come first so a referee's
    "Phone: ..." at the end never wins over the candidate's own."""
    near: tuple[str, str] | None = None
    labelled_later: tuple[str, str] | None = None
    plus: tuple[str, str] | None = None
    for n, line in enumerate(lines):
        text = mask(mask(mask(line.text, EMAIL_RE), LINKEDIN_RE), URL_RE)
        label_above = n > 0 and bool(_LABEL_ONLY.match(lines[n - 1].text))
        for m in PHONE_RE.finditer(text):
            quote = line.text[m.start() : m.end()].strip(" .-(")
            if not _phone_ok(quote):
                continue
            labelled = bool(PHONE_LABEL_RE.search(text[: m.start()][-30:])) or (
                label_above and m.start() == 0
            )
            if n < 40 and labelled:
                return quote, quote
            if n < 40:
                near = near or (quote, quote)
            elif labelled:
                labelled_later = labelled_later or (quote, quote)
            elif quote.startswith("+"):
                plus = plus or (quote, quote)
    return near or labelled_later or plus


LOCATION_LABEL_RE = re.compile(
    r"^(?:address|location|current\s+location|based\s+in|alamat)\s*[:：]\s*(.+)$", re.I
)
_COUNTRY_TAIL = re.compile(
    r"\b(?:malaysia|singapore|indonesia|philippines|thailand|vietnam|india|brunei|hong kong|australia)\b",
    re.I,
)


def location(lines: list[Line]) -> tuple[str, str] | None:
    for line in lines[:60]:
        if m := LOCATION_LABEL_RE.match(line.text):
            quote = m.group(1).strip()
            return _short_place(quote), quote
    for line in lines[:8]:
        text = line.text
        if len(text) <= 80 and _COUNTRY_TAIL.search(text) and not EMAIL_RE.search(text) and "," in text:
            return _short_place(text), text
    return None


def _short_place(address: str) -> str:
    """The last two parts of an address, without postcodes: "..., 47301 Petaling Jaya, Selangor"."""
    parts = [re.sub(r"\b\d{4,6}\b", "", p).strip(" .") for p in address.split(",")]
    parts = [p for p in parts if p]
    return ", ".join(parts[-2:])[:120] if parts else address[:120]


DEMONYMS = {
    "malaysian": "MY",
    "malaysia": "MY",
    "singaporean": "SG",
    "singapore": "SG",
    "indonesian": "ID",
    "indonesia": "ID",
    "filipino": "PH",
    "philippine": "PH",
    "philippines": "PH",
    "thai": "TH",
    "vietnamese": "VN",
    "indian": "IN",
    "india": "IN",
    "chinese": "CN",
    "china": "CN",
    "bangladeshi": "BD",
    "pakistani": "PK",
    "sri lankan": "LK",
    "myanmar": "MM",
    "burmese": "MM",
    "bruneian": "BN",
    "australian": "AU",
    "british": "GB",
    "american": "US",
    "japanese": "JP",
    "korean": "KR",
    "south korean": "KR",
    "nepali": "NP",
    "nepalese": "NP",
    "taiwanese": "TW",
    "hongkonger": "HK",
}
NATIONALITY_RE = re.compile(
    r"\b(?:nationality|citizenship|warganegara(?:an)?)\s*[:：]?\s*([A-Za-z][A-Za-z ]{2,30})", re.I
)


def nationality(lines: list[Line]) -> tuple[str, str] | None:
    for line in lines:
        if m := NATIONALITY_RE.search(line.text):
            word = m.group(1).strip().lower()
            for demonym, code in sorted(DEMONYMS.items(), key=lambda kv: -len(kv[0])):
                if word.startswith(demonym):
                    return code, line.text[m.start() : m.end()].strip()
    return None


NOTICE_RE = re.compile(
    r"\b(?:notice(?:\s+period)?|availability|available)\b\s*[:：]?\s*(?:of\s+)?"
    r"(?:(?P<n>\d{1,3}|one|two|three|four|six|1/2)\s*(?P<unit>days?|weeks?|months?)|(?P<now>immediate(?:ly)?))",
    re.IGNORECASE,
)
_WORD_NUMBERS = {"one": 1, "two": 2, "three": 3, "four": 4, "six": 6}


def notice_period(lines: list[Line]) -> tuple[str, str] | None:
    for line in lines:
        m = NOTICE_RE.search(line.text)
        if not m:
            continue
        quote = m.group(0).strip()
        if m.group("now"):
            return "0 day", quote
        raw = m.group("n").lower()
        if raw == "1/2":
            continue
        n = int(raw) if raw.isdigit() else _WORD_NUMBERS[raw]
        unit = m.group("unit").lower().rstrip("s")
        return f"{n} {unit}", quote
    return None

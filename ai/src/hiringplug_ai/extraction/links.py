"""Links a file holds behind its text, as Kano reads them.

Many CVs show only the word "LinkedIn" (or an icon) where the address is a
hyperlink: in a PDF a link annotation (/URI), in Word a hyperlink
relationship. Kano adds those addresses to the text it reads ("Embedded PDF
links"), LinkedIn first, so the CV reader finds the profile without an AI.
HiringPlug does the same for PDF and Word files."""

from __future__ import annotations

import io
import re
import zipfile
from typing import Any

HEADING = "Links in the file:"
_ALLOWED = re.compile(r"^(?:https?://|www\.|linkedin\.com/|lnkd\.in/)", re.I)
# Addresses that come with file formats, never a person's.
_FORMAT = re.compile(
    r"^(?:https?://)?(?:www\.)?(?:w3\.org|ns\.adobe\.com|purl\.org|schemas\.openxmlformats\.org|schemas\.microsoft\.com|adobe\.com)\b",
    re.I,
)
_LINKEDIN = re.compile(r"linkedin\.com|lnkd\.in", re.I)
_REL = re.compile(
    r'<Relationship\b[^>]*\bTarget="([^"]+)"[^>]*\bTargetMode="External"|<Relationship\b[^>]*\bTargetMode="External"[^>]*\bTarget="([^"]+)"'
)
MAX_LINKS = 20


def clean(url: str) -> str:
    """A link as a person would write it, or "" for one that is not theirs."""
    u = url.strip().rstrip("\\),>].")
    if not _ALLOWED.match(u) or _FORMAT.match(u):
        return ""
    if not u.lower().startswith("http"):
        u = "https://" + u.lstrip("/")
    return u[:300]


def _ordered(urls: list[str]) -> list[str]:
    seen: dict[str, str] = {}
    for raw in urls:
        u = clean(raw)
        if u and u.lower() not in seen:
            seen[u.lower()] = u
    links = list(seen.values())[:MAX_LINKS]
    links.sort(key=lambda u: 0 if _LINKEDIN.search(u) else 1)  # LinkedIn first, as Kano
    return links


def pdf_links(data: bytes) -> list[str]:
    import pdfplumber  # noqa: PLC0415

    urls: list[str] = []
    with pdfplumber.open(io.BytesIO(data)) as pdf:
        for page in pdf.pages:
            for link in getattr(page, "hyperlinks", None) or []:
                uri: Any = link.get("uri")
                if isinstance(uri, bytes):
                    uri = uri.decode("latin-1", "ignore")
                if isinstance(uri, str):
                    urls.append(uri)
    return _ordered(urls)


def docx_links(data: bytes) -> list[str]:
    urls: list[str] = []
    with zipfile.ZipFile(io.BytesIO(data)) as z:
        for name in z.namelist():
            if name.startswith("word/_rels/") and name.endswith(".rels"):
                xml = z.read(name).decode("utf-8", "ignore")
                urls += [a or b for a, b in _REL.findall(xml)]
    return [u.replace("&amp;", "&") for u in _ordered(urls)]


def with_links(text: str, links: list[str]) -> str:
    """The text with the links it does not already show, under a heading."""
    lower = text.lower()
    missing = [u for u in links if re.sub(r"^https?://(www\.)?", "", u.lower()).rstrip("/") not in lower]
    if not missing:
        return text
    return f"{text.rstrip()}\n\n{HEADING}\n" + "\n".join(missing)

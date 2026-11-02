"""A CV's text as lines with their sections (experience, education, skills, ...).

Section names are matched after tidying (case, punctuation, "&" = "and"), in
English and Malay. A heading may carry content on the same line
("Skills: Excel, SAP"); that content stays in the section.
"""

from __future__ import annotations

import re
from dataclasses import dataclass

SECTION_NAMES: dict[str, tuple[str, ...]] = {
    "experience": (
        "experience",
        "work experience",
        "working experience",
        "professional experience",
        "employment",
        "employment history",
        "employment experience",
        "employment record",
        "career history",
        "career",
        "work history",
        "relevant experience",
        "career summary",
        "professional background",
        "pengalaman",
        "pengalaman kerja",
    ),
    "education": (
        "education",
        "educational background",
        "education background",
        "academic background",
        "academic qualifications",
        "academic qualification",
        "qualifications",
        "academic",
        "education and qualifications",
        "educational qualifications",
        "pendidikan",
        "kelayakan akademik",
    ),
    "skills": (
        "skills",
        "skill",
        "technical skills",
        "key skills",
        "core skills",
        "core competencies",
        "competencies",
        "technical expertise",
        "skills and expertise",
        "areas of expertise",
        "expertise",
        "it skills",
        "computer skills",
        "software skills",
        "tools",
        "technologies",
        "tools and technologies",
        "skills and abilities",
        "skills and competencies",
        "kemahiran",
        "professional skills",
        "technical competencies",
        "skills summary",
    ),
    "other": (
        "summary",
        "profile",
        "professional summary",
        "career objective",
        "objective",
        "about me",
        "personal profile",
        "profile summary",
        "executive summary",
        "certifications",
        "certification",
        "licenses and certifications",
        "certificates",
        "professional certifications",
        "projects",
        "key projects",
        "project experience",
        "languages",
        "language",
        "references",
        "referees",
        "reference",
        "awards",
        "achievements",
        "awards and achievements",
        "publications",
        "training",
        "courses",
        "trainings",
        "interests",
        "hobbies",
        "hobbies and interests",
        "personal details",
        "personal information",
        "personal particulars",
        "personal data",
        "additional information",
        "volunteer experience",
        "volunteering",
        "activities",
        "extracurricular activities",
        "contact",
        "contact details",
        "declaration",
        "memberships",
        "professional memberships",
        "maklumat peribadi",
        "rujukan",
        "bahasa",
    ),
}
# Headings seen on real CVs (JobStreet and LinkedIn exports, designer templates).
SECTION_NAMES["experience"] += (
    "career timeline",
    "employment details",
    "working history",
    "professional history",
    "career experience",
    "work experience and achievements",
    "experience summary",
    "job history",
)
SECTION_NAMES["skills"] += ("skills and qualities", "skills and attributes")
SECTION_NAMES["education"] += ("education and certification", "education and training", "academic profile")
SECTION_NAMES["skills"] += (
    "top skills",
    "core expertise",
    "key competencies",
    "technical proficiencies",
    "skills and tools",
    "technical skills and tools",
)
SECTION_NAMES["other"] += (
    "career overview",
    "key achievements",
    "highlights",
    "achievements highlights",
    "contacts",
    "honors awards",
    "licences and certifications",
    "licenses and certification",
    "current remuneration",
    "remuneration",
    "key strengths",
    "strengths",
    "projects handled",
    "project pool",
)
_LOOKUP = {name: kind for kind, names in SECTION_NAMES.items() for name in names}
# Letter-spaced headings ("P R OFIL E", "CON TA CT") are matched without spaces.
_LOOKUP_JOINED = {name.replace(" ", ""): kind for name, kind in _LOOKUP.items()}
_INLINE_HEADING = re.compile(r"^([A-Za-z &/]{3,40}?)\s*[:：]\s*(\S.*)$")


@dataclass(frozen=True)
class Line:
    text: str  # stripped; always a substring of the CV text
    section: str  # experience, education, skills, other, or "" before the first heading
    heading: bool = False


def heading_key(text: str) -> str:
    t = text.lower().replace("&", " and ")
    t = re.sub(r"[^a-z ]+", " ", t)
    return re.sub(r"\s+", " ", t).strip()


# One-word headings that also end company names and job lines ("... at Dell" +
# "Technologies"): headings only in capitals or with a colon.
_AMBIGUOUS = {
    "technologies",
    "tools",
    "career",
    "employment",
    "expertise",
    "academic",
    "training",
    "activities",
    "highlights",
    "strengths",
    "achievements",
    "interests",
    "competencies",
}


def section_of(text: str) -> str | None:
    """The section a heading line starts, or None if it is not a heading."""
    if len(text) > 45:
        return None
    key = heading_key(text.rstrip(":："))
    if key in _AMBIGUOUS and not (text.isupper() or text.rstrip().endswith((":", "："))):
        return None
    singular = " ".join(w[:-1] if w.endswith("s") and len(w) > 4 else w for w in key.split())
    return _LOOKUP.get(key) or _LOOKUP.get(singular) or _LOOKUP_JOINED.get(key.replace(" ", ""))


# Page furniture that is not part of the CV's content.
_FURNITURE = re.compile(
    r"^(?:created with jobstreet profile|page \d+ of \d+|-?\s*\d{1,3}\s*-?)$", re.IGNORECASE
)
# JobStreet profile exports put a heading and its first line together, without a
# colon: "Career history Senior Engineer at Acme", "Skills Excel, SAP".
_JOBSTREET_INLINE = re.compile(
    r"^(summary|career history|skills|languages|education|licences & certifications|"
    r"personal details|about me)\s+(\S.*)$",
    re.IGNORECASE,
)


def read_lines(text: str) -> list[Line]:
    lines: list[Line] = []
    section = ""
    # JobStreet profile exports: the marker is a page footer (absent from Word copies).
    jobstreet = "created with jobstreet profile" in text.lower() or bool(
        re.search(r"^career history \S", text, re.IGNORECASE | re.MULTILINE)
    )
    for raw in text.splitlines():
        stripped = raw.strip()
        if not stripped or _FURNITURE.match(stripped):
            continue
        if jobstreet and (m := _JOBSTREET_INLINE.match(stripped)) and (kind := section_of(m.group(1))):
            section = kind
            lines.append(Line(m.group(2).strip(), section))
            continue
        kind = section_of(stripped)
        if kind:
            section = kind
            lines.append(Line(stripped, section, heading=True))
            continue
        inline = _INLINE_HEADING.match(stripped)
        if inline and (kind := section_of(inline.group(1))) and kind != "other":
            section = kind
            lines.append(Line(inline.group(2).strip(), section))
            continue
        lines.append(Line(stripped, section))
    return lines


BULLET = re.compile(r"^[\-•*·▪●○■◦➢➤►✓✔–—]+\s*")


def is_bullet(text: str) -> bool:
    return bool(BULLET.match(text))

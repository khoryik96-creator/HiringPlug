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
_LOOKUP = {name: kind for kind, names in SECTION_NAMES.items() for name in names}
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


def section_of(text: str) -> str | None:
    """The section a heading line starts, or None if it is not a heading."""
    if len(text) > 45:
        return None
    return _LOOKUP.get(heading_key(text.rstrip(":：")))


def read_lines(text: str) -> list[Line]:
    lines: list[Line] = []
    section = ""
    for raw in text.splitlines():
        stripped = raw.strip()
        if not stripped:
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

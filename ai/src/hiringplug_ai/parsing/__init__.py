"""Reading profile details out of a CV's text.

Each value comes with the exact CV text it was read from (`quote`). The Go
side keeps only values whose quote it finds in the saved CV text, so any
reader added later (an AI model, a better rule) is checked the same way.
"""

from __future__ import annotations

from ..contracts import ParsedEducation, ParsedJob, ParsedProfile, SourcedText
from . import contacts
from .education import education
from .experience import experience
from .industry import industry
from .layout import read_lines
from .names import name
from .residence import residence_country
from .skills import skills

PARSER = "rules@1"


def _sourced(found: tuple[str, str] | None) -> SourcedText | None:
    if not found or not found[0].strip() or not found[1].strip():
        return None
    value, quote = found
    return SourcedText(value=value.strip()[:300], quote=quote.strip()[:1000])


def _text(value: str | None) -> SourcedText | None:
    return _sourced((value, value)) if value else None


def parse_cv(text: str, file_name: str | None = None) -> ParsedProfile:
    """The details in a CV's text; file_name (often the candidate's name) helps find the name."""
    lines = read_lines(text)
    email, linkedin = contacts.email(lines), contacts.linkedin(lines)
    phone, place = contacts.phone(lines), contacts.location(lines)
    found = experience(lines)
    latest = found[0] if found else None  # CVs list the newest job first
    jobs = [
        ParsedJob(
            title=_text(job.title),
            company=_text(job.company),
            dates=_text(job.dates.quote),
            startDate=job.dates.start,
            endDate=job.dates.end,
            current=job.dates.current,
        )
        for job in found
    ]
    schools = [
        ParsedEducation(
            qualification=_text(s.qualification),
            institution=_text(s.institution),
            dates=_text(s.dates),
            endYear=s.end_year if s.end_year and 1950 <= s.end_year <= 2100 else None,
        )
        for s in education(lines)
    ]
    return ParsedProfile(
        parser=PARSER,
        fullName=_sourced(name(lines, email and email[0], linkedin and linkedin[0], file_name)),
        email=_sourced(email),
        phone=_sourced(phone),
        linkedinUrl=_sourced(linkedin),
        location=_sourced(place),
        nationality=_sourced(contacts.nationality(lines)),
        residenceCountry=_sourced(residence_country(lines, place, phone)),
        industry=_sourced(industry(latest.company, latest.title) if latest else None),
        noticePeriod=_sourced(contacts.notice_period(lines)),
        skills=[s for s in (_sourced(x) for x in skills(lines)) if s],
        experience=jobs,
        education=schools,
        dropped=0,
    )

"""An employer written once over several roles.

"Dell Global Business Center Sdn Bhd, Cyberjaya | May 2010 – Oct 2026" then
"Database Engineer Sr. Advisor   May 2021 – Oct 2026" and earlier roles, each
with its own dates and no employer: the first line is the employer's span,
not a job; the roles under it, inside that span, are at that employer.
"""

from __future__ import annotations

from dataclasses import replace
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .experience import Job


def _within(start: str | None, outer: tuple[str | None, str | None, bool]) -> bool:
    """Whether a role starting at start falls inside an employer's span (YYYY or YYYY-MM)."""
    o_start, o_end, o_current = outer
    if not start or not o_start:
        return False
    if start[: len(o_start)] < o_start[: len(start)]:
        return False
    return o_current or not o_end or start[: len(o_end)] <= o_end[: len(start)]


def with_employers(jobs: list[Job]) -> list[Job]:
    """jobs with each employer's span folded into the untitled roles under it."""
    out: list[Job] = []
    i = 0
    while i < len(jobs):
        head = jobs[i]
        span = (head.dates.start, head.dates.end, head.dates.current)
        roles = []
        j = i + 1
        while (
            j < len(jobs)
            and jobs[j].title
            and jobs[j].company in (None, head.company)  # a role naming the same employer continues the run
            and _within(jobs[j].dates.start, span)
        ):
            roles.append(jobs[j])
            j += 1
        if head.company and not head.title and any(r.company is None for r in roles):
            out.extend(replace(r, company=head.company) for r in roles)
            i = j
            continue
        out.append(head)
        i += 1
    return out

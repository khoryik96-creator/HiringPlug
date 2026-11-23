"""Guardrails for a blind CV, on top of CV Studio's own blind checks.

The blind CV is made from the formatted reading, which CV Studio's rules have
already tidied (titles, dates, sub-headings, one employer per stint). The AI's
anonymised answer can quietly undo that tidying: it rewords a job title, adds
a full stop, flattens a sub-heading into a bullet. CV Studio's restore of the
sub-headings then gives up on that role, as its title no longer matches, and
the rule that joins neighbouring jobs at one employer, run again on "[Company]"
names, joins different employers. So:

- job titles, dates and the last position come back from the reading the AI
  was given (titles are never masked), unless one names an employer or the
  candidate, when the AI's anonymised one stays;
- the jobs are marked to be kept apart, never joined after the names are gone;
- a sub-heading the AI ended with a colon loses it, as in the formatted CV.
"""

from __future__ import annotations

import copy
from typing import Any

# Extra rules for the AI, after CV Studio's blind prompt.
GUARDRAILS = (
    "\n\nSTRUCTURE AND MASKING GUARDRAILS (these override nothing above; they make it exact):\n"
    "- Return every work experience, role and bullet in the same order and the same number. "
    'A bullet object with a "heading" and its own "bullets" stays such an object: never turn its '
    "heading into a bullet, never merge a heading into the bullet after it, never merge or split "
    "roles or work experiences.\n"
    '- Copy every job title (role "title" and candidate "current_position") and every "date_range" '
    "exactly as given, character for character. Titles are never masked.\n"
    '- Use "[Company]" only where an organisation or brand name stood. Never put it in place of '
    'ordinary words ("company codes", "insurance"), tax or business terms (GST, TDS, TCS, VAT), '
    "or a technology or product that does not reveal the employer."
)


def _plain(value: Any) -> str:
    return " ".join(str(value or "").split())


def _names(prepared: dict[str, Any]) -> list[str]:
    """The employers' and the candidate's names, which a title must not show."""
    out: list[str] = []
    candidate = prepared.get("candidate")
    if isinstance(candidate, dict):
        out += [_plain(candidate.get("name")), _plain(candidate.get("current_company"))]
    for exp in prepared.get("work_experiences") or []:
        if isinstance(exp, dict):
            out.append(_plain(exp.get("company")))
    words: list[str] = []
    for name in out:
        words += [w for w in name.replace(",", " ").split() if len(w) >= 3]
    return [n.casefold() for n in out + words if n]


_GENERIC = {
    "ltd",
    "sdn",
    "bhd",
    "inc",
    "llc",
    "plc",
    "group",
    "company",
    "the",
    "and",
    "uae",
    "dubai",
    "malaysia",
    "singapore",
}


def _names_someone(text: str, names: list[str]) -> bool:
    low = f" {text.casefold()} "
    return any(f" {n} " in low or (len(n) > 3 and " " in n and n in low) for n in names if n not in _GENERIC)


def steady(answer: dict[str, Any], prepared: dict[str, Any]) -> dict[str, Any]:
    """The AI's anonymised CV with the given titles and dates kept (see above)."""
    out = copy.deepcopy(answer)
    names = _names(prepared)

    def keep(target: dict[str, Any], source: dict[str, Any], key: str) -> None:
        given = source.get(key)
        if isinstance(given, str) and given.strip() and not _names_someone(given, names):
            target[key] = given

    if isinstance(out.get("candidate"), dict) and isinstance(prepared.get("candidate"), dict):
        keep(out["candidate"], prepared["candidate"], "current_position")
    given_exps, made_exps = prepared.get("work_experiences") or [], out.get("work_experiences") or []
    if (
        not isinstance(given_exps, list)
        or not isinstance(made_exps, list)
        or len(given_exps) != len(made_exps)
    ):
        return out  # not one for one: CV Studio's checks decide
    for given, made in zip(given_exps, made_exps, strict=True):
        if not isinstance(given, dict) or not isinstance(made, dict):
            continue
        keep(made, given, "date_range")
        given_roles, made_roles = given.get("roles") or [], made.get("roles") or []
        if (
            isinstance(given_roles, list)
            and isinstance(made_roles, list)
            and len(given_roles) == len(made_roles)
        ):
            for given_role, made_role in zip(given_roles, made_roles, strict=True):
                if isinstance(given_role, dict) and isinstance(made_role, dict):
                    keep(made_role, given_role, "title")
                    keep(made_role, given_role, "date_range")
    return out


def finish(cv: dict[str, Any]) -> dict[str, Any]:
    """The checked blind CV, ready to render: jobs kept apart, headings tidy."""
    out = copy.deepcopy(cv)
    for exp in out.get("work_experiences") or []:
        if not isinstance(exp, dict):
            continue
        exp["_keep_apart"] = True
        for role in exp.get("roles") or []:
            for item in (role.get("bullets") or []) if isinstance(role, dict) else []:
                if isinstance(item, dict) and isinstance(item.get("heading"), str):
                    item["heading"] = item["heading"].rstrip().rstrip(":").rstrip()
    return out

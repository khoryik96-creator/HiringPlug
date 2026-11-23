"""Guardrails for a blind CV, on top of CV Studio's own blind checks.

The blind CV is made from the formatted reading, which CV Studio's rules have
already tidied (titles, dates, sub-headings, one employer per stint). The AI's
anonymised answer can quietly undo that tidying: it rewords a job title, adds
a full stop, flattens a sub-heading into a bullet. CV Studio's restore of the
sub-headings then gives up on that role, as its title no longer matches, and
the rule that joins neighbouring jobs at one employer, run again on "[Company]"
names, joins different employers. So:

- a job title, date range or the last position the AI changed only in its
  punctuation, capitals or spacing ("Finance Lead." for "Finance Lead") comes
  back as formatted, so the sub-headings can be restored; one whose words the
  AI changed or masked stays as the AI wrote it, so nothing it hid comes back;
- the jobs are marked to be kept apart, never joined after the names are gone;
- a sub-heading the AI ended with a colon loses it, as in the formatted CV.
"""

from __future__ import annotations

import copy
import re
from typing import Any

# Extra rules for the AI, after CV Studio's blind prompt.
GUARDRAILS = (
    "\n\nSTRUCTURE AND MASKING GUARDRAILS (these override nothing above; they make it exact):\n"
    "- Return every work experience, role and bullet in the same order and the same number. "
    'A bullet object with a "heading" and its own "bullets" stays such an object: never turn its '
    "heading into a bullet, never merge a heading into the bullet after it, never merge or split "
    "roles or work experiences.\n"
    '- Copy every job title (role "title" and candidate "current_position") and every "date_range" '
    "exactly as given, character for character, except an organisation, brand, school or person "
    "named in one, which you mask as usual.\n"
    '- Use "[Company]" only where an organisation or brand name stood. Never put it in place of '
    'ordinary words ("company codes", "insurance"), tax or business terms (GST, TDS, TCS, VAT), '
    "or a technology or product that does not reveal the employer."
)


def _words(value: Any) -> list[str]:
    """A text's words, without punctuation or case: "Lead, GST." -> ["lead", "gst"]."""
    return re.findall(r"[^\W_]+", str(value or "").casefold())


def same_words(given: Any, made: Any) -> bool:
    """Whether the AI kept the given text word for word (only punctuation,
    capitals or spacing may differ). Pure: unit tested."""
    words = _words(given)
    return bool(words) and words == _words(made)


def steady(answer: dict[str, Any], prepared: dict[str, Any]) -> dict[str, Any]:
    """The AI's anonymised CV with the given titles and dates kept (see above)."""
    out = copy.deepcopy(answer)

    def keep(target: dict[str, Any], source: dict[str, Any], key: str) -> None:
        given = source.get(key)
        if isinstance(given, str) and same_words(given, target.get(key)):
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


# A mask inside a word: a whole name was never there ("[Company]gital" from
# "Digital", where "Digi" is an employer). A product named after a brand is
# masked before a capital or a digit on purpose ("[Company]Food", "[Company]2u").
_CUT = re.compile(r"(?<=[A-Za-z])\[Company\]|\[Company\](?=[a-z])")


def unglued(made: Any, given: Any) -> Any:
    """The AI's text with each mask that cut a word in two given back its
    word from the given text; whole-name masks stay. Pure: unit tested."""
    if not isinstance(made, str) or not isinstance(given, str) or not _CUT.search(made):
        return made
    parts = re.split(r"(\[Company\])", made)
    cut: list[bool] = []
    pattern = ""
    for i, part in enumerate(parts):
        if part != "[Company]":
            pattern += re.escape(part)
            continue
        inside = (
            parts[i - 1][-1:].isalpha() or parts[i + 1][:1].islower()
            if i + 1 < len(parts)
            else parts[i - 1][-1:].isalpha()
        )
        cut.append(bool(inside))
        pattern += r"([A-Za-z]{1,40}?)" if inside else r"(.+?)"
    m = re.fullmatch(pattern, given, re.DOTALL)
    if not m:
        return made
    out, n = [], 0
    for part in parts:
        if part == "[Company]":
            out.append(m.group(n + 1) if cut[n] else part)
            n += 1
        else:
            out.append(part)
    return "".join(out)


def _unglue_items(made: Any, given: Any) -> Any:
    """Bullets one for one (strings and sub-heading groups)."""
    if isinstance(made, str):
        return unglued(made, given)
    if isinstance(made, list) and isinstance(given, list) and len(made) == len(given):
        return [_unglue_items(m, g) for m, g in zip(made, given, strict=True)]
    if isinstance(made, dict) and isinstance(given, dict):
        out = dict(made)
        for key in ("heading", "title", "bullets"):
            if key in made and key in given:
                out[key] = _unglue_items(made[key], given[key])
        return out
    return made


def unglue(cv: dict[str, Any], prepared: dict[str, Any]) -> dict[str, Any]:
    """The checked blind CV with every word a mask cut in two put back:
    titles, the last position, the summary and the jobs' bullets."""
    out = copy.deepcopy(cv)
    if isinstance(out.get("candidate"), dict) and isinstance(prepared.get("candidate"), dict):
        out["candidate"]["current_position"] = unglued(
            out["candidate"].get("current_position"), prepared["candidate"].get("current_position")
        )
    out["summary_bullets"] = _unglue_items(out.get("summary_bullets"), prepared.get("summary_bullets"))
    given_exps, made_exps = prepared.get("work_experiences") or [], out.get("work_experiences") or []
    if isinstance(given_exps, list) and isinstance(made_exps, list) and len(given_exps) == len(made_exps):
        for made, given in zip(made_exps, given_exps, strict=True):
            if isinstance(made, dict) and isinstance(given, dict):
                made["roles"] = _unglue_items(made.get("roles"), given.get("roles"))
    return out

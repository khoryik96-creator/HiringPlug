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
import functools
import gzip
import re
from importlib import resources
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


# A mask inside a word ("[Company]gital" from "Digital", where "Digi" is an
# employer): the letters stuck to it, before and after.
_CUT = re.compile(r"([A-Za-z]*)\[Company\]([A-Za-z]*)")
# Endings an ordinary word may add to one in the list ("digitalisation").
_ENDINGS = (
    "s",
    "es",
    "ed",
    "d",
    "ing",
    "er",
    "ers",
    "ly",
    "ally",
    "al",
    "ise",
    "ised",
    "ises",
    "ize",
    "ized",
    "izes",
    "isation",
    "isations",
    "ization",
    "izations",
    "ity",
    "ness",
    "ment",
    "ments",
    "ation",
    "ations",
)


@functools.cache
def _english() -> frozenset[str]:
    """Ordinary English words, lowercase (SCOWL, see wordlists/README.md)."""
    data = resources.files("hiringplug_ai").joinpath("wordlists/english_words.txt.gz").read_bytes()
    return frozenset(gzip.decompress(data).decode("ascii").split())


def ordinary_word(word: str) -> bool:
    """Whether a word is plain English ("Digital", "Digitalisation"), not a
    brand ("MyDigi", "Shopeepay", "Grabcar"). Pure: unit tested."""
    low = word.casefold()
    words = _english()
    return low in words or any(
        low.endswith(end) and len(low) - len(end) >= 4 and low[: -len(end)] in words for end in _ENDINGS
    )


def unglued(made: Any, given: Any) -> Any:
    """The AI's text with each mask that cut a word in two given back its
    word, found in the given text: the one word there with the same letters
    around the mask, and only when that word is ordinary English, so a brand
    ("My[Company]" for "MyDigi") stays masked. Whole-name masks stay; the rest
    of the line may be reworded. Pure: unit tested."""
    if not isinstance(made, str) or not isinstance(given, str) or "[Company]" not in made:
        return made
    words: list[str] = re.findall(r"[A-Za-z]+", given)

    def back(m: re.Match[str]) -> str:
        before, after = m.group(1), m.group(2)
        if not before and not after[:1].islower():
            return str(m.group(0))  # a whole name, or a brand product masked on purpose ("[Company]Food")
        found = {
            w
            for w in words
            if len(w) > len(before) + len(after)
            and w.casefold().startswith(before.casefold())
            and w.casefold().endswith(after.casefold())
        }
        if len({w.casefold() for w in found}) != 1:
            return str(m.group(0))  # none, or more than one could be meant
        word = found.pop()
        if not ordinary_word(word):
            return str(m.group(0))
        return before + word[len(before) : len(word) - len(after)] + after

    return _CUT.sub(back, made)


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

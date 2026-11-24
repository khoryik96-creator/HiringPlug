"""How a recruiter reads a job title and a search, before the meaning model
compares them: abbreviations spelt out (TA, AR/AP, HRBP), well-known tools
and qualifications placed (S/4HANA is SAP, ACCA is accounting), a title
split into what the person does (its job family, families.py) and how
senior they are (the level), misleading titles explained, and a search split
into the role asked for and the rest ("headhunter | for finance roles"). The
words are data (words.py).

A search reads the same whatever its case: the app keeps one reading per
search however it was capitalised, so nothing here may depend on the case.

Pure functions: unit tested, and measured on the recruiter benches.
"""

from __future__ import annotations

import re
from dataclasses import dataclass

from .families import AMBIGUOUS, FAMILY_RES, FAMILY_WORDS, WORK_RES
from .words import ABBREV, CODES, LEVELS, MEANS, SKILLS

_ABBREV_LOWER = {k.lower(): v for k, v in ABBREV.items()}
_ABBREV_RE = re.compile(
    r"(?<![\w&/])(" + "|".join(re.escape(k) for k in sorted(ABBREV, key=len, reverse=True)) + r")(?![\w&/])",
    re.IGNORECASE,
)
_SKILLS_RE = re.compile(
    r"(?<![\w/])(" + "|".join(re.escape(k) for k in sorted(SKILLS, key=len, reverse=True)) + r")(?![\w/])",
    re.IGNORECASE,
)
_MEANS_RES = [
    (re.compile(rf"\b{re.escape(k)}\b"), v) for k, v in sorted(MEANS.items(), key=lambda kv: -len(kv[0]))
]
_LEVEL_WORDS = sorted(((w, n) for n, ws in LEVELS.items() for w in ws.split("|")), key=lambda x: -len(x[0]))

# Words that say only the level, or nothing about the field: left out of
# what the person does, unless nothing else is left.
_RANK = re.compile(
    r"\b(senior|junior|principal|lead|chief|head|of|deputy|vice|president|assistant manager|manager|"
    r"executive|"
    r"officer|associate|specialist|coordinator|supervisor|intern|trainee|ii|iii|iv|i|team|staff|grade|"
    r"director|sr|jr)\b",
    re.IGNORECASE,
)

# Abbreviations that are also ordinary words are spelt out only where they
# read as one: "it" as a department ("it executive", "head of it"), never in
# "led a team in it"; "hr", "pa", "ea" and "gm" not after an amount ("rm 60k
# pa", "8 hr shift"); "bi" not in "bi-weekly"; "sem" not a semester.
_DEPT = (
    r"(?:executive|exec|manager|mgr|officer|assistant|asst|support|helpdesk|help desk|service desk|director|"
    r"head|lead|leader|specialist|consultant|analyst|clerk|admin|administration|administrator|generalist|"
    r"partner|business|department|dept|team|intern|internship|trainee|engineer|technician|coordinator|"
    r"supervisor|auditor|audit|security|infrastructure|infra|projects?|programmes?|programs?|operations|ops|"
    r"polic(?:y|ies)|recruiter|recruitment|staff|services?|systems?|solutions?|networks?|hardware|software|"
    r"governance|risk|strategy|transformation|development|training|personnel|advisory|advis[eo]r|vp|"
    r"vice president|associate|representative|sales|marketing|finance|accounts|accounting|payroll|"
    r"procurement|purchasing|legal|compliance|product|consultancy|outsourcing|assets?|vendors?|contracts?)"
)
_DEPT_AFTER = re.compile(rf"^\s*(?:(?:[&/,+]|and|or)\s*)?{_DEPT}\b", re.IGNORECASE)
_DEPT_BEFORE = re.compile(
    r"(?:\b(?:head|director|manager|vp|vice president|chief|lead|senior|junior|sr|jr|snr|assistant|associate|"
    r"principal|regional|group|global|country)\s+(?:of\s+(?:the\s+)?)?|[(,&/–—-]\s*|"
    rf"\b{_DEPT}\s+(?:and|or|&)\s+)$",
    re.IGNORECASE,
)
_AFTER_AMOUNT = re.compile(r"\d\s*[km]?\s*$", re.IGNORECASE)
_SEMESTER = re.compile(r"(?:\b(?:final|last|first|second|this|next)|\d)\s*$", re.IGNORECASE)


def _spelt(text: str, m: re.Match[str]) -> str:
    """The abbreviation found at m, spelt out where it reads as one."""
    key = m.group(1)
    low = key.lower()
    before, after = text[: m.start()], text[m.end() :]
    if low == "it":
        whole = not before.strip() and not after.strip(" .!?")
        ordinary = not (whole or _DEPT_AFTER.match(after) or _DEPT_BEFORE.search(before))
    else:
        ordinary = bool(
            (low in ("hr", "pa", "ea", "gm") and _AFTER_AMOUNT.search(before))
            or (low == "bi" and after.startswith("-"))
            or (low == "sem" and (_SEMESTER.search(before) or re.match(r"\s*\d", after)))
        )
    return key if ordinary else _ABBREV_LOWER[low]


def expand(text: str) -> str:
    """Abbreviations spelt out and tools placed, whatever the case typed:
    "AR/AP Exec" or "ar/ap exec" -> "accounts receivable / accounts payable
    executive"."""
    text = _SKILLS_RE.sub(lambda m: SKILLS[m.group(1).lower()], text)  # before "BI" is spelt out
    text = _ABBREV_RE.sub(lambda m: _spelt(m.string, m), text)
    text = re.sub(r"(?<=[A-Za-z])/(?=[A-Za-z])", " / ", text)
    text = _ABBREV_RE.sub(lambda m: _spelt(m.string, m), text)
    return re.sub(r"\s+", " ", text).strip()


# "Personal Assistant to the CEO": the part after "to" says whom they work
# for, not what they do ("Procure to Pay", "Order to Cash" are processes).
_FOR_WHOM = re.compile(
    r"(?:\s+|\s*[(,–—-]\s*)(?:reporting |reports )?to (?:the )?"
    r"(?!(?:pay|cash|report|retire|contract|order|resolve|deliver|market)\b)",
    re.IGNORECASE,
)


def _role_part(t: str) -> str:
    """A title without whom the person works for: "personal assistant to the
    chief executive officer" -> "personal assistant"."""
    return _FOR_WHOM.split(t, maxsplit=1)[0]


def level(title: str) -> int | None:
    """0 (intern) to 7 (director and above), or None when the title does not
    say; the boss they work for says nothing ("PA to the CEO" is not 7)."""
    t = " " + re.sub(r"[^a-z0-9]+", " ", _role_part(expand(title).lower())) + " "
    best = None
    for word, n in _LEVEL_WORDS:
        k = t.find(f" {word} ")
        if k >= 0:
            best = n if best is None else max(best, n)
            t = t[:k] + " " + "#" * len(word) + " " + t[k + len(word) + 2 :]
    return best


# Words that say how senior: a search for a "headhunter" or a "software
# engineer" names the job, not a level, and any level will do.
_SAID = re.compile(
    r"\b(intern|internship|trainee|apprentice|graduate|junior|assistant|executive|officer|associate|senior|lead|"
    r"principal|supervisor|manager|head|director|vice president|chief|president|controller|clerk)\b"
)


def said_level(title: str) -> int | None:
    """The level a title names with a rank word ("senior", "manager"), or None
    when it names only the job ("headhunter": any level will do)."""
    return level(title) if _SAID.search(_role_part(expand(title).lower())) else None


def field(title: str, explain: bool = True) -> str:
    """What the person does, in words: rank words left out, misleading titles
    explained (not when the work under the title has placed it: an Account
    Executive keeping the books is not explained as sales)."""
    t = re.sub(r"[(),:;|]", " ", _role_part(expand(title).lower()))
    for rx, means in _MEANS_RES if explain else ():
        if rx.search(t):
            t = rx.sub(means, t)
            break
    stripped = re.sub(r"\s+", " ", _RANK.sub(" ", t)).strip(" -&/,")
    return stripped if len(stripped) >= 2 else t.strip()


# Families that say little on their own: what a title adds in brackets says more.
_BROAD = {"hr", "administration"}


def _family_in(t: str) -> tuple[str, str] | None:
    """The family of the title's last field words: in "IT Project Manager"
    the head is project management; IT only says where."""
    best: tuple[tuple[int, int, int], str, str] | None = None
    for k, (name, rx, words) in enumerate(FAMILY_RES):
        for m in rx.finditer(t):
            tail = re.match(r"\w*", t[m.end() :])
            end = m.end() + (len(tail.group(0)) if tail else 0)  # to the end of its word
            key = (end, m.end() - m.start(), -k)
            if best is None or key > best[0]:
                best = (key, name, words)
    return (best[1], best[2]) if best else None


def family(title: str, role_only: bool = False) -> tuple[str, str] | None:
    """(family, what the work is) for a title, or None when the title alone
    cannot say. The part before a comma or bracket is the role; the part
    after says where, used only when the role says nothing ("Assistant
    Manager, Finance") and role_only is false. Whom they work for ("to the
    CEO") never says what they do."""
    t = _role_part(expand(title).lower())
    parts = re.split(r"\s*[(,|:–—]\s*|\s+-\s+", t, maxsplit=1)
    main = parts[0].strip(" )")
    extra = parts[1].strip(" )") if len(parts) > 1 else ""
    if not AMBIGUOUS.match(main):
        found = _family_in(main)
        # A broad family narrowed in brackets: "HR Executive (Recruitment)" recruits.
        if found and found[0] in _BROAD and "(" in t:
            return _family_in(extra) or found
        if found:
            return found
    return _family_in(extra) if extra and not role_only else None


def family_of_work(text: str) -> tuple[str, str] | None:
    """The family the work describes, when one stands out; None otherwise."""
    hits = {name: len(rx.findall(text)) for name, rx in WORK_RES.items()}
    hits = {name: n for name, n in hits.items() if n}
    if not hits:
        return None
    top = sorted(hits.values(), reverse=True)
    if len(top) > 1 and top[0] == top[1]:
        return None
    name = max(hits, key=hits.__getitem__)
    return name, FAMILY_WORDS[name]


def reading(title: str) -> str:
    """A title as the meaning model reads it: its family's work, then its own words."""
    found = family(title)
    words = field(title)
    return f"{found[1]}; {words}" if found else words


def _placed(title: str, work: str) -> tuple[tuple[str, str] | None, bool]:
    """A title's family, and whether the title placed it (not the work under it)."""
    own = family(title, role_only=True)
    if own:
        return own, True
    by_work = family_of_work(work)
    if by_work:
        return by_work, False
    return family(title), True


def work_family(title: str, work: str) -> tuple[str, str] | None:
    """A title's family, from the title, else from the work under it."""
    return _placed(title, work)[0]


def reading_in_work(title: str, work: str) -> str:
    """A title as the meaning model reads it, placed by the work under it
    when the title alone cannot say ("Principal Consultant" placing
    accountants reads as recruitment)."""
    found, by_title = _placed(title, work)
    words = field(title, explain=by_title)
    return f"{found[1]}; {words}" if found else words


def reading_with_work(title: str, work: str) -> str:
    """A title read with what the person did in it."""
    head = reading_in_work(title, work)
    return f"{head}. {work}" if work.strip() else head


def job_reading(title: str, work: str) -> tuple[tuple[str, str] | None, str]:
    """A job's family and its title as the meaning model reads it: placed by
    the title, else by the work its description lists, the way a search is
    placed by the rest of it ("Account Executive" keeping the books is an
    accounts job; selling software, a sales one)."""
    found = family(title)
    if found:
        return found, reading(title)
    by_work = family_of_work(work)
    if by_work:
        return by_work, f"{by_work[1]}; {field(title, explain=False)}"
    return None, reading(title)


_SPLIT = re.compile(
    r"\s+(?:for|in|with|to|at|who|that|from|of the|handling|covering|on)\s+"
    r"|\s+(?=selling\b|managing\b|leading\b)",
    re.IGNORECASE,
)
# A search describes work when it says who or what they did, never for words
# that are part of a title ("Customer Experience Manager", "Managed Services
# Engineer", "LED lighting engineer").
_DESCRIBES = re.compile(
    r"^(?:someone|somebody|a person|person|people|candidates?|anyone)\b"
    r"|\b(?:hired|recruited|prepared|handled|sold|designed|ran|did|worked)\b"  # never in a title
    r"|\b(?:who|that|which)(?: (?:also|has|have|had))? (?:has|have|had|managed|led|built)\b"
    r"|\b(?:has|have|had)(?: (?:also|successfully))? (?:managed|led|built|run|done|experience)\b"
    r"|\bexperienced? (?:in|with|of|as|at|on|from|across|handling|managing|leading|working)\b"
    r"|\b(?:years?|yrs?)(?: of)? (?:experience|exp)\b"
    r"|^(?:\d+\+? ?(?:years?|yrs?)(?: of)? )?experienced?$",
    re.IGNORECASE,
)
# What they did, first ("managed a team of accountants"); a title ends in its job word.
_DID_FIRST = re.compile(r"^(?:managed|led|built|has|have|had)\b", re.IGNORECASE)
_TITLE_END = re.compile(
    r"\b(?:engineer|manager|executive|designer|officer|consultant|analyst|specialist|assistant|director|lead|"
    r"head|clerk|accountant|developer|architect|administrator|technician|coordinator|supervisor|partner|"
    r"associate|representative|planner|buyer|intern|trainee|services|lighting|environment)$",
    re.IGNORECASE,
)


def describes(text: str) -> bool:
    """Whether a search describes work ("someone who hired engineers") rather
    than naming a title ("Customer Experience Manager")."""
    text = text.strip()
    return bool(_DESCRIBES.search(text) or (_DID_FIRST.match(text) and not _TITLE_END.search(text)))


@dataclass(frozen=True)
class Search:
    """A search as a recruiter reads it."""

    role: str  # the role asked for ("" when it describes work)
    context: str  # what else it says ("finance roles")
    describes: bool  # it describes work rather than naming a title
    family: tuple[str, str] | None
    level: int | None
    exact: bool = False  # it names a tool, qualification or code: those who name it count most


def search_reading(s: Search) -> str:
    """The role a search asks for, as the meaning model reads it: placed by
    the rest of the search when the role alone cannot say."""
    words = field(s.role, explain=s.family is None or family(s.role) is not None)
    return f"{s.family[1]}; {words}" if s.family else words


_TOKEN = re.compile(r"[^\s,;:()\[\]{}|\"!?]+")
# Amounts, years and ordinals ("3", "5+", "rm5k", "2nd"), and business models
# ("B2B"): numbers, not codes.
_AMOUNT = re.compile(
    r"^(?:rm|myr|usd|sgd|\$)?\d[\d,.]*(?:st|nd|rd|th|s|k|m|yrs?|years?|mths?|months?|\+)?$|^[bd]2[bc]$"
)


def _code(token: str) -> bool:
    """Whether a word of a search is a tool, qualification or code: a known
    one (SAP, ISO), or one written like a code (C++, .NET, PRINCE2, S/4HANA);
    never a place, a number, an ordinary or a field's word (HR/Admin, FP&A)."""
    low = token.lower().rstrip(".'’")
    if low in CODES:
        return True
    if "/" in low:
        return any(_code(part) for part in low.split("/") if part)
    if "&" in low or low in _ABBREV_LOWER or _AMOUNT.match(low):
        return False
    if re.search(r"[+#]", low) or low.startswith(".") or re.search(r"[a-z]\.[a-z]{2,}", low):
        return True
    return bool(re.search(r"\d", low) and re.search(r"[a-z]", low))


def names_a_skill(text: str) -> bool:
    """Whether a search names a tool, qualification or code, whatever its case."""
    return bool(_SKILLS_RE.search(text)) or any(_code(t) for t in _TOKEN.findall(text))


def read_search(text: str) -> Search:
    """ "headhunter for finance roles" -> role headhunter, context finance
    roles; "someone who hired engineers" describes work (recruitment)."""
    text = text.strip()
    exact = names_a_skill(text)
    if describes(text):
        return Search("", text, True, family_of_work(text), None, exact)
    parts = _SPLIT.split(text, maxsplit=1)
    role = parts[0].strip()
    context = parts[1].strip() if len(parts) > 1 else ""
    # A role that cannot say is placed by the rest ("account executive selling software" sells).
    found = family(role) or (family_of_work(context) if context else None)
    return Search(role, context, False, found, said_level(role), exact)

"""How a recruiter reads a job title and a search, before the meaning model
compares them: abbreviations spelt out (TA, AR/AP, HRBP), well-known tools
and qualifications placed (S/4HANA is SAP, ACCA is accounting), a title
split into what the person does (its job family, families.py) and how
senior they are (the level), misleading titles explained, and a search split
into the role asked for and the rest ("headhunter | for finance roles").

Pure functions: unit tested, and measured on the recruiter benches.
"""

from __future__ import annotations

import re
from dataclasses import dataclass

from .families import AMBIGUOUS, FAMILY_RES, FAMILY_WORDS, WORK_RES

# Abbreviations, spelt out (whole words; upper-case keys match exactly).
ABBREV = {
    "Sr": "senior",
    "Sr.": "senior",
    "Snr": "senior",
    "Jr": "junior",
    "Jr.": "junior",
    "Exec": "executive",
    "Exec.": "executive",
    "Mgr": "manager",
    "Asst": "assistant",
    "Asst.": "assistant",
    "Asst Mgr": "assistant manager",
    "Dept": "department",
    "Eng": "engineer",
    "Engr": "engineer",
    "TA": "talent acquisition",
    "HRBP": "HR business partner",
    "HR": "human resources",
    "AR": "accounts receivable",
    "AP": "accounts payable",
    "GL": "general ledger",
    "BD": "business development",
    "BDM": "business development manager",
    "BDE": "business development executive",
    "KAM": "key account manager",
    "QS": "quantity surveyor",
    "M&E": "mechanical and electrical",
    "SDE": "software development engineer",
    "SWE": "software engineer",
    "SRE": "site reliability engineer",
    "QA": "quality assurance",
    "QC": "quality control",
    "MIS": "management information systems",
    "BI": "business intelligence",
    "FICO": "FI CO finance and controlling",
    "FI/CO": "FI CO finance and controlling",
    "PA": "personal assistant",
    "EA": "executive assistant",
    "CSR": "customer service representative",
    "GM": "general manager",
    "MD": "managing director",
    "CEO": "chief executive officer",
    "CFO": "chief financial officer",
    "CTO": "chief technology officer",
    "COO": "chief operating officer",
    "CHRO": "chief human resources officer",
    "VP": "vice president",
    "AVP": "assistant vice president",
    "SVP": "senior vice president",
    "C&B": "compensation and benefits",
    "L&D": "learning and development",
    "ER": "employee relations",
    "IR": "investor relations",
    "PR": "public relations",
    "UX": "user experience",
    "UI": "user interface",
    "R&D": "research and development",
    "EHS": "environment health and safety",
    "HSE": "health safety and environment",
    "SCM": "supply chain management",
    "S&OP": "sales and operations planning",
    "P2P": "procure to pay",
    "O2C": "order to cash",
    "R2R": "record to report",
    "FP&A": "financial planning and analysis",
    "IT": "information technology",
    "ICT": "information and communications technology",
    "SEO": "search engine optimisation",
    "SEM": "search engine marketing",
    "CRM": "customer relationship management",
}
_ABBREV_RE = re.compile(
    r"(?<![\w&/])(" + "|".join(re.escape(k) for k in sorted(ABBREV, key=len, reverse=True)) + r")(?![\w&/])"
)

# Tools and qualifications a recruiter places at once (a search for one
# finds the field too; their own name still counts most, by its words).
SKILLS = {
    "s/4hana": "SAP S/4HANA ERP",
    "s4hana": "SAP S/4HANA ERP",
    "abap": "SAP ABAP ERP developer",
    "acca": "ACCA accounting qualification",
    "cima": "CIMA management accounting qualification",
    "cpa": "CPA accounting qualification",
    "mfrs": "MFRS accounting standards",
    "ifrs": "IFRS accounting standards",
    "autocount": "AutoCount accounting software",
    "power bi": "Power BI data dashboards",
    "tableau": "Tableau data dashboards",
    "kubernetes": "Kubernetes cloud DevOps",
    "terraform": "Terraform cloud DevOps",
    "hubspot": "HubSpot CRM sales",
    "salesforce": "Salesforce CRM",
    "figma": "Figma design",
    "selenium": "Selenium software test automation",
    "cypress": "Cypress software test automation",
    "haccp": "HACCP food safety quality",
    "gmp": "GMP manufacturing quality",
    "pmp": "PMP project management",
    "cipd": "CIPD human resources",
    "shrm": "SHRM human resources",
    "flutter": "Flutter mobile app developer",
    "react": "React frontend developer",
    "linkedin recruiter": "LinkedIn Recruiter recruitment sourcing",
}
_SKILLS_RE = re.compile(
    r"(?<![\w/])(" + "|".join(re.escape(k) for k in sorted(SKILLS, key=len, reverse=True)) + r")(?![\w/])",
    re.IGNORECASE,
)

# Titles whose words mislead: what the person really does.
MEANS = {
    "account executive": "account executive sales",
    "account manager": "account manager client sales",
    "key account manager": "key account manager client sales",
    "accounts executive": "accounts executive accounting",
    "accounts assistant": "accounts assistant accounting",
    "account assistant": "account assistant accounting",
    "accounts officer": "accounts officer accounting",
    "accounts clerk": "accounts clerk accounting",
    "headhunter": "headhunter recruiter",
    "talent acquisition": "talent acquisition recruitment hiring",
    "talent sourcer": "talent sourcer recruitment",
    "resourcing": "resourcing recruitment",
    "people partner": "people partner human resources business partner",
    "personnel": "personnel human resources",
    "executive assistant": "executive assistant personal assistant",
    "programmer": "programmer software developer",
    "buyer": "buyer purchasing procurement",
    "strategic sourcing": "strategic sourcing procurement",
    "sourcing manager": "sourcing manager procurement",
    "akauntan": "accountant",
    "kerani": "clerk",
    "pegawai": "officer",
    "jurutera": "engineer",
}
_MEANS_RES = [
    (re.compile(rf"\b{re.escape(k)}\b"), v) for k, v in sorted(MEANS.items(), key=lambda kv: -len(kv[0]))
]

# Levels, low to high. The longest words are read first, each once
# ("assistant manager" is 4, not 1 and 5); a title takes the highest it names.
LEVELS = {
    0: "intern|internship|trainee|apprentice|industrial training|student|graduate trainee",
    1: "clerk|assistant|junior|admin|administrative|operator|agent|technician|coordinator|representative|"
    "data entry|account assistant|accounts assistant",
    2: "executive|officer|associate|analyst|engineer|developer|programmer|consultant|specialist|accountant|"
    "recruiter|buyer|planner|designer|surveyor|tester|scientist|auditor|generalist|sourcer|headhunter|"
    "secretary|personal assistant|executive assistant|sales executive|account executive|business partner|"
    "people partner|partner",
    3: "senior|lead|principal|team lead|supervisor|ii|iii|senior executive|senior engineer|senior consultant",
    4: "assistant manager|assistant vice president|team leader|section head|deputy manager",
    5: "manager|controller|head of|head|financial controller",
    6: "senior manager|associate director|general manager|regional manager",
    7: "director|vice president|chief|managing director|country manager|president|executive director|"
    "chief executive officer|chief financial officer|chief technology officer|chief operating officer",
}
LEVEL_NAMES = {
    0: "Intern or trainee",
    1: "Junior or assistant",
    2: "Executive",
    3: "Senior",
    4: "Assistant manager",
    5: "Manager",
    6: "Senior manager",
    7: "Director and above",
}
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


def expand(text: str) -> str:
    """Abbreviations spelt out and tools placed: "AR/AP Exec" -> "accounts
    receivable / accounts payable executive"."""

    def one(m: re.Match[str]) -> str:
        k = m.group(1)
        return ABBREV.get(k, ABBREV.get(k.capitalize(), k))

    text = _SKILLS_RE.sub(lambda m: SKILLS[m.group(1).lower()], text)  # before "BI" is spelt out
    text = _ABBREV_RE.sub(one, text)
    text = re.sub(r"(?<=[A-Za-z])/(?=[A-Za-z])", " / ", text)
    text = _ABBREV_RE.sub(one, text)
    return re.sub(r"\s+", " ", text).strip()


def level(title: str) -> int | None:
    """0 (intern) to 7 (director and above), or None when the title does not say."""
    t = " " + re.sub(r"[^a-z0-9]+", " ", expand(title).lower()) + " "
    best = None
    for word, n in _LEVEL_WORDS:
        k = t.find(f" {word} ")
        if k >= 0:
            best = n if best is None else max(best, n)
            t = t[:k] + " " + "#" * len(word) + " " + t[k + len(word) + 2 :]
    return best


def field(title: str) -> str:
    """What the person does, in words: rank words left out, misleading titles explained."""
    t = re.sub(r"[(),:;|]", " ", expand(title).lower())
    for rx, means in _MEANS_RES:
        if rx.search(t):
            t = rx.sub(means, t)
            break
    stripped = re.sub(r"\s+", " ", _RANK.sub(" ", t)).strip(" -&/,")
    return stripped if len(stripped) >= 2 else t.strip()


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
    Manager, Finance") and role_only is false."""
    t = expand(title).lower()
    parts = re.split(r"\s*[(,|:–—]\s*|\s+-\s+", t, maxsplit=1)
    main = parts[0].strip(" )")
    extra = parts[1].strip(" )") if len(parts) > 1 else ""
    if not AMBIGUOUS.match(main):
        found = _family_in(main)
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


def work_family(title: str, work: str) -> tuple[str, str] | None:
    """A title's family, from the title, else from the work under it."""
    return family(title, role_only=True) or family_of_work(work) or family(title)


def reading_in_work(title: str, work: str) -> str:
    """A title as the meaning model reads it, placed by the work under it
    when the title alone cannot say ("Principal Consultant" placing
    accountants reads as recruitment)."""
    found = work_family(title, work)
    words = field(title)
    return f"{found[1]}; {words}" if found else words


def reading_with_work(title: str, work: str) -> str:
    """A title read with what the person did in it."""
    head = reading_in_work(title, work)
    return f"{head}. {work}" if work.strip() else head


_SPLIT = re.compile(
    r"\s+(?:for|in|with|to|at|who|that|from|of the|handling|covering|on)\s+"
    r"|\s+(?=selling\b|managing\b|leading\b)",
    re.IGNORECASE,
)
_DESCRIBES = re.compile(
    r"^(someone|somebody|a person|person|people|candidates?|anyone)\b|"
    r"\b(hired|recruited|managed|led|built|prepared|handled|sold|designed|ran|did|worked|has|have|experience)\b",
    re.IGNORECASE,
)


@dataclass(frozen=True)
class Search:
    """A search as a recruiter reads it."""

    role: str  # the role asked for ("" when it describes work)
    context: str  # what else it says ("finance roles")
    describes: bool  # it describes work rather than naming a title
    family: tuple[str, str] | None
    level: int | None
    exact: bool = False  # it names a tool, qualification or code: those who name it count most


# A tool, qualification or code: S/4HANA, C++, C#, .NET, ISO 9001, ACCA (not an
# abbreviation spelt out above, such as HR or QA).
_CODE = re.compile(r"(?<!\w)(\.?[A-Za-z]+[/+#.][\w+#.]*|\w*\d\w*|[A-Z]{2,})(?!\w)")


def names_a_skill(text: str) -> bool:
    """Whether a search names a tool, qualification or code."""
    if _SKILLS_RE.search(text):
        return True
    for m in _CODE.finditer(text):
        token = m.group(1).rstrip(".")
        if token in ABBREV or all(part in ABBREV for part in token.split("/")):
            continue  # HR, QA, AR/AP: spelt out, not codes
        return True
    return False


def read_search(text: str) -> Search:
    """ "headhunter for finance roles" -> role headhunter, context finance
    roles; "someone who hired engineers" describes work (recruitment)."""
    text = text.strip()
    exact = names_a_skill(text)
    if _DESCRIBES.search(text):
        return Search("", text, True, family_of_work(text), None, exact)
    parts = _SPLIT.split(text, maxsplit=1)
    role = parts[0].strip()
    context = parts[1].strip() if len(parts) > 1 else ""
    return Search(role, context, False, family(role), level(role), exact)

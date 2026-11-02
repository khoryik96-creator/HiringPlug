"""Words that tell CV lines apart: companies, job titles, places, schools,
durations and sentences. Shared by the work-history and education readers.

Collected from Kano's CV scanner, CV Studio's normaliser and real CVs.
"""

from __future__ import annotations

import re

COMPANY_RE = re.compile(
    r"\b(?:sdn\.?\s*bhd\.?|bhd\.?|berhad|pte\.?\s*ltd\.?|ltd\.?|limited|llc|llp|inc\.?|corp\.?|corporation|plc|"
    r"gmbh|co\.|company|group|bank|holdings?|technolog(?:y|ies)|solutions?|systems|consult(?:ing|ancy)|services|"
    r"industries|enterprise|resources|international|global|partners|associates|capital|logistics|"
    r"hospital|clinic|agency|firm|(?:\(m\)))(?=\W|$)",
    re.IGNORECASE,
)
LEGAL_SUFFIX_RE = re.compile(
    r"\b(?:sdn\.?\s*bhd|bhd|berhad|pte\.?\s*ltd|ltd|llc|inc|corp|plc|gmbh|limited)\b", re.I
)
ROLE_RE = re.compile(
    r"\b(?:engineer|developer|programmer|architect|manager|lead|leader|analyst|consultant|director|specialist|"
    r"executive|administrator|admin|officer|designer|scientist|head|vp|cto|cfo|ceo|coo|cio|accountant|assistant|"
    r"clerk|intern|internship|trainee|supervisor|technician|coordinator|associate|recruiter|auditor|"
    r"representative|advisor|adviser|nurse|teacher|lecturer|tutor|cashier|driver|chef|cook|operator|"
    r"controller|planner|buyer|merchandiser|tester|writer|editor|founder|owner|secretary|dba|devops|sre|"
    r"receptionist|agent|attendant|worker|support|president|chairman|pharmacist|doctor|surgeon|"
    r"therapist|researcher|instructor|trainer|strategist|investigator|inspector|surveyor|draughtsman|"
    r"principal|sme|expert|professional|commercial|sales|marketing)\b",
    re.IGNORECASE,
)
QUALIFICATION_RE = re.compile(
    r"\b(?:bachelor'?s?|degree|ijazah|diploma|master'?s?|mba|phd|ph\.d|doctor(?:ate)?|certificate|"
    r"sijil|spm|stpm|uec|a-levels?|o-levels?|igcse|foundation|asasi|matriculation|matrikulasi|b\.?sc|b\.?a\b|"
    r"b\.?eng|m\.?sc|m\.?eng|b\.?com|bba|llb|acca|cpa|cima|cfa|hnd|advanced?\s+diploma|honou?rs|hons|"
    r"post\s*graduate|pgdit|bsc|msc|be|btech|mtech|mca|bca)\b",
    re.IGNORECASE,
)
INSTITUTION_RE = re.compile(
    r"\b(?:universit(?:y|i|as)|college|kolej|institute|institut|polytechnic|politeknik|school|sekolah|academy|"
    r"akademi|uitm|ukm|usm|upm|utm|utar|unimas|ums|uum|mmu|apu|segi|inti|sunway|taylor'?s|monash|nottingham|"
    r"campus|smk|sjk|nus|ntu|smu|matrikulasi)\b",
    re.IGNORECASE,
)
# Places (CV Studio's location markers, plus cities seen on CVs). A piece made only
# of these is a location, never a job title or a company.
_PLACE_GROUPS = (
    "malaysia malasiya singapore indonesia brunei thailand vietnam philippines india china hong kong",
    "japan korea australia uk usa us united kingdom states saudi arabia uae dubai qatar oman bahrain",
    "kuwait taiwan sri lanka",
    "remote hybrid onsite on-site wfh asia apac asean region",
    "kuala lumpur kl petaling jaya pj shah alam subang klang selangor putrajaya cyberjaya bangsar mont kiara",
    "damansara puchong cheras ampang kajang bangi seremban negeri sembilan melaka malacca johor bahru jb",
    "penang pulau pinang george town ipoh perak kedah alor setar kelantan kota bharu terengganu pahang",
    "kuantan sabah",
    "kinabalu sarawak kuching miri labuan federal territory wp gombak seri kembangan rawang nilai",
    "bangalore bengaluru hyderabad chennai mumbai pune delhi new noida gurgaon gurugram kolkata vizag",
    "jakarta bangkok manila hanoi ho chi minh city riyadh dammam doha sydney melbourne london",
)
PLACES = frozenset(word for group in _PLACE_GROUPS for word in group.split())
_PLACE_FILLER = {"of", "and", "&", "the", "/", "-", "–", "·", "|", "(", ")"}

DURATION_RE = re.compile(
    r"\(?\s*\d+\s*(?:years?|yrs?)(?:,?\s*(?:and\s+)?\d+\s*(?:months?|mos?))?\s*\)?"
    r"|\(?\s*\d+\s*(?:months?|mos?)\s*\)?",
    re.IGNORECASE,
)
_SMALL_WORDS = {
    "of",
    "and",
    "&",
    "the",
    "for",
    "in",
    "at",
    "to",
    "with",
    "on",
    "a",
    "an",
    "de",
    "da",
    "bin",
    "binti",
}


def is_place(text: str) -> bool:
    words = [w for w in re.split(r"[\s,/|·()\-–]+", text.lower().strip(" .")) if w]
    return bool(words) and all(w in PLACES or w in _PLACE_FILLER for w in words)


def is_sentence(text: str) -> bool:
    """Prose (a duty or a summary), not a title or a company name."""
    words = re.findall(r"[^\W\d_][\w'’.-]*", text)
    if len(words) < 5:
        return False
    lower = [w for w in words if w[0].islower() and w.lower() not in _SMALL_WORDS]
    return len(lower) / len(words) >= 0.34


def strip_duration(text: str) -> str:
    return DURATION_RE.sub(" ", text)

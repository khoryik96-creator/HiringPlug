"""Search by meaning, measured the way a recruiter would judge it: made-up
candidates and searches (meaning_bench_a.py, meaning_bench_b.py), read with
the real meaning model and this worker's reading rules, ranked with the same
weights as the Go meaning module (backend/internal/modules/meaning/domain.go:
change both together). Each bench must rank right people first (NDCG@10),
well above a keyword search, and avoid the traps a recruiter would notice.

Needs the meaning model (`task setup` downloads it); set
HP_SKIP_MEANING_TESTS=1 to skip on a machine without it.
"""

from __future__ import annotations

import math
import os
import re
from dataclasses import dataclass
from types import ModuleType

import numpy as np
import pytest

from hiringplug_ai.meaning import model as meaning_model
from hiringplug_ai.meaning.recruiter import (
    expand,
    family,
    level,
    read_search,
    reading,
    reading_in_work,
    reading_with_work,
)

from . import meaning_bench_a, meaning_bench_b

THIS_YEAR = 2026  # the benches' "now"

# The Go meaning module's weights (domain.go).
TITLE_ONLY = (0.7, 0.3)  # role, profile
WITH_CONTEXT = (0.45, 0.35, 0.2)  # role, CV passage on the rest of the search, profile
DESCRIBES = (0.5, 0.5)  # CV passage on the whole search, role (its family)
DESCRIBES_NO_FAMILY = (0.75, 0.25)  # CV passage, profile
JOB = (0.55, 0.25, 0.2)  # role, needs met by the CV, profile
WORDS = 0.06  # the search's own words in the CV
WORDS_EXACT = 0.25  # ... when it names a tool, qualification or code (S/4HANA, ACCA)
LEVEL_STEP, LEVEL_MAX_STEPS = 0.03, 3
OLD_ROLE_STEP, OLD_ROLE_FLOOR = 0.05, 0.55


@pytest.fixture(scope="module")
def model() -> meaning_model.MeaningModel:
    if os.environ.get("HP_SKIP_MEANING_TESTS") == "1":
        pytest.skip("HP_SKIP_MEANING_TESTS=1")
    loaded = meaning_model.load()
    if loaded is None:
        pytest.fail("the meaning model is not installed: run `task setup` (or set HP_SKIP_MEANING_TESTS=1)")
    return loaded


def recency(end: int | None) -> float:
    return 1.0 if end is None else max(OLD_ROLE_FLOOR, 1 - OLD_ROLE_STEP * (THIS_YEAR - end))


def level_fit(want: int | None, have: int | None) -> float:
    if want is None or have is None:
        return 1.0
    return 1 - LEVEL_STEP * min(LEVEL_MAX_STEPS, abs(want - have))


@dataclass
class Read:
    """The bench's candidates as the worker reads them (one passage each: what they did)."""

    ids: list[str]
    roles: list[list[tuple[np.ndarray, np.ndarray, int | None, int | None]]]  # field, work, level, end
    passages: np.ndarray
    profiles: np.ndarray
    texts: list[str]  # for the keyword search


def read_bench(bench: ModuleType, m: meaning_model.MeaningModel) -> Read:
    roles = []
    for _, titles, did, _ in bench.C:
        field_v = m.embed([reading_in_work(t, did if k == 0 else "") for k, (t, _, _) in enumerate(titles)])
        work_v = m.embed([reading_with_work(t, did if k == 0 else "") for k, (t, _, _) in enumerate(titles)])
        roles.append([(field_v[k], work_v[k], level(t), end) for k, (t, _, end) in enumerate(titles)])
    profiles = m.embed([". ".join(t for t, _, _ in titles) + ". " + did for _, titles, did, _ in bench.C])
    texts = [" ".join(t for t, _, _ in titles) + ". " + did for _, titles, did, _ in bench.C]
    return Read([c[0] for c in bench.C], roles, m.embed([c[2] for c in bench.C]), profiles, texts)


def role_score(r: Read, i: int, role: np.ndarray, want: int | None) -> float:
    return max(
        max(float(f @ role), float(w @ role)) * recency(end) * level_fit(want, lvl)
        for f, w, lvl, end in r.roles[i]
    )


WORD = re.compile(r"[a-z0-9&/+#]+")
NOT_KEYWORDS = frozenset(
    {
        "a",
        "an",
        "the",
        "for",
        "in",
        "with",
        "to",
        "at",
        "who",
        "that",
        "of",
        "and",
        "or",
        "someone",
        "somebody",
        "person",
        "people",
        "candidate",
        "candidates",
        "hired",
        "prepared",
        "senior",
        "junior",
        "manager",
        "executive",
        "officer",
        "head",
        "lead",
        "assistant",
        "specialist",
        "role",
        "roles",
        "work",
        "job",
        "jobs",
        "experience",
        "years",
    }
)


def keywords(texts: list[str], query: str) -> np.ndarray:
    """A plain keyword search (BM25), the search's distinctive words only."""
    docs = [WORD.findall(t.lower()) for t in texts]
    words = [w for w in WORD.findall(expand(query).lower()) if w not in NOT_KEYWORDS]
    avg = sum(map(len, docs)) / len(docs)
    out = np.zeros(len(docs))
    for w in set(words):
        df = sum(1 for d in docs if w in d)
        if not df:
            continue
        idf = math.log(1 + (len(docs) - df + 0.5) / (df + 0.5))
        for i, d in enumerate(docs):
            tf = d.count(w)
            out[i] += idf * tf * 2.2 / (tf + 1.2 * (0.25 + 0.75 * len(d) / avg)) if tf else 0
    return out / out.max() if out.max() > 0 else out


def search(r: Read, m: meaning_model.MeaningModel, query: str) -> np.ndarray:
    """Scores for a typed search, as the Go meaning module ranks."""
    s = read_search(query)
    whole = m.embed([expand(query)])[0]
    profile = r.profiles @ whole
    words = (WORDS_EXACT if s.exact else WORDS) * keywords(r.texts, query)
    scores: np.ndarray
    if s.describes and s.family is None:
        a, b = DESCRIBES_NO_FAMILY
        scores = a * (r.passages @ whole) + b * profile
    elif s.describes and s.family is not None:
        role = m.embed([s.family[1]])[0]
        roles = np.array([role_score(r, i, role, None) for i in range(len(r.ids))])
        a, b = DESCRIBES
        scores = a * (r.passages @ whole) + b * roles
    else:
        role = m.embed([reading(s.role)])[0]
        roles = np.array([role_score(r, i, role, s.level) for i in range(len(r.ids))])
        if s.context:
            context = m.embed([expand(s.context)])[0]
            a, b, c = WITH_CONTEXT
            scores = a * roles + b * (r.passages @ context) + c * profile
        else:
            a, b = TITLE_ONLY
            scores = a * roles + b * profile
    return scores + words


def grade(families: list[str], cid: str, wanted: dict[str, int]) -> int:
    best = wanted.get("id:" + cid, 0)
    for f in families:
        for prefix, g in wanted.items():
            if f == prefix or f.startswith(prefix + "."):
                best = max(best, g)
    return best


def ndcg(scores: np.ndarray, grades: list[int], k: int = 10) -> float:
    order = list(np.argsort(-scores, kind="stable"))
    dcg = sum((2 ** grades[i] - 1) / math.log2(n + 2) for n, i in enumerate(order[:k]))
    ideal = sorted(grades, reverse=True)
    best = sum((2**g - 1) / math.log2(n + 2) for n, g in enumerate(ideal[:k]))
    return dcg / best if best else 0.0


def top(r: Read, scores: np.ndarray, n: int) -> list[str]:
    return [r.ids[i] for i in np.argsort(-scores, kind="stable")[:n]]


@pytest.mark.parametrize(("bench", "at_least"), [(meaning_bench_a, 0.92), (meaning_bench_b, 0.89)])
def test_right_people_come_first_far_above_a_keyword_search(
    model: meaning_model.MeaningModel, bench: ModuleType, at_least: float
) -> None:
    r = read_bench(bench, model)
    by_meaning, by_words = [], []
    for query, wanted in bench.Q:
        grades = [grade(c[3], c[0], wanted) for c in bench.C]
        by_meaning.append(ndcg(search(r, model, query), grades))
        by_words.append(ndcg(keywords(r.texts, query), grades))
    meaning, words = float(np.mean(by_meaning)), float(np.mean(by_words))
    assert meaning >= at_least, f"NDCG@10 {meaning:.3f} by meaning (keywords {words:.3f})"
    assert meaning >= words + 0.2, f"meaning {meaning:.3f} is not far above keywords {words:.3f}"


def test_the_traps_a_recruiter_would_notice(model: meaning_model.MeaningModel) -> None:
    a = read_bench(meaning_bench_a, model)
    sales = {"s1", "s2", "s3", "s4", "s5", "s6", "s7"}
    assert not sales & set(top(a, search(a, model, "accounts executive"), 3)), (
        "Account (sales) is not Accounts"
    )
    procurement = {"q1", "q2", "q3", "q4", "q5", "q6"}
    assert not procurement & set(top(a, search(a, model, "talent acquisition"), 5)), "talent, not suppliers"
    recruiters = {"r1", "r2", "r3", "r4", "r5", "r6", "r7", "r8", "r9"}
    found = top(a, search(a, model, "someone who hired software engineers for a tech company"), 3)
    assert set(found) <= recruiters, f"who hired engineers, not engineers: {found}"
    assert set(top(a, search(a, model, "software tester"), 2)) == {"d9", "d10"}
    assert top(a, search(a, model, "S/4HANA"), 1) == ["p1"], "the one who names it first"
    assert set(top(a, search(a, model, "headhunter for finance roles"), 3)) & {"r3", "r4"}, "the finance desk"
    b = read_bench(meaning_bench_b, model)
    assert top(b, search(b, model, "finance business partner"), 1) == ["b4"], "a business partner in finance"
    assert "b24" in top(b, search(b, model, "company secretarial"), 2), "not an executive assistant"
    assert "b35" in top(b, search(b, model, "IT project manager"), 2)


# Jobs: a title and the needs its description lists (bench A's candidates).
JOBS = [
    (
        "Talent Acquisition Manager",
        [
            "Own end-to-end recruitment for technology hiring",
            "Source candidates on LinkedIn and run interviews",
            "Partner hiring managers and negotiate offers",
        ],
        {"rec": 2, "hr": 1},
    ),
    (
        "Accounts Executive",
        [
            "Full set of accounts and month-end closing",
            "Bank reconciliations and SST returns",
            "Liaise with external auditors",
        ],
        {"fin.acc": 2, "fin.mgr": 1, "fin.audit": 1},
    ),
    (
        "Key Account Manager (FMCG)",
        [
            "Manage key retail accounts such as hypermarkets",
            "Negotiate trade terms and promotions",
            "Grow sales and meet targets",
        ],
        {"sales.am": 2, "sales.bd": 1, "sales.ae": 1, "sales.mgr": 1},
    ),
    (
        "Software Engineer (Backend)",
        [
            "Build APIs and microservices",
            "Work with PostgreSQL and Kubernetes",
            "Be on call for production systems",
        ],
        {"sw.backend": 2, "sw.fullstack": 2, "data.eng": 1, "sw": 1},
    ),
    (
        "QA Executive (Food Manufacturing)",
        ["HACCP and GMP compliance", "Supplier and halal audits", "Incoming inspection of raw materials"],
        {"qa.food": 2, "qa.mfg": 1},
    ),
]


def test_possible_candidates_for_a_job_are_found_by_its_title_and_needs(
    model: meaning_model.MeaningModel,
) -> None:
    r = read_bench(meaning_bench_a, model)
    results = []
    for title, needs, wanted in JOBS:
        role = model.embed([reading(title)])[0]
        whole = model.embed([(expand(title) + ". " + " ".join(needs))])[0]
        need_v = model.embed(needs)
        roles = np.array([role_score(r, i, role, level(title)) for i in range(len(r.ids))])
        met = (r.passages @ need_v.T).mean(axis=1)  # each need against their CV
        a, b, c = JOB
        scores = a * roles + b * met + c * (r.profiles @ whole)
        grades = [grade(cand[3], cand[0], wanted) for cand in meaning_bench_a.C]
        results.append(ndcg(scores, grades))
        assert family(title) is not None
    assert float(np.mean(results)) >= 0.85, (
        f"jobs NDCG@10 {np.mean(results):.3f}: {[round(x, 2) for x in results]}"
    )

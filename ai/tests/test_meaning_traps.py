"""The traps a recruiter noticed in search by meaning, measured with the real
meaning model and ranked as the Go meaning module ranks (the mirror in
test_meaning_bench.py): a title that shares a word with another field, a
job titled "Account Executive", the boss a PA works for, a search typed in
lower case, a title with a word that can describe work. Made-up candidates
(meaning_bench_traps.py).

Needs the meaning model, as the benches do; HP_SKIP_MEANING_TESTS=1 skips.
"""

from __future__ import annotations

import os

import pytest

from hiringplug_ai.meaning import model as meaning_model
from hiringplug_ai.meaning.passages import needs
from hiringplug_ai.meaning.recruiter import expand, job_reading, said_level

from . import meaning_bench_traps
from .test_meaning_bench import GOOD, JOB, Read, best_roles, read_bench, search

STRONG, SHOWN = 0.65, 0.38  # what a score says: backend/internal/modules/meaning/domain.go

Traps = tuple[meaning_model.MeaningModel, Read]


@pytest.fixture(scope="module")
def traps() -> Traps:
    if os.environ.get("HP_SKIP_MEANING_TESTS") == "1":
        pytest.skip("HP_SKIP_MEANING_TESTS=1")
    loaded = meaning_model.load()
    if loaded is None:
        pytest.fail("the meaning model is not installed: run `task setup` (or set HP_SKIP_MEANING_TESTS=1)")
    return loaded, read_bench(meaning_bench_traps, loaded)


def scores(traps: Traps, query: str) -> dict[str, float]:
    m, r = traps
    return dict(zip(r.ids, (float(x) for x in search(r, m, query)), strict=True))


@pytest.mark.parametrize(
    ("query", "right", "wrong", "below"),
    [
        ("admin executive", "adm", ["dba", "netadm"], SHOWN),  # a database's administrator is IT
        ("data analyst", "analyst", ["entry"], SHOWN),  # keying data in is not analysing it
        ("oil and gas engineer", "drill", ["palm"], GOOD),  # a palm oil mill is a plantation's
        ("marketing executive", "mkt", ["telco"], SHOWN),  # telecommunications is not communications
        ("logistics manager", "log", ["sdm"], SHOWN),  # an IT service's delivery is not deliveries
        ("CEO", "ceo", ["pa_ceo"], GOOD),  # the CEO a PA works for is not their job
    ],
)
def test_a_title_sharing_a_word_with_another_field_is_not_a_match(
    traps: Traps, query: str, right: str, wrong: list[str], below: float
) -> None:
    got = scores(traps, query)
    assert got[right] >= STRONG, got
    for cid in wrong:
        assert got[cid] < below, (cid, round(got[cid], 3))


def test_a_pa_is_found_as_a_pa_whoever_they_work_for(traps: Traps) -> None:
    got = scores(traps, "personal assistant")
    assert got["pa_ceo"] >= STRONG
    assert got["pa_ceo"] > got["admin_asst"]


def job_scores(traps: Traps, title: str, description: str) -> dict[str, float]:
    """A job's possible candidates, as the Go meaning module scores them,
    from the worker's reading of the job (jobs/meaning_read.py)."""
    m, r = traps
    listed = needs(description)
    found, role = job_reading(title, " ".join(listed))
    whole = m.embed([expand(title) + ". " + " ".join(listed)])[0]
    roles, counts = best_roles(r, m.embed([role])[0], said_level(title), found[0] if found else None)
    met = (r.passages @ m.embed(listed).T).mean(axis=1)
    a, b, c = JOB
    got = (a * roles + b * met + c * (r.profiles @ whole)) * counts
    return dict(zip(r.ids, (float(x) for x in got), strict=True))


def test_an_account_executive_job_is_placed_by_its_duties(traps: Traps) -> None:
    books = (
        "Job Responsibilities:\n- Handle full set of accounts and month-end closing\n"
        "- Prepare bank reconciliation, accruals and management accounts\n"
        "- Liaise with auditors and tax agents\n"
    )
    got = job_scores(traps, "Account Executive", books)
    assert got["acc_ae"] >= STRONG
    assert got["sales_ae"] < GOOD, "a salesperson is at most a possible match for keeping the books"
    selling = (
        "Job Responsibilities:\n- Sell our HR software to SMEs\n"
        "- Prospect and close new business with HR managers\n"
    )
    got = job_scores(traps, "Account Executive", selling)
    assert got["acc_ae"] < GOOD < got["sales_ae"]


def test_a_search_typed_in_lower_case_reads_as_typed_in_capitals(traps: Traps) -> None:
    lower, upper = scores(traps, "ta executive"), scores(traps, "TA Executive")
    assert max(lower, key=lower.__getitem__) == "ta"
    assert lower == pytest.approx(upper)


def test_a_title_with_a_word_that_can_describe_work_finds_who_holds_it(traps: Traps) -> None:
    """Their title alone (no CV), exactly the search: a good match, not work described."""
    assert scores(traps, "Customer Experience Manager")["cx"] >= GOOD

"""The meaning jobs (jobs/meaning_read.py) with a stand-in model that
records what it was asked to read: what goes back to Go, and in what shape.
The real model's judgement is measured in test_meaning_bench.py."""

from __future__ import annotations

from collections.abc import Sequence

import numpy as np

from hiringplug_ai.contracts import (
    JobKind,
    MeaningCandidateSource,
    MeaningCandidatesReadJob,
    MeaningCVSource,
    MeaningQueryReadJob,
    MeaningReadCandidatesJob,
    MeaningReadQueryJob,
    MeaningRoleSource,
)
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import HANDLERS, MEANING_KINDS, Deps
from hiringplug_ai.meaning.model import DIMENSIONS, NAME


class Recorder:
    """Reads every text as the same unit vector, and keeps the texts."""

    def __init__(self) -> None:
        self.read: list[str] = []

    def embed(self, texts: Sequence[str]) -> np.ndarray:
        self.read.extend(texts)
        out = np.zeros((len(texts), DIMENSIONS), dtype=np.float32)
        out[:, 0] = 1
        return out


def deps(model: Recorder | None) -> Deps:
    return Deps(
        download=lambda url, limit: b"",
        ocr=None,
        limits=DEFAULT_LIMITS,
        max_download_bytes=1,
        meaning=lambda: model,  # type: ignore[arg-type,return-value]
    )


CV = (
    "Principal Consultant, Sampel Recruitment (2019 - present)\n"
    "Ran a 360 desk placing accountants and finance managers with MNC clients; 25 placements a year.\n\n"
    "Sales Executive, Contoh Insurance (2016 - 2019)\n"
    "Sold insurance and unit trusts to families; met monthly targets for three years running.\n"
)


def candidates_job() -> MeaningReadCandidatesJob:
    return MeaningReadCandidatesJob(
        candidates=[
            MeaningCandidateSource(
                candidateId="cand_1",
                fingerprint="fp1",
                roles=[
                    MeaningRoleSource(ordinal=0, title="Principal Consultant", current=True, endYear=None),
                    MeaningRoleSource(
                        ordinal=1,
                        title="Sales Executive",
                        current=False,
                        endYear=2019,
                        description="Sold insurance and unit trusts to families.",
                    ),
                ],
                cv=MeaningCVSource(documentId="doc_1", text=CV),
            ),
            MeaningCandidateSource(candidateId="cand_2", fingerprint="fp2", roles=[]),
        ]
    )


def test_each_candidate_is_read_title_by_title_with_their_cv() -> None:
    model = Recorder()
    [answer] = HANDLERS[JobKind.meaning_read_candidates].run(candidates_job(), deps(model))
    assert answer.kind == JobKind.meaning_candidates_read
    assert answer.queue == "core"
    result = answer.payload
    assert isinstance(result, MeaningCandidatesReadJob)
    assert result.model == NAME
    assert not result.unavailable
    first, empty = result.candidates
    assert (first.candidateId, first.fingerprint) == ("cand_1", "fp1")
    consultant, sales = first.roles
    # The title says nothing alone; the CV's opening lines (no lines of its own) place it.
    assert (consultant.title, consultant.family, consultant.level, consultant.endYear) == (
        "Principal Consultant",
        "recruitment",
        3,
        None,
    )
    assert (sales.family, sales.level, sales.endYear) == ("sales", 2, 2019)
    assert len(consultant.field.root) == DIMENSIONS
    assert [p.documentId for p in first.passages] == ["doc_1"] * len(first.passages)
    assert "360 desk" in first.passages[0].passage
    assert first.profile is not None
    # Read as a recruiter would: the family's work, then the title's words, then what they did.
    assert any(t.startswith("recruitment: finding") and "consultant" in t for t in model.read)
    assert any(t.startswith("recruitment: finding") and "360 desk" in t for t in model.read)
    # Someone with nothing to read is answered too, so they are not queued again.
    assert (empty.candidateId, empty.roles, empty.passages, empty.profile) == ("cand_2", [], [], None)


def test_without_the_model_go_is_told_and_nothing_is_read() -> None:
    [answer] = HANDLERS[JobKind.meaning_read_candidates].run(candidates_job(), deps(None))
    assert isinstance(answer.payload, MeaningCandidatesReadJob)
    assert answer.payload.unavailable
    assert answer.payload.candidates == []
    [answer] = HANDLERS[JobKind.meaning_read_query].run(
        MeaningReadQueryJob(queryId="mnq_1", body="TA"), deps(None)
    )
    assert isinstance(answer.payload, MeaningQueryReadJob)
    assert answer.payload.unavailable


def read_query(job: MeaningReadQueryJob) -> tuple[MeaningQueryReadJob, list[str]]:
    model = Recorder()
    [answer] = HANDLERS[JobKind.meaning_read_query].run(job, deps(model))
    assert answer.kind == JobKind.meaning_query_read
    assert answer.queue == "core"
    assert isinstance(answer.payload, MeaningQueryReadJob)
    return answer.payload, model.read


def test_a_search_is_read_as_its_role_and_the_rest() -> None:
    got, texts = read_query(MeaningReadQueryJob(queryId="mnq_1", body="Senior headhunter for finance roles"))
    assert got.queryId == "mnq_1"
    assert not got.describes
    assert got.needs == []
    assert got.role is not None
    assert got.whole is not None
    assert got.context is not None
    assert got.level == 3
    assert got.understood is not None
    assert (got.understood.role, got.understood.family, got.understood.levelName, got.understood.context) == (
        "senior headhunter",
        "recruitment",
        "Senior",
        "finance roles",
    )
    assert texts[0].startswith("recruitment: finding")  # the role, read as its family's work
    assert got.understood.near == ["hr", "hr business partner"]  # also looked at; any other field counts less
    assert not got.exact


def test_a_search_that_describes_work_is_read_by_its_family() -> None:
    got, texts = read_query(MeaningReadQueryJob(queryId="mnq_2", body="someone who hired software engineers"))
    assert got.describes
    assert got.context is None
    assert got.level is None
    assert got.understood is not None
    assert got.understood.family == "recruitment"
    assert texts[0].startswith("recruitment: finding")


def test_a_search_naming_a_tool_says_so() -> None:
    got, _ = read_query(MeaningReadQueryJob(queryId="mnq_3", body="S/4HANA"))
    assert got.exact
    assert got.understood is not None
    assert got.understood.family == "erp"


def test_a_job_is_read_as_its_title_and_the_needs_it_lists() -> None:
    jd = (
        "About us\nWe are a fintech.\n\nRequirements\n- Full set of accounts\n"
        "- Bank reconciliations and SST returns\n\nBenefits\n- Medical insurance\n"
    )
    got, texts = read_query(MeaningReadQueryJob(queryId="mnq_4", title="Accounts Executive", body=jd))
    assert [n.need for n in got.needs] == ["Full set of accounts", "Bank reconciliations and SST returns"]
    assert got.level == 2
    assert got.context is None
    assert not got.describes
    assert got.understood is not None
    assert got.understood.family == "accounting"
    assert got.understood.near is not None
    assert "finance management" in got.understood.near
    assert "insurance" not in " ".join(texts)
    assert "fintech" not in " ".join(texts)


def test_a_job_title_that_cannot_say_is_placed_by_the_work_it_lists() -> None:
    """Many accounts jobs in Malaysia are titled "Account Executive": the
    duties place it, as a candidate's work places their title."""
    books = (
        "Job Responsibilities:\n- Handle full set of accounts and month-end closing\n"
        "- Prepare bank reconciliation, accruals and management accounts\n"
        "- Liaise with auditors and tax agents\n"
    )
    got, texts = read_query(MeaningReadQueryJob(queryId="mnq_6", title="Account Executive", body=books))
    assert got.understood is not None
    assert got.understood.family == "accounting"
    assert got.understood.near is not None
    assert "finance management" in got.understood.near
    assert "sales" not in got.understood.near
    assert texts[0].startswith("accounting:")
    assert "sales" not in texts[0], "keeping the books, not selling"
    selling = "Job Responsibilities:\n- Sell our HR software to SMEs\n- Prospect and close new business\n"
    got, texts = read_query(MeaningReadQueryJob(queryId="mnq_7", title="Account Executive", body=selling))
    assert got.understood is not None
    assert got.understood.family == "sales"
    assert texts[0].startswith("sales:")


def test_a_job_title_naming_only_the_job_asks_for_no_level() -> None:
    job = MeaningReadQueryJob(queryId="mnq_5", title="Software Engineer (Backend)", body="- Go and SQL")
    got, _ = read_query(job)
    assert got.level is None
    assert got.understood is not None
    assert got.understood.levelName is None


def test_the_meaning_slot_takes_searches_first() -> None:
    assert MEANING_KINDS[0] == JobKind.meaning_read_query
    assert set(MEANING_KINDS) == {JobKind.meaning_read_query, JobKind.meaning_read_candidates}

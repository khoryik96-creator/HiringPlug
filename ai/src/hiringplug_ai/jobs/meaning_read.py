"""meaning.read_candidates and meaning.read_query: search by meaning's reading,
with the small meaning model on this server (nothing is sent anywhere).

A candidate is read as a recruiter reads them: each title they held, as its
field ("Accounts Executive": accounting) and with the work they did in it
(a "Principal Consultant" placing accountants is a recruiter); the passages
of their newest CV; and their whole profile. A search is read as the role it
asks for (its field and level) and what else it says; a job, as its title
and the needs its description lists. Go compares and ranks (meaning module).
"""

from __future__ import annotations

import logging
from collections.abc import Callable, Sequence
from typing import TYPE_CHECKING

import numpy as np

from ..contracts import (
    JobKind,
    MeaningCandidateRead,
    MeaningCandidateSource,
    MeaningCandidatesReadJob,
    MeaningNeedRead,
    MeaningPassageRead,
    MeaningQueryReadJob,
    MeaningReadCandidatesJob,
    MeaningReadQueryJob,
    MeaningRoleRead,
    MeaningUnderstood,
    MeaningVector,
)
from ..meaning.families import NEAR
from ..meaning.model import NAME, as_list
from ..meaning.passages import needs, passages
from ..meaning.recruiter import (
    expand,
    job_reading,
    level,
    read_search,
    reading_in_work,
    reading_with_work,
    said_level,
    search_reading,
    work_family,
)
from ..meaning.words import LEVEL_NAMES
from ..queue import FollowUp

if TYPE_CHECKING:
    from . import Deps

log = logging.getLogger(__name__)

RESULT_QUEUE = "core"  # checked against the contract by tests/test_contract_wiring.py
WORK_CHARS = 600  # what a role's own lines say, at most (the model reads about 200 words)
WHOLE_CHARS = 1200


def _vector(v: np.ndarray) -> MeaningVector:
    return MeaningVector(as_list(v))


class _Texts:
    """The texts to read in one go (the model is fastest on a batch)."""

    def __init__(self) -> None:
        self.items: list[str] = []

    def add(self, text: str) -> int:
        self.items.append(text)
        return len(self.items) - 1


def run_candidates(payload: MeaningReadCandidatesJob, deps: Deps) -> list[FollowUp]:
    model = deps.meaning()
    if model is None:
        log.warning("no meaning model on this worker; candidates not read")
        return [
            _answer(
                JobKind.meaning_candidates_read,
                MeaningCandidatesReadJob(model=NAME, unavailable=True, candidates=[]),
            )
        ]
    texts = _Texts()
    plans = [_plan(c, texts) for c in payload.candidates]
    vectors = model.embed(texts.items)
    read = [plan.read(vectors) for plan in plans]
    log.info(
        "candidates read by meaning",
        extra={"candidates": len(read), "texts": len(texts.items)},
    )
    return [_answer(JobKind.meaning_candidates_read, MeaningCandidatesReadJob(model=NAME, candidates=read))]


class _Plan:
    """Where one candidate's texts are in the batch."""

    def __init__(self, source: MeaningCandidateSource) -> None:
        self.source = source
        self.roles: list[tuple[int, str, str | None, int | None, int | None, int, int]] = []
        self.passages: list[tuple[int, str, int]] = []
        self.profile: int | None = None

    def read(self, vectors: np.ndarray) -> MeaningCandidateRead:
        doc = self.source.cv.documentId if self.source.cv else ""
        now = {r.ordinal: r.current for r in self.source.roles}  # a past job with no end year is not now
        return MeaningCandidateRead(
            candidateId=self.source.candidateId,
            fingerprint=self.source.fingerprint,
            profile=_vector(vectors[self.profile]) if self.profile is not None else None,
            roles=[
                MeaningRoleRead(
                    ordinal=ordinal,
                    title=title,
                    family=fam,
                    level=lvl,
                    endYear=end,
                    current=now.get(ordinal, False),
                    field=_vector(vectors[f]),
                    work=_vector(vectors[w]),
                )
                for ordinal, title, fam, lvl, end, f, w in self.roles
            ],
            passages=[
                MeaningPassageRead(ordinal=i, documentId=doc, passage=text, vec=_vector(vectors[k]))
                for i, text, k in self.passages
            ],
        )


def _plan(source: MeaningCandidateSource, texts: _Texts) -> _Plan:
    plan = _Plan(source)
    cv = passages(source.cv.text) if source.cv else []
    roles = sorted(source.roles, key=lambda r: r.ordinal)
    for role in roles:
        work = (role.description or "").strip()[:WORK_CHARS]
        if not work and role is roles[0] and cv:
            work = cv[0][:WORK_CHARS]  # the newest role, read with the CV's opening lines
        found = work_family(role.title, work)
        plan.roles.append(
            (
                role.ordinal,
                role.title,
                found[0] if found else None,
                level(role.title),
                None if role.current else role.endYear,
                texts.add(reading_in_work(role.title, work)),
                texts.add(reading_with_work(role.title, work)),
            )
        )
    for i, text in enumerate(cv):
        plan.passages.append((i, text, texts.add(text)))
    titles = ". ".join(dict.fromkeys(r.title for r in roles[:6]))
    profile = ". ".join(t for t in (titles, cv[0] if cv else "") if t)
    if profile:
        plan.profile = texts.add(profile[:WHOLE_CHARS])
    return plan


def run_query(payload: MeaningReadQueryJob, deps: Deps) -> list[FollowUp]:
    model = deps.meaning()
    if model is None:
        log.warning("no meaning model on this worker; search not read")
        return [
            _answer(
                JobKind.meaning_query_read,
                MeaningQueryReadJob(
                    queryId=payload.queryId,
                    model=NAME,
                    unavailable=True,
                    describes=False,
                    needs=[],
                ),
            )
        ]
    if payload.title and payload.title.strip():
        return [_answer(JobKind.meaning_query_read, _read_job(payload, payload.title.strip(), model.embed))]
    search = read_search(payload.body)
    texts = _Texts()
    role = None
    if search.describes:
        role = texts.add(search.family[1]) if search.family else None
    elif search.role:
        role = texts.add(search_reading(search))
    whole = texts.add(expand(payload.body)[:WHOLE_CHARS])
    # What else the search says ("for finance roles"); a search that describes work is all context.
    context = texts.add(expand(search.context)) if search.context and not search.describes else None
    vectors = model.embed(texts.items)
    lvl = search.level
    understood = MeaningUnderstood(
        role=expand(search.role).lower() or None,
        family=search.family[0] if search.family else None,
        near=_near(search.family),
        level=lvl,
        levelName=LEVEL_NAMES.get(lvl) if lvl is not None else None,
        context=(search.context or None) if not search.describes else None,
        describes=search.describes,
    )
    return [
        _answer(
            JobKind.meaning_query_read,
            MeaningQueryReadJob(
                queryId=payload.queryId,
                model=NAME,
                role=_vector(vectors[role]) if role is not None else None,
                whole=_vector(vectors[whole]),
                context=_vector(vectors[context]) if context is not None else None,
                level=lvl,
                describes=search.describes,
                exact=search.exact,
                understood=understood,
                needs=[],
            ),
        )
    ]


def _read_job(
    payload: MeaningReadQueryJob, title: str, embed: Callable[[Sequence[str]], np.ndarray]
) -> MeaningQueryReadJob:
    """A job: its title is the role (placed by the work it lists when the
    title alone cannot say: an "Account Executive" keeping the books is an
    accounts job); the needs its description lists are each compared with CV
    passages; its title and needs together are the whole."""
    listed = needs(payload.body)
    found, role = job_reading(title, " ".join(listed) or payload.body)
    whole = (expand(title) + ". " + (" ".join(listed) or payload.body))[:WHOLE_CHARS]
    vectors = embed([role, whole, *listed])
    lvl = said_level(title)
    return MeaningQueryReadJob(
        queryId=payload.queryId,
        model=NAME,
        role=_vector(vectors[0]),
        whole=_vector(vectors[1]),
        context=None,
        level=lvl,
        describes=False,
        understood=MeaningUnderstood(
            role=expand(title).lower(),
            family=found[0] if found else None,
            near=_near(found),
            level=lvl,
            levelName=LEVEL_NAMES.get(lvl) if lvl is not None else None,
            describes=False,
        ),
        needs=[MeaningNeedRead(need=n, vec=_vector(vectors[2 + i])) for i, n in enumerate(listed)],
    )


def _near(found: tuple[str, str] | None) -> list[str] | None:
    """The families a recruiter also looks at for this one (a candidate in
    any other counts for less: the Go meaning module)."""
    return sorted(NEAR.get(found[0], ())) if found else None


def _answer(kind: JobKind, payload: MeaningCandidatesReadJob | MeaningQueryReadJob) -> FollowUp:
    return FollowUp(queue=RESULT_QUEUE, kind=kind, payload=payload)

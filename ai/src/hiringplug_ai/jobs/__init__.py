"""Handlers for jobs on the `ai` queue: one module per job kind.

A handler receives the job's payload (already validated against the
contract) and returns the follow-up jobs to queue, typically a result job for
the Go worker. Returning instead of writing keeps handlers free of database
code and easy to test.
"""

from __future__ import annotations

import dataclasses
from collections.abc import Callable
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any

from pydantic import BaseModel

from ..contracts import (
    AiAccess,
    AiProviderCheckJob,
    AiProviderReportedJob,
    AssistantAskJob,
    ContactReadLinkedinJob,
    CvFormatStudioJob,
    DocumentExtractTextJob,
    DocumentParseCvJob,
    DocumentRenderPdfJob,
    JdBlindJob,
    JobKind,
    JobReadJdJob,
    MatchScoreJob,
    TalentRankJob,
)
from ..extraction import Limits, OcrEngine
from ..providers import Completion, Provider, ProviderError
from ..queue import FollowUp
from . import (
    ai_check,
    assistant_ask,
    blind_jd,
    extract_text,
    format_studio,
    match_score,
    parse_cv,
    read_jd,
    read_linkedin,
    render_pdf,
    talent_rank,
)


@dataclass
class ProviderLog:
    """What the AI provider did on a job's calls. The worker tells the app the
    last one (it answered, or why not), so people see a notice when the AI is
    down instead of wondering why nothing happens."""

    provider: str = ""
    ok: bool | None = None  # None: the job did not call the provider
    error: str | None = None  # safe to show (a ProviderError's text)

    def note(self, provider: str, error: str | None) -> None:
        self.provider, self.ok, self.error = provider, error is None, error

    def report(self) -> FollowUp | None:
        """The job telling the app, or None when the provider was not called."""
        if self.ok is None:
            return None
        return FollowUp(
            queue="core",  # checked against the contract by tests/test_contract_wiring.py
            kind=JobKind.aiprovider_reported,
            payload=AiProviderReportedJob(
                provider=self.provider,
                ok=self.ok,
                error=(self.error or "")[:500] or None,
                at=datetime.now(UTC),
            ),
        )


@dataclass(frozen=True)
class _Watched:
    """A provider whose every call is noted in the job's ProviderLog."""

    inner: Provider
    name: str
    log: ProviderLog

    def complete(self, system: str, user: str, *, max_tokens: int) -> Completion:
        try:
            done = self.inner.complete(system, user, max_tokens=max_tokens)
        except ProviderError as exc:
            self.log.note(self.name, str(exc))
            raise
        self.log.note(self.name, None)
        return done


@dataclass(frozen=True)
class Deps:
    """What handlers may use; tests pass fakes."""

    download: Callable[[str, int], bytes]
    ocr: OcrEngine | None
    limits: Limits
    max_download_bytes: int
    ai: Provider | None = None  # the server's own provider (HP_AI_*); None: off
    # Builds the provider a company chose in Settings (opens its sealed key).
    connect: Callable[[AiAccess], Provider] | None = None
    # How long one long AI answer may take (HP_AI_LONG_TIMEOUT_SECONDS).
    long_timeout_seconds: float | None = None
    # Keeps this job's claim before a long step (the worker sets it per job);
    # raises LeaseLostError when another worker took the job.
    keep_alive: Callable[[], None] = lambda: None
    # Where this job's provider calls are noted (the worker sets it per job).
    log: ProviderLog | None = None

    def _chosen(self, access: AiAccess | None) -> Provider | None:
        if access is None:
            return self.ai
        if self.connect is None:
            raise ProviderError("the AI worker cannot open saved AI keys (HP_AI_KEYS_SECRET is not set)")
        return self.connect(access)

    def provider(self, access: AiAccess | None, *, long: bool = False) -> Provider | None:
        """The provider a job was queued for: the company's choice, or the
        server's own when it chose none; `long` for answers that take minutes
        (a whole CV as JSON). Raises ProviderError when the company's key
        cannot be opened."""
        name = access.provider.value if access is not None else "server"
        try:
            chosen = self._chosen(access)
        except ProviderError as exc:
            if self.log is not None:
                self.log.note(name, str(exc))
            raise
        if (
            long
            and self.long_timeout_seconds
            and dataclasses.is_dataclass(chosen)
            and hasattr(chosen, "timeout_seconds")
        ):
            chosen = dataclasses.replace(chosen, timeout_seconds=self.long_timeout_seconds)
        if chosen is not None and self.log is not None:
            return _Watched(chosen, name, self.log)
        return chosen


@dataclass(frozen=True)
class Handler:
    payload_model: type[BaseModel]
    run: Callable[[Any, Deps], list[FollowUp]]


HANDLERS: dict[JobKind, Handler] = {
    JobKind.document_extract_text: Handler(DocumentExtractTextJob, extract_text.run),
    JobKind.document_parse_cv: Handler(DocumentParseCvJob, parse_cv.run),
    JobKind.document_render_pdf: Handler(DocumentRenderPdfJob, render_pdf.run),
    JobKind.assistant_ask: Handler(AssistantAskJob, assistant_ask.run),
    JobKind.match_score: Handler(MatchScoreJob, match_score.run),
    JobKind.talent_rank: Handler(TalentRankJob, talent_rank.run),
    JobKind.jd_blind: Handler(JdBlindJob, blind_jd.run),
    JobKind.job_read_jd: Handler(JobReadJdJob, read_jd.run),
    JobKind.aiprovider_check: Handler(AiProviderCheckJob, ai_check.run),
    JobKind.contact_read_linkedin: Handler(ContactReadLinkedinJob, read_linkedin.run),
    JobKind.cv_format_studio: Handler(CvFormatStudioJob, format_studio.run),
}

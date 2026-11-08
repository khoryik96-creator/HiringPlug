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
from typing import Any

from pydantic import BaseModel

from ..contracts import (
    AiAccess,
    AiProviderCheckJob,
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
)
from ..extraction import Limits, OcrEngine
from ..providers import Provider, ProviderError
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
)


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

    def provider(self, access: AiAccess | None, *, long: bool = False) -> Provider | None:
        """The provider a job was queued for: the company's choice, or the
        server's own when it chose none; `long` for answers that take minutes
        (a whole CV as JSON). Raises ProviderError when the company's key
        cannot be opened."""
        if access is None:
            chosen = self.ai
        elif self.connect is None:
            raise ProviderError("the AI worker cannot open saved AI keys (HP_AI_KEYS_SECRET is not set)")
        else:
            chosen = self.connect(access)
        if (
            long
            and self.long_timeout_seconds
            and dataclasses.is_dataclass(chosen)
            and hasattr(chosen, "timeout_seconds")
        ):
            return dataclasses.replace(chosen, timeout_seconds=self.long_timeout_seconds)
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
    JobKind.jd_blind: Handler(JdBlindJob, blind_jd.run),
    JobKind.job_read_jd: Handler(JobReadJdJob, read_jd.run),
    JobKind.aiprovider_check: Handler(AiProviderCheckJob, ai_check.run),
    JobKind.contact_read_linkedin: Handler(ContactReadLinkedinJob, read_linkedin.run),
    JobKind.cv_format_studio: Handler(CvFormatStudioJob, format_studio.run),
}

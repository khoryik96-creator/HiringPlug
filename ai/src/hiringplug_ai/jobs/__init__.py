"""Handlers for jobs on the `ai` queue: one module per job kind.

A handler receives the job's payload (already validated against the
contract) and returns the follow-up jobs to queue, typically a result job for
the Go worker. Returning instead of writing keeps handlers free of database
code and easy to test.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from pydantic import BaseModel

from ..contracts import (
    AssistantAskJob,
    DocumentExtractTextJob,
    DocumentParseCvJob,
    DocumentRenderPdfJob,
    JobKind,
)
from ..extraction import Limits, OcrEngine
from ..provider import Provider
from ..queue import FollowUp
from . import assistant_ask, extract_text, parse_cv, render_pdf


@dataclass(frozen=True)
class Deps:
    """What handlers may use; tests pass fakes."""

    download: Callable[[str, int], bytes]
    ocr: OcrEngine | None
    limits: Limits
    max_download_bytes: int
    ai: Provider | None = None  # None: the AI provider is off


@dataclass(frozen=True)
class Handler:
    payload_model: type[BaseModel]
    run: Callable[[Any, Deps], list[FollowUp]]


HANDLERS: dict[JobKind, Handler] = {
    JobKind.document_extract_text: Handler(DocumentExtractTextJob, extract_text.run),
    JobKind.document_parse_cv: Handler(DocumentParseCvJob, parse_cv.run),
    JobKind.document_render_pdf: Handler(DocumentRenderPdfJob, render_pdf.run),
    JobKind.assistant_ask: Handler(AssistantAskJob, assistant_ask.run),
}

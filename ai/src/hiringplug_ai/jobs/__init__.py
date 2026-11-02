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

from ..contracts import DocumentExtractTextJob, DocumentParseCvJob, JobKind
from ..extraction import Limits, OcrEngine
from ..queue import FollowUp
from . import extract_text, parse_cv


@dataclass(frozen=True)
class Deps:
    """What handlers may use; tests pass fakes."""

    download: Callable[[str, int], bytes]
    ocr: OcrEngine | None
    limits: Limits
    max_download_bytes: int


@dataclass(frozen=True)
class Handler:
    payload_model: type[BaseModel]
    run: Callable[[Any, Deps], list[FollowUp]]


HANDLERS: dict[JobKind, Handler] = {
    JobKind.document_extract_text: Handler(DocumentExtractTextJob, extract_text.run),
    JobKind.document_parse_cv: Handler(DocumentParseCvJob, parse_cv.run),
}

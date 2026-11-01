"""The outcome of reading one document."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Literal

Status = Literal["extracted", "needs_ocr", "failed"]


@dataclass(frozen=True)
class Extraction:
    status: Status
    text: str | None
    page_count: int | None
    extractor: str
    warnings: list[str] = field(default_factory=list)
    error: str | None = None

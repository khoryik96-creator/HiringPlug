from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

import pytest

from hiringplug_ai.contracts import DocumentTextExtractedJob, JobKind
from hiringplug_ai.extraction import DEFAULT_LIMITS
from hiringplug_ai.jobs import Deps
from hiringplug_ai.queue import ClaimedJob, FollowUp, LeaseLostError
from hiringplug_ai.worker import process

from .documents import CV_LINES, text_pdf

DOC_ID = "doc_01jbz6w0m0k8q9x2r3s4t5v6w7"


def job(kind: str = "document.extract_text", payload: dict[str, Any] | None = None) -> ClaimedJob:
    return ClaimedJob(
        id="job_01jbz6w0m0k8q9x2r3s4t5v6w7",
        tenant_id="tnt_01jbz6w0m0k8q9x2r3s4t5v6w7",
        kind=kind,
        payload=payload
        if payload is not None
        else {
            "documentId": DOC_ID,
            "downloadUrl": "http://api.test/internal/files/x?sig=1",
            "contentType": "application/pdf",
            "fileName": "cv.pdf",
            "sizeBytes": 100,
        },
        attempts=1,
        max_attempts=5,
        request_id="req_test",
    )


@dataclass
class FakeQueue:
    completed: list[list[FollowUp]] = field(default_factory=list)
    failed: list[str] = field(default_factory=list)
    lose_lease: bool = False

    def complete(self, job: ClaimedJob, follow_ups: list[FollowUp]) -> None:
        if self.lose_lease:
            raise LeaseLostError(job.id)
        self.completed.append(follow_ups)

    def fail(self, job: ClaimedJob, error: str) -> str:
        self.failed.append(error)
        return "pending"


def deps(data: bytes = b"", error: Exception | None = None) -> Deps:
    def download(url: str, max_bytes: int) -> bytes:
        if error:
            raise error
        return data

    return Deps(download=download, ocr=None, limits=DEFAULT_LIMITS, max_download_bytes=10_000_000)


def test_extract_text_sends_the_text_back_to_the_go_worker() -> None:
    queue = FakeQueue()
    process(queue, job(), deps(text_pdf(CV_LINES)))  # type: ignore[arg-type]
    assert queue.failed == []
    [[follow_up]] = queue.completed
    assert follow_up.queue == "core"
    assert follow_up.kind == JobKind.document_text_extracted
    result = follow_up.payload
    assert isinstance(result, DocumentTextExtractedJob)
    assert result.status.value == "extracted"
    assert result.text is not None
    assert "Nur Aisyah binti Rahman" in result.text
    # The result must satisfy the contract the Go side validates against.
    DocumentTextExtractedJob.model_validate(result.model_dump(mode="json"))


def test_an_unreadable_document_is_a_result_not_a_retry() -> None:
    queue = FakeQueue()
    process(queue, job(), deps(b"not a document"))  # type: ignore[arg-type]
    assert queue.failed == [], "retrying cannot fix a bad file"
    result = queue.completed[0][0].payload
    assert isinstance(result, DocumentTextExtractedJob)
    assert result.status.value == "failed"
    assert result.errorMessage == "Only PDF and Word (.docx) files can be read."


@pytest.mark.parametrize(
    ("the_job", "the_deps", "message"),
    [
        (job(), deps(error=RuntimeError("network down")), "RuntimeError: network down"),
        (job(payload={"documentId": "nope"}), deps(), "ValidationError"),
        (job(kind="made.up"), deps(), "ValueError"),
    ],
)
def test_failures_are_recorded_on_the_job(the_job: ClaimedJob, the_deps: Deps, message: str) -> None:
    queue = FakeQueue()
    process(queue, the_job, the_deps)  # type: ignore[arg-type]
    assert queue.completed == []
    assert len(queue.failed) == 1
    assert message in queue.failed[0]


def test_a_lost_lease_discards_the_result_without_recording_a_failure() -> None:
    queue = FakeQueue(lose_lease=True)
    process(queue, job(), deps(text_pdf(CV_LINES)))  # type: ignore[arg-type]
    assert queue.failed == []

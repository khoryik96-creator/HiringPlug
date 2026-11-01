"""End to end, across real processes: Go API -> job queue -> Python worker ->
Go worker -> API. Run by `task e2e`, which starts all three services.

Opt-in (marker `e2e`) because it needs the running services; `task e2e` and CI
run it explicitly.
"""

from __future__ import annotations

import os
import time

import httpx
import pytest

from ..documents import CV_LINES, join_pdfs, scanned_pdf, text_pdf

pytestmark = pytest.mark.e2e


@pytest.fixture(scope="module")
def api() -> httpx.Client:
    base = os.environ.get("HP_E2E_API_URL", "http://localhost:8080")
    token = os.environ.get("HP_E2E_TOKEN", "")
    if not token:
        pytest.fail("HP_E2E_TOKEN is required (a recruiter token from `hp dev-setup`)")
    return httpx.Client(base_url=base, headers={"Authorization": f"Bearer {token}"}, timeout=30)


def wait_for_extraction(api: httpx.Client, document_id: str, timeout: float = 90) -> dict[str, object]:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        doc: dict[str, object] = api.get(f"/v1/documents/{document_id}").json()
        if doc["status"] != "queued":
            return doc
        time.sleep(0.5)
    pytest.fail(f"document {document_id} was still queued after {timeout}s")


def test_a_cv_is_uploaded_read_and_returned(api: httpx.Client) -> None:
    created = api.post("/v1/candidates", json={"firstName": "E2E", "lastName": f"Test {time.time_ns()}"})
    assert created.status_code == 201, created.text
    candidate_id = created.json()["id"]

    # Page 1 has a text layer; page 2 is a scan that needs OCR.
    pdf = join_pdfs(text_pdf(CV_LINES[:2]), scanned_pdf(text_pdf(CV_LINES[2:], font_size=14)))
    upload = api.post(
        f"/v1/candidates/{candidate_id}/documents",
        data={"source": "original"},
        files={"file": ("cv.pdf", pdf, "application/pdf")},
    )
    assert upload.status_code == 201, upload.text
    assert upload.json()["status"] == "queued"

    doc = wait_for_extraction(api, str(upload.json()["id"]))
    assert doc["status"] == "extracted", doc
    text = str(doc["extractedText"])
    assert "Nur Aisyah binti Rahman" in text  # from the text layer
    assert "Deloitte" in text  # from OCR
    assert "tesseract" in str(doc["extractor"])
    assert doc["pageCount"] == 2

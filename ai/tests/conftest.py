"""Pytest setup: dependency checks that fail loudly instead of skipping silently."""

from __future__ import annotations

import os
import shutil

import pytest

from .documents import CV_LINES, text_pdf


def pytest_runtest_setup(item: pytest.Item) -> None:
    # Missing dependencies fail loudly; skipping must be asked for explicitly.
    if item.get_closest_marker("ocr") and not shutil.which("tesseract"):
        if os.environ.get("HP_SKIP_OCR_TESTS") == "1":
            pytest.skip("HP_SKIP_OCR_TESTS=1")
        pytest.fail(
            "OCR tests need Tesseract (apt install tesseract-ocr / brew install tesseract); "
            "or set HP_SKIP_OCR_TESTS=1"
        )


@pytest.fixture
def cv_text_pdf() -> bytes:
    return text_pdf(CV_LINES)

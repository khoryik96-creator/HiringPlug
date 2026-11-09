from __future__ import annotations

from dataclasses import dataclass, field

import pytest

from hiringplug_ai.extraction import TesseractOcr, extract
from hiringplug_ai.extraction.limits import Limits
from hiringplug_ai.extraction.ocr import OcrPages
from hiringplug_ai.extraction.pdf import _Page, choose_page_text

from .documents import CV_LINES, join_pdfs, scanned_pdf, text_pdf


@dataclass
class FakeOcr:
    """Records which pages were sent to OCR and returns canned text."""

    texts: dict[int, str] = field(default_factory=dict)
    blank: set[int] = field(default_factory=set)
    installed: bool = True
    calls: list[list[int]] = field(default_factory=list)

    @property
    def name(self) -> str:
        return "fake-ocr@1"

    def available(self) -> bool:
        return self.installed

    def read_image(self, data: bytes, limits: Limits) -> str | None:
        return self.texts.get(1)

    def read_pdf_pages(self, data: bytes, pages: list[int], limits: Limits) -> OcrPages:
        self.calls.append(pages)
        return OcrPages(
            texts={n: t for n, t in self.texts.items() if n in pages}, blank=self.blank & set(pages)
        )


def test_a_text_pdf_is_read_exactly_and_never_ocrd(cv_text_pdf: bytes) -> None:
    ocr = FakeOcr()
    result = extract(cv_text_pdf, ocr=ocr)
    assert result.status == "extracted"
    assert result.page_count == 1
    assert result.text is not None
    assert "aisyah.rahman@example.com +60 12 345 6789" in result.text
    assert "C++" in result.text
    assert ocr.calls == [], "a short but real text page must not be OCR'd (OCR can damage emails and C++)"


def test_only_scanned_pages_go_to_ocr_and_page_order_is_kept() -> None:
    pdf = join_pdfs(
        text_pdf(["Page one is real text"]), scanned_pdf(text_pdf(["whatever"])), text_pdf(["Page three"])
    )
    ocr = FakeOcr(texts={2: "Page two came from OCR"})
    result = extract(pdf, ocr=ocr)
    assert ocr.calls == [[2]]
    assert result.text == "Page one is real text\n\nPage two came from OCR\n\nPage three"
    assert result.extractor == "pdfplumber@" + result.extractor.split("@")[1].split("+")[0] + "+fake-ocr@1"


def test_pages_ocr_cannot_read_become_a_warning_not_silent_loss() -> None:
    pdf = join_pdfs(
        text_pdf(["Readable first page"]), scanned_pdf(text_pdf(["x"])), scanned_pdf(text_pdf(["y"]))
    )
    result = extract(pdf, ocr=FakeOcr(texts={2: ""}, blank={3}))
    assert result.status == "extracted"
    assert result.warnings == [
        "OCR could not read page 2. Text from the other pages was kept, but this document may be incomplete; "
        "review it or upload a clearer copy."
    ], "page 3 is visibly blank, so it is not reported as lost"


def test_without_ocr_a_scan_needs_ocr_and_a_mixed_pdf_keeps_its_text_pages() -> None:
    scan = scanned_pdf(text_pdf(CV_LINES))
    only_scan = extract(scan, ocr=FakeOcr(installed=False))
    assert only_scan.status == "needs_ocr"
    assert only_scan.text is None

    mixed = extract(join_pdfs(text_pdf(["Text page"]), scan), ocr=None)
    assert mixed.status == "extracted"
    assert mixed.text == "Text page"
    assert mixed.warnings == [
        "Page 2 looks scanned, but OCR is not available; only the other pages were read."
    ]


def test_ocr_replaces_a_weak_text_layer_only_when_it_finds_clearly_more() -> None:
    weak = _Page(text="Jane Tan", ocr_reason="sparse-visual")
    assert choose_page_text(weak, "Jane Tan Senior Accountant Maybank 2016 to 2024").startswith(
        "Jane Tan Senior"
    )
    assert choose_page_text(weak, "Jane Tan.") == "Jane Tan", "OCR noise does not replace exact text"
    garbled = _Page(text="(cid:1)(cid:2)", ocr_reason="unextractable")
    assert choose_page_text(garbled, "Real words") == "Real words"


def test_too_many_pages_are_refused() -> None:
    result = extract(text_pdf(*[["p"]] * 4), ocr=None, limits=Limits(max_pdf_pages=3))
    assert result.status == "failed"
    assert result.error == "The PDF has 4 pages; the limit is 3."


def test_damaged_files_fail_with_a_readable_message() -> None:
    assert extract(b"%PDF-1.4 garbage", ocr=None).error == "The file is damaged or not a valid document."
    assert (extract(b"hello", ocr=None).error or "").startswith("Only PDF, Word (.docx or .doc), text")


@pytest.mark.ocr
def test_real_ocr_reads_a_scanned_cv() -> None:
    result = extract(scanned_pdf(text_pdf(CV_LINES, font_size=14)), ocr=TesseractOcr())
    assert result.status == "extracted"
    assert result.text is not None
    assert "Nur Aisyah binti Rahman" in result.text
    assert "Maybank" in result.text
    assert "+tesseract@" in result.extractor


@pytest.mark.ocr
def test_real_ocr_turns_sideways_and_upside_down_scans_upright() -> None:
    for angle in (90, 180):
        result = extract(scanned_pdf(text_pdf(CV_LINES, font_size=14), rotate=angle), ocr=TesseractOcr())
        assert result.text is not None
        assert "Senior Accountant" in result.text, f"rotated {angle}"

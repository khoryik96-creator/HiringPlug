from __future__ import annotations

import pytest

from hiringplug_ai.extraction.limits import DocumentRejectedError, Limits
from hiringplug_ai.extraction.sniff import sniff
from hiringplug_ai.extraction.text import clean_text, letter_count, looks_unextractable
from hiringplug_ai.extraction.zipsafety import validate_zip

from .documents import zip_bytes


def test_clean_text_fixes_known_ligature_codes_and_drops_nul() -> None:
    assert clean_text("Accoun(cid:415)ng so(cid:332)ware\x00") == "Accounting software"


def test_clean_text_keeps_real_content() -> None:
    raw = "  C++ / C#  developer \r\n\r\n\r\n\n  jane@example.com  +60 12-345 6789 "
    assert clean_text(raw) == "C++ / C# developer\n\njane@example.com +60 12-345 6789"


def test_glyph_noise_is_unextractable_but_real_scripts_are_not() -> None:
    assert looks_unextractable("(cid:12)(cid:15)(cid:22)(cid:9) " * 20)
    assert looks_unextractable("\x01\x02\x03\x04 abc " * 20)
    assert not looks_unextractable("Experienced accountant with ten years in audit and tax. " * 3)
    assert not looks_unextractable("会计师 拥有十年审计和税务经验 " * 10)  # Chinese
    assert not looks_unextractable("short")


def test_letter_count_ignores_glyph_codes() -> None:
    assert letter_count("ab(cid:415)c 12") == 3


def test_sniff_trusts_bytes_not_names() -> None:
    assert sniff(b"%PDF-1.7 rest") == "pdf"
    assert sniff(zip_bytes({"word/document.xml": b"<w:document/>"})) == "docx"
    assert sniff(zip_bytes({"photo.jpg": b"..."})) == "unknown"
    assert sniff(b"PK\x03\x04 broken zip") == "unknown"
    assert sniff(b"plain text") == "unknown"


def test_zip_bombs_are_rejected() -> None:
    limits = Limits(max_zip_expanded_bytes=1_000_000, max_zip_single_entry_bytes=600_000, max_zip_entries=5)
    with pytest.raises(DocumentRejectedError, match="oversized internal file"):
        validate_zip(zip_bytes({"word/document.xml": b"0" * 700_000}), limits)
    with pytest.raises(DocumentRejectedError, match="expands beyond"):
        validate_zip(zip_bytes({f"part{i}.xml": b"0" * 500_000 for i in range(3)}), limits)
    with pytest.raises(DocumentRejectedError, match="too many internal files"):
        validate_zip(zip_bytes({f"f{i}": b"x" for i in range(6)}), limits)
    with pytest.raises(DocumentRejectedError, match="compression ratio"):
        validate_zip(zip_bytes({"word/document.xml": b"\0" * (9 * 1024 * 1024)}), Limits())
    validate_zip(zip_bytes({"word/document.xml": b"<ok/>"}), limits)  # a normal file passes

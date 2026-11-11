from __future__ import annotations

from hiringplug_ai.parsing import parse_cv
from hiringplug_ai.parsing.layout import read_lines
from hiringplug_ai.parsing.residence import residence_country


def where(
    text: str, location: tuple[str, str] | None = None, phone: tuple[str, str] | None = None
) -> tuple[str, str] | None:
    return residence_country(read_lines(text), location, phone)


def test_from_the_address_then_places_then_the_phone() -> None:
    assert where("x", location=("Jakarta, Indonesia", "Jl. Sudirman, Jakarta, Indonesia")) == (
        "ID",
        "Indonesia",
    )
    assert where("AHMAD\nTaman Melawati, Kuala Lumpur\nahmad@x.com") == ("MY", "Kuala Lumpur")
    assert where("AHMAD\nBased in KL") == ("MY", "KL")
    assert where("AHMAD", phone=("+65 9123 4567", "+65 9123 4567")) == ("SG", "+65 9123 4567")
    assert where("AHMAD", phone=("+673 712 3456", "+673 712 3456")) == ("BN", "+673 712 3456")
    assert where("AHMAD", phone=("012-345 6789", "012-345 6789")) == ("MY", "012-345 6789")


def test_an_employer_further_down_says_nothing_about_where_they_live() -> None:
    text = (
        "AHMAD\nahmad@x.com\n"
        + "\n".join(f"line {i}" for i in range(20))
        + "\nAccountant, Contoh Sdn Bhd, Penang"
    )
    assert where(text) is None, "not known: shown as NA"
    assert where("AHMAD\nkl office") is None, "kl only in capitals"
    assert where("AHMAD\nSeeking opportunities in Singapore, KL or Penang") is None, (
        "where they would like to work"
    )
    assert where("AHMAD\nAnalyst | Shopee Malaysia") is None, "whom they work for"


def test_parser_extracts_employer_evidence_for_go_classification() -> None:
    p = parse_cv(
        "SITI AISYAH\nsiti@example.com | +60 12-345 6789\n\n"
        "EXPERIENCE\nSenior Accountant | Maybank Berhad\nJan 2020 - Present\n"
    )
    assert p.residenceCountry is not None
    assert (p.residenceCountry.value, p.residenceCountry.quote) == ("MY", "+60 12-345 6789")
    assert p.industry is None, "the Go catalogue owns employer-sector rules"
    assert p.experience[0].company is not None
    assert p.experience[0].company.value == "Maybank Berhad"

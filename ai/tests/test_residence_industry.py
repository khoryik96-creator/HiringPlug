from __future__ import annotations

import pytest

from hiringplug_ai.parsing import parse_cv
from hiringplug_ai.parsing.industry import industry
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


@pytest.mark.parametrize(
    ("company", "title", "expected"),
    [
        ("Maybank Berhad", "Software Engineer", "Financial Services"),
        ("Grab", None, "Digital & E-Commerce"),
        ("Deloitte PLT", "Audit Associate", "Professional Services"),
        (
            "Contoh Sdn Bhd",
            "SAP FICO Consultant",
            "Information Technology & Services",
        ),  # as Kano: SAP/ERP work is IT
        ("Contoh Sdn Bhd", "Software Engineer", "Information Technology & Services"),
        ("Infineon Technologies", None, "Industrial/Manufacturing"),
        ("Nestlé Products Sdn Bhd", None, "FMCG"),
        ("Universiti Malaya", "Lecturer", "Education"),
        ("Kementerian Kewangan", None, "Government Sector"),
        ("KPJ Healthcare", None, "Life Science/Medical"),
        ("Contoh Sdn Bhd", "Clerk", None),
        ("Contoh Sdn Bhd", "Java Developer", "Information Technology & Services"),
        ("Contoh Sdn Bhd", "Power BI Analyst", "Information Technology & Services"),
        ("Contoh Sdn Bhd", "Audit Associate", None),  # a title says little about the employer
        ("Sunrise Property Developer Sdn Bhd", None, "Property & Construction"),
    ],
)
def test_industry_of_the_latest_employer(company: str, title: str | None, expected: str | None) -> None:
    got = industry(company, title)
    assert (got[0] if got else None) == expected


def test_infineon_is_a_maker_not_a_tech_services_firm() -> None:
    # A known name wins over general words such as "Technologies".
    assert industry("Infineon Technologies", None) == ("Industrial/Manufacturing", "Infineon Technologies")


def test_parse_cv_fills_both_with_their_evidence() -> None:
    p = parse_cv(
        "SITI AISYAH\nsiti@example.com | +60 12-345 6789\n\n"
        "EXPERIENCE\nSenior Accountant | Maybank Berhad\nJan 2020 - Present\n"
    )
    assert p.residenceCountry is not None
    assert (p.residenceCountry.value, p.residenceCountry.quote) == ("MY", "+60 12-345 6789")
    assert p.industry is not None
    assert (p.industry.value, p.industry.quote) == ("Financial Services", "Maybank Berhad")

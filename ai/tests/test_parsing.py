"""CV parsing rules, one CV layout per test. Every CV here is invented."""

from __future__ import annotations

from hiringplug_ai.contracts import ParsedProfile, SourcedText
from hiringplug_ai.extraction import TesseractOcr, extract
from hiringplug_ai.parsing import parse_cv

from .documents import scanned_pdf, text_pdf


def v(s: SourcedText | None) -> str | None:
    return s.value if s else None


def jobs(p: ParsedProfile) -> list[tuple[str | None, str | None, str | None, str | None, bool]]:
    return [(v(j.title), v(j.company), j.startDate, j.endDate, j.current) for j in p.experience]


def assert_grounded(text: str, p: ParsedProfile) -> None:
    """Every quote is CV text: the property the Go side enforces."""
    values = [
        p.fullName,
        p.email,
        p.phone,
        p.linkedinUrl,
        p.location,
        p.nationality,
        p.noticePeriod,
        *p.skills,
    ]
    for j in p.experience:
        values += [j.title, j.company, j.dates]
    for e in p.education:
        values += [e.qualification, e.institution, e.dates]
    flat = " ".join(text.split())  # quotes may join wrapped lines (Go compares the same way)
    for s in values:
        if s:
            assert " ".join(s.quote.split()) in flat, s


MODERN = """SITI AISYAH BINTI AHMAD
Senior Accountant
+60 12-345 6789 | siti.aisyah@example.com | linkedin.com/in/siti-aisyah
Address: No 5, Jalan SS2/24, 47300 Petaling Jaya, Selangor

PROFESSIONAL SUMMARY
Chartered accountant with 8 years in shared services.

WORK EXPERIENCE
Senior Accountant | Nestlé Malaysia Berhad | Jan 2020 – Present
• Led month-end close for 3 entities.
Accountant
ABC Sdn Bhd | March 2016 - Dec 2019
• Prepared management accounts.

EDUCATION
Bachelor of Accounting (Hons), Universiti Malaya, 2015
SPM, SMK Damansara Jaya, 2010

SKILLS
Technical: SAP FICO, Excel (Advanced), Power BI
Languages: English, Bahasa Malaysia
Nationality: Malaysian
Notice period: 1 month
"""


def test_reads_a_typical_cv_with_quotes_from_the_text() -> None:
    p = parse_cv(MODERN)
    assert v(p.fullName) == "Siti Aisyah Binti Ahmad"
    assert p.fullName
    assert p.fullName.quote == "SITI AISYAH BINTI AHMAD"
    assert v(p.email) == "siti.aisyah@example.com"
    assert v(p.phone) == "+60 12-345 6789"
    assert v(p.linkedinUrl) == "https://linkedin.com/in/siti-aisyah"
    assert v(p.location) == "Petaling Jaya, Selangor"
    assert v(p.nationality) == "MY"
    assert v(p.noticePeriod) == "1 month"
    assert [s.value for s in p.skills] == ["SAP FICO", "Excel", "Power BI"]  # not languages or nationality
    assert jobs(p) == [
        ("Senior Accountant", "Nestlé Malaysia Berhad", "2020-01", None, True),
        ("Accountant", "ABC Sdn Bhd", "2016-03", "2019-12", False),
    ]
    assert [(v(e.qualification), v(e.institution), e.endYear) for e in p.education] == [
        ("Bachelor of Accounting (Hons)", "Universiti Malaya", 2015),
        ("SPM", "SMK Damansara Jaya", 2010),
    ]
    assert_grounded(MODERN, p)


def test_reads_the_same_cv_from_a_pdf_and_from_a_scan() -> None:
    pdf = text_pdf(MODERN.splitlines())
    for data in (pdf, scanned_pdf(pdf)):
        text = extract(data, ocr=TesseractOcr()).text or ""
        p = parse_cv(text)
        assert v(p.fullName) == "Siti Aisyah Binti Ahmad"  # OCR reads "BINTI" as "BINT]"
        assert v(p.email) == "siti.aisyah@example.com"
        assert [s.value for s in p.skills] == ["SAP FICO", "Excel", "Power BI"]
        assert jobs(p)[0] == ("Senior Accountant", "Nestlé Malaysia Berhad", "2020-01", None, True)
        assert_grounded(text, p)


def test_jobstreet_export_with_title_at_company() -> None:
    text = """Created with Jobstreet Profile
Tan Wei Ming
weiming.tan@example.com
0123456789
Career history
Software Engineer at Grab Malaysia Sdn Bhd
Oct 2021 - Present
Built payment services.
Junior Developer at Acme Technologies
Jun 2019 - Sep 2021
"""
    p = parse_cv(text)
    assert v(p.fullName) == "Tan Wei Ming"
    assert v(p.phone) == "0123456789"
    assert jobs(p) == [
        ("Software Engineer", "Grab Malaysia Sdn Bhd", "2021-10", None, True),
        ("Junior Developer", "Acme Technologies", "2019-06", "2021-09", False),
    ]
    assert_grounded(text, p)


def test_company_above_a_dated_role_and_dates_first_layouts() -> None:
    text = """Lim Mei Ling
EXPERIENCE
KINESSO LLC., Bangsar, KL
Senior Data Engineer, October 2022 – Present
Jan 2018 - Sep 2022
Maybank Berhad
Data Analyst
"""
    p = parse_cv(text)
    assert jobs(p) == [
        ("Senior Data Engineer", "KINESSO LLC.", "2022-10", None, True),
        ("Data Analyst", "Maybank Berhad", "2018-01", "2022-09", False),
    ]


def test_name_sharing_the_first_line_with_a_phone() -> None:
    p = parse_cv("SOH YONG HUAT 013-2899564\nsoh@example.com\nSAP Consultant\n")
    assert v(p.fullName) == "Soh Yong Huat"
    assert v(p.phone) == "013-2899564"


def test_ocr_misreading_a_final_i() -> None:
    p = parse_cv("SITI AISYAH BINT] AHMAD\nsiti@example.com\n")
    assert p.fullName
    assert (p.fullName.value, p.fullName.quote) == ("Siti Aisyah Binti Ahmad", "SITI AISYAH BINT] AHMAD")


def test_labelled_name_on_the_next_line() -> None:
    p = parse_cv("Personal Details\nName:\nTeng Shi Li\nNationality: Singaporean\n")
    assert v(p.fullName) == "Teng Shi Li"
    assert v(p.nationality) == "SG"


def test_phone_rules_from_real_cv_mistakes() -> None:
    # Year ranges are not phones; numbers do not join across lines; 8-digit runs
    # are phones only when they look like Singapore mobiles.
    p = parse_cv("Ivy Ng\nlinkedin.com/in/ivy-ng-123\n1999-2001\nRef 20240115\n")
    assert p.phone is None
    p = parse_cv("Ivy Ng\nlinkedin.com/in/ivy-ng-123\n0122223333\n")
    assert v(p.phone) == "0122223333"
    assert v(parse_cv("Ivy Ng\nMobile: 9123 4567\n").phone) == "9123 4567"


def test_immediate_availability_and_unknown_sections() -> None:
    p = parse_cv("Ali bin Abu\nAvailability: Immediate\nHOBBIES\nFootball, reading\n")
    assert v(p.noticePeriod) == "0 day"
    assert p.skills == []  # never guessed from other sections
    assert p.experience == []


def test_nothing_to_read_gives_an_empty_profile() -> None:
    p = parse_cv("")
    assert p.fullName is None
    assert p.experience == []
    assert p.parser == "rules@1"


def test_a_referees_labelled_phone_does_not_beat_the_candidates_own() -> None:
    text = "Siti Aisyah | 012-345 6789 | siti@example.com\nEXPERIENCE\nAccountant\n" + "Duties.\n" * 50
    text += "REFERENCES\nMr Tan, Phone: 019-888 7777\n"
    assert v(parse_cv(text).phone) == "012-345 6789"


def test_name_particles_are_not_email_support() -> None:
    # "bin" is in "robinson", but that says nothing about who the CV belongs to.
    p = parse_cv("SKILLS\nAhmad Bin Ali\nEMAIL\nrobinson@example.com\n")
    assert p.fullName is None

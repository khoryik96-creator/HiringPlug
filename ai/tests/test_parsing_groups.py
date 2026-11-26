"""Shapes the rules used to get wrong: a name written surname first with a
comma, an employer written once over several dated roles, and a list of
projects (client engagements) taken for jobs. Every name here is invented;
the layouts copy real CVs."""

from __future__ import annotations

from hiringplug_ai.parsing import parse_cv

from .test_parsing import assert_grounded, jobs


def test_a_name_written_surname_first_keeps_every_name() -> None:
    text = "Tan, Mei Ling\nSenior Engineer | Petaling Jaya, Malaysia | +60120000001 | meilingtan@example.com\n"
    p = parse_cv(text)
    assert_grounded(text, p)
    assert p.fullName is not None
    assert (p.fullName.value, p.fullName.quote) == ("Mei Ling Tan", "Tan, Mei Ling")


def test_letters_after_a_name_are_not_given_names() -> None:
    text = "Lim Wei Jie, CPA\nlimweijie@example.com\n"
    assert parse_cv(text).fullName.value == "Lim Wei Jie"  # type: ignore[union-attr]


def test_an_employer_written_once_over_its_roles() -> None:
    text = (
        "Tan, Mei Ling\nmeilingtan@example.com\n\nPROFESSIONAL EXPERIENCE\n\n"
        "Contoh Global Business Center Sdn Bhd, Cyberjaya | May 2010 – Oct 2026\n\n"
        " Database Engineer Sr. Advisor                     May 2021 – Oct 2026\n"
        " • Built an agent auto-heal automation.\n\n"
        " Database Administrator Advisor (promoted)        Oct 2019 – May 2021\n"
        " Database Administrator Sr. Analyst (Team Lead) | Contoh Global Business Center Sdn Bhd | Oct 2015 – Oct 2019\n"
        " Database Administrator Analyst                    May 2010 – Oct 2015\n"
    )
    p = parse_cv(text)
    assert_grounded(text, p)
    employer = "Contoh Global Business Center Sdn Bhd"
    assert [(t, c) for t, c, *_ in jobs(p)] == [
        ("Database Engineer Sr. Advisor", employer),
        ("Database Administrator Advisor (promoted)", employer),
        ("Database Administrator Sr. Analyst (Team Lead)", employer),
        ("Database Administrator Analyst", employer),
    ], "the employer's own line is its span, not a job"


def test_client_engagements_are_not_jobs() -> None:
    text = (
        "MUHAMMAD HAFIZ ISMAIL\nhafiz@example.com\n\nPROFESSIONAL EXPERIENCE\n"
        "Solutions Architect                         May 2021 - Present\n"
        "Contoh Web Solutions | Kuala Lumpur, Malaysia\n"
        " • Design end-to-end solutions.\n\n"
        "SELECTED ARCHITECTURE & DELIVERY ENGAGEMENTS\n"
        "Negara Domain Name Platform - UAE | 2024 - Present\n"
        " • Designed an active-active registry platform.\n"
        "Registry R2R System | 2019 - 2023\n"
        "Technical Support Lead | Contoh Solutions Sdn Bhd | Jul 2017 - May 2021\n"
    )
    p = parse_cv(text)
    first = jobs(p)[0]
    assert first[:2] == ("Solutions Architect", "Contoh Web Solutions"), "the current job, not a 2024 project"
    assert all("ENGAGEMENTS" not in (c or "") and "Platform" not in (t or "") for t, c, *_ in jobs(p))

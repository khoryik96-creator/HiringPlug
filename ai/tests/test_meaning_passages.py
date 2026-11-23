"""CVs cut into passages, and job descriptions into the needs they list
(meaning/passages.py). Made-up texts only."""

from __future__ import annotations

from hiringplug_ai.meaning.passages import MAX_CHARS, needs, passages

CV = """Aina Binti Rahman
aina.rahman@example.com | +60 12-345 6789 | linkedin.com/in/aina-example

SUMMARY
Talent acquisition specialist with eight years of full-cycle hiring for technology and shared services
teams across Malaysia and Singapore, from sourcing to offer negotiation.

EXPERIENCE
Talent Acquisition Specialist, Contoh Fintech Sdn Bhd (2021 - present)
• Hired 120 engineers, product managers and designers a year
• Built the referral programme and the campus hiring calendar
• Partnered hiring managers on interview panels and scorecards

Recruitment Consultant, Sampel Recruitment (2018 - 2021)
• Ran a 360 desk for finance and accounting roles; 25 placements a year
• Business development with MNC clients in Kuala Lumpur

EDUCATION
Bachelor of Human Resource Management, Universiti Contoh (2017)
"""


def test_a_cv_is_cut_into_passages_in_its_order_without_contact_details() -> None:
    got = passages(CV)
    assert got, "the CV has passages"
    joined = " ".join(got)
    assert "@" not in joined
    assert "345 6789" not in joined
    assert "linkedin.com" not in joined
    assert joined.index("full-cycle hiring") < joined.index("Hired 120 engineers") < joined.index("360 desk")
    assert all(len(p.split()) >= 8 for p in got), "no headings or names on their own"
    assert not any(p.startswith(("•", "-")) for p in got), "bullets are taken off"


def test_a_cvs_name_where_they_live_and_its_headings_are_left_out() -> None:
    joined = " ".join(passages(CV))
    for left_out in ("Aina Binti Rahman", "SUMMARY", "EXPERIENCE", "EDUCATION"):
        assert left_out not in joined  # what a passage shows starts at the work
    cv = "NURUL AINA\n\nKuala Lumpur, Malaysia\n\nWORK EXPERIENCE\n"
    cv += "Talent Acquisition Partner | Contoh | 2021 - Present\nHired 120 engineers and designers a year.\n"
    assert passages(cv)[0].startswith("Talent Acquisition Partner | Contoh")
    skills = "SKILLS\nSAP FICO, ACCA, MFRS reporting and consolidation for group companies"
    assert passages(skills) == ["SAP FICO, ACCA, MFRS reporting and consolidation for group companies"]


def test_a_long_cv_keeps_its_first_passages_and_long_lines_are_cut() -> None:
    long_line = " ".join(f"word{i}" for i in range(400))
    got = passages(long_line + "\n\n" + CV * 10, limit=5)
    assert len(got) == 5
    assert all(len(p) <= MAX_CHARS for p in got)
    assert all(len(p.split()) <= 120 for p in got)


def test_an_empty_or_contact_only_cv_has_no_passages() -> None:
    assert passages("") == []
    assert passages("Aina\naina@example.com\n+60 12 345 6789") == []


JD = """About us
Contoh Bhd is a fast-growing fintech with offices in Kuala Lumpur and Singapore.

Responsibilities
- Prepare monthly management accounts and group consolidation
- Liaise with external auditors and tax agents
- Lead a team of three accounts executives

Requirements
- Degree in accounting; ACCA or CIMA
- At least 5 years in a similar role, audit firm experience preferred
- Hands-on with SAP S/4HANA

What we offer
- Competitive salary and annual bonus
- Medical insurance and 18 days annual leave

How to apply
Send your CV to careers@example.com
"""


def test_a_job_description_lists_its_needs_not_its_story_or_benefits() -> None:
    got = needs(JD)
    assert got[0] == "Prepare monthly management accounts and group consolidation"
    assert "Hands-on with SAP S/4HANA" in got
    assert len(got) == 6
    joined = " ".join(got)
    for left_out in ("fast-growing", "salary", "insurance", "Send your CV"):
        assert left_out not in joined


def test_a_job_description_without_headings_gives_its_bullets_or_its_sentences() -> None:
    bullets = "We are hiring!\n- Five years of payroll for 500 staff\n- EPF, SOCSO and PCB submissions\n"
    assert needs(bullets) == ["Five years of payroll for 500 staff", "EPF, SOCSO and PCB submissions"]
    plain = (
        "We need a recruiter who has hired engineers. Salary is competitive. "
        "You will run interviews and offers."
    )
    assert needs(plain) == [
        "We need a recruiter who has hired engineers.",
        "You will run interviews and offers.",
    ]

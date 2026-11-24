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


JOBSTREET = """Job Responsibilities:
- Handle full set of accounts
- Prepare monthly management reports
- Liaise with auditors and tax agents

Job Requirements:
- Degree in Accounting
- ACCA
- Experience in AutoCount
- At least 2 years of working experience
- Able to start immediately
"""


def test_jobstreet_headings_keep_every_duty_and_requirement() -> None:
    assert needs(JOBSTREET) == [
        "Handle full set of accounts",
        "Prepare monthly management reports",
        "Liaise with auditors and tax agents",
        "Degree in Accounting",
        "Experience in AutoCount",  # a need that starts with a heading's word, not a heading
        "At least 2 years of working experience",
        "Able to start immediately",
    ]


def test_a_need_starting_with_a_headings_word_is_not_a_heading() -> None:
    listed = [
        "Degree in Accounting or equivalent",
        "Experience in full set of accounts",
        "Experience with SQL Account software",
        "Must have own transport",
        "Skills in Excel and SAP",
        "Good command of English and Bahasa Malaysia",
    ]
    assert needs("Requirements:\n" + "".join(f"- {line}\n" for line in listed)) == listed


def test_bullets_before_a_later_heading_are_kept_and_the_companys_are_not() -> None:
    jd = (
        "We are hiring an Accounts Executive!\n"
        "- Handle full set of accounts\n"
        "- Prepare monthly reports for management\n"
        "Your Mission:\n"  # a heading that says neither
        "- Close the books every month on time\n"
        "Requirements: Degree in Accounting or ACCA\n"  # a heading with its first need
        "- Two years in an audit firm\n"
        "Why join us?\n"
        "- Great team and a fun culture\n"
        "About Contoh\n"
        "- Founded in 2001 with 300 staff\n"
    )
    assert needs(jd) == [
        "Handle full set of accounts",
        "Prepare monthly reports for management",
        "Close the books every month on time",
        "Requirements: Degree in Accounting or ACCA",
        "Two years in an audit firm",
    ]

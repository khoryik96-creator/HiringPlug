"""The agency's verticals (20 to 30 consultants, each with their own:
software, data, DevOps, security, sales, marketing, finance, operations,
compliance): their typical titles, tools and qualifications are each read
in the right job family, so a search in one vertical finds its people and
counts other fields for less (meaning/families.py, meaning/recruiter.py)."""

from __future__ import annotations

import pytest

from hiringplug_ai.meaning.recruiter import family, field, read_search

# One family per line: its typical titles, separated by "; ".
_TITLES = """
software: Software Engineer; Senior Backend Developer; Full Stack Developer; Frontend Engineer (React);
  Mobile Developer; Staff Software Engineer; Tech Lead; Technical Architect; CTO
software testing: QA Automation Engineer; SDET; Software Tester; Automation Test Engineer
data: Data Analyst; Data Engineer; Data Scientist; Machine Learning Engineer; AI Engineer; MLOps Engineer;
  BI Developer; Head of Data; Analytics Engineer
cloud and devops: DevOps Engineer; Site Reliability Engineer; Cloud Engineer; Platform Engineer;
  AWS Solutions Architect; DevSecOps Engineer
cyber security: Cyber Security Engineer; Information Security Manager; SOC Analyst; Penetration Tester;
  Security Architect; GRC Analyst; IAM Engineer; CISO; Security Operations Manager;
  Application Security Engineer; Threat Intelligence Analyst; IT Security Officer; Cloud Security Engineer
sales: Business Development Manager; Enterprise Account Manager; Key Account Manager; SDR;
  Sales Development Representative; Head of Sales; Pre-Sales Consultant
marketing: Marketing Manager; Brand Manager; Product Marketing Manager; CMO; Trade Marketing Executive
digital marketing: Digital Marketing Executive; Performance Marketing Manager; SEO Specialist; Growth Marketer
accounting: Accounts Executive; Accountant; Accounting Manager; AP Executive; Cost Accountant
finance management: Finance Manager; Financial Controller; FP&A Manager; CFO; Management Accountant
operations: Operations Manager; Operations Executive; Head of Operations; COO; Revenue Operations Manager;
  Business Operations Analyst
compliance: Compliance Officer; AML Analyst; KYC Analyst; Sanctions Analyst; Fraud Analyst;
  Data Protection Officer; Shariah Compliance Officer
"""


def _verticals() -> dict[str, list[str]]:
    out: dict[str, list[str]] = {}
    for block in _TITLES.strip().replace("\n  ", " ").splitlines():
        name, titles = block.split(": ", 1)
        out[name] = [t.strip() for t in titles.split(";") if t.strip()]
    return out


VERTICALS = _verticals()


@pytest.mark.parametrize(
    ("title", "want"), [(title, fam) for fam, titles in VERTICALS.items() for title in titles]
)
def test_each_verticals_titles_are_read_in_its_family(title: str, want: str) -> None:
    found = family(title)
    assert found is not None, title
    assert found[0] == want


def test_revops_is_revenue_operations() -> None:
    s = read_search("RevOps")
    assert s.family is not None
    assert s.family[0] == "operations"
    assert "revenue operations" in field("RevOps")


@pytest.mark.parametrize(
    "title",
    [
        "Security Officer",  # a guard, not cyber security
        "Account Executive",  # sales or accounts: the work under it says
        "Engineering Manager",  # software or a factory's engineering: the work says
    ],
)
def test_titles_that_cannot_say_are_left_to_the_work(title: str) -> None:
    assert family(title) is None


@pytest.mark.parametrize(
    ("search", "want"),
    [
        ("AWS", "cloud and devops"),
        ("Kubernetes", "cloud and devops"),
        ("Snowflake", "data"),  # a data platform, not a warehouse of goods
        ("Java developer", "software"),
        ("CISSP", "cyber security"),
        ("OSCP", "cyber security"),
        ("Google Ads", "digital marketing"),
        ("ACCA", "accounting"),
        ("Xero", "accounting"),  # accounting software is accounting
        ("AutoCount", "accounting"),
        ("CAMS", "compliance"),
    ],
)
def test_a_tool_or_qualification_is_searched_in_its_field_and_counts_where_a_cv_names_it(
    search: str, want: str
) -> None:
    s = read_search(search)
    assert s.family is not None
    assert s.family[0] == want
    assert s.exact

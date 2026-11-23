"""How a recruiter reads titles and searches (meaning/recruiter.py): the
rules the meaning model is given, each with the trap it is there for."""

from __future__ import annotations

import pytest

from hiringplug_ai.meaning.recruiter import (
    expand,
    family,
    family_of_work,
    field,
    level,
    read_search,
    reading,
    reading_with_work,
    work_family,
)


@pytest.mark.parametrize(
    ("text", "says"),
    [
        ("AR/AP Exec", ["accounts receivable", "accounts payable", "executive"]),
        ("HRBP", ["human resources business partner"]),
        ("Sr. QS", ["senior", "quantity surveyor"]),
        ("TA Partner", ["talent acquisition"]),
        ("SAP FI/CO Consultant", ["finance and controlling"]),
        ("S/4HANA", ["SAP"]),
        ("ACCA", ["accounting qualification"]),
        ("Power BI", ["data dashboards"]),
    ],
)
def test_abbreviations_tools_and_qualifications_are_spelt_out(text: str, says: list[str]) -> None:
    for words in says:
        assert words in expand(text)


def test_words_that_only_look_like_abbreviations_are_left() -> None:
    assert expand("Mia Tan, recruiter") == "Mia Tan, recruiter"
    assert expand("Head of IT") == "Head of information technology"


@pytest.mark.parametrize(
    ("title", "want"),
    [
        ("Intern, Finance", 0),
        ("Account Assistant", 1),
        ("Accounts Executive", 2),
        ("Executive Assistant", 2),  # not an assistant's level: an EA
        ("Senior Accountant", 3),
        ("Assistant Manager, Finance", 4),  # not 1 and 5
        ("Finance Manager", 5),
        ("Senior Manager", 6),
        ("CFO", 7),
        ("Talent Acquisition", None),
    ],
)
def test_the_level_of_a_title(title: str, want: int | None) -> None:
    assert level(title) == want


@pytest.mark.parametrize(
    ("title", "want"),
    [
        ("Accounts Executive", "accounting"),
        ("Akauntan", "accounting"),
        ("Finance Business Partner", "finance management"),  # a business partner, not HR
        ("HR Business Partner", "hr business partner"),
        ("Recruitment Business Partner", "recruitment"),
        ("Talent Partner", "recruitment"),
        ("Talent Sourcer", "recruitment"),
        ("Strategic Sourcing Manager", "procurement"),  # sourcing suppliers, not people
        ("Company Secretary", "corporate secretarial"),  # not an assistant
        ("Executive Assistant", "executive assistant"),
        ("IT Project Manager", "project management"),  # IT only says where
        ("Project Manager, IT", "project management"),
        ("Assistant Manager, Finance", "accounting"),  # the part after the comma, when the role says nothing
        ("Software Tester", "software testing"),
        ("QA Executive", "manufacturing quality"),
        ("Power BI Developer", "data"),
        ("Network Engineer", "it infrastructure"),
        ("Civil Engineer", "civil engineering"),
        ("Business Development Manager", "sales"),
        ("Relationship Manager", "banking"),
        ("Staff Nurse", "healthcare"),
    ],
)
def test_a_title_is_placed_in_its_job_family(title: str, want: str) -> None:
    found = family(title)
    assert found is not None
    assert found[0] == want


@pytest.mark.parametrize(
    "title", ["Account Executive", "QA Engineer", "Principal Consultant", "Senior Manager"]
)
def test_a_title_that_cannot_say_is_left_to_the_work_under_it(title: str) -> None:
    assert family(title) is None


@pytest.mark.parametrize(
    ("title", "work", "want"),
    [
        (
            "Principal Consultant",
            "Ran a 360 desk placing accountants with MNC clients; 25 placements a year.",
            "recruitment",
        ),
        ("Account Executive", "Sold SaaS subscriptions to SMEs; closed RM 1.2m new business.", "sales"),
        (
            "Account Executive",
            "Full set of accounts, bank reconciliation and month-end closing.",
            "accounting",
        ),
        (
            "QA Engineer",
            "Automated tests with Selenium and Cypress; test plans for releases.",
            "software testing",
        ),
        ("QA Engineer", "HACCP and GMP audits in a food factory; CAPA.", "manufacturing quality"),
    ],
)
def test_a_title_that_cannot_say_is_placed_by_the_work(title: str, work: str, want: str) -> None:
    found = work_family(title, work)
    assert found is not None
    assert found[0] == want
    assert reading_with_work(title, work).startswith(found[1])


def test_work_that_points_two_ways_equally_says_nothing() -> None:
    assert family_of_work("Sold software; prepared invoices.") is None
    assert family_of_work("Answered the phone.") is None


def test_what_the_person_does_leaves_the_rank_words_out() -> None:
    assert field("Senior Finance Manager") == "finance"
    assert field("Accounts Executive") == "accounts accounting"
    assert field("Account Executive") == "account sales"  # what most mean by it
    assert field("Manager") == "manager"  # nothing else left: kept


def test_a_title_is_read_as_its_familys_work_then_its_own_words() -> None:
    assert reading("Accounts Executive").startswith("accounting: bookkeeping")
    assert reading("Account Executive") == "account sales"


@pytest.mark.parametrize(
    ("text", "want"),
    [
        ("headhunter for finance roles", ("headhunter", "finance roles", False, "recruitment", 2)),
        ("account executive selling software", ("account executive", "selling software", False, None, 2)),
        ("Finance Manager", ("Finance Manager", "", False, "finance management", 5)),
        ("someone who hired software engineers for a tech company", ("", "", True, "recruitment", None)),
        (
            "prepared monthly management accounts and liaised with auditors",
            ("", "", True, "accounting", None),
        ),
    ],
)
def test_a_search_is_read_as_the_role_and_the_rest(
    text: str, want: tuple[str, str, bool, str | None, int | None]
) -> None:
    role, context, describes, fam, lvl = want
    s = read_search(text)
    assert (s.role, s.describes, s.level) == (role, describes, lvl)
    assert s.context == (text if describes else context)
    assert (s.family[0] if s.family else None) == fam

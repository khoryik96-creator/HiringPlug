"""How a recruiter reads titles and searches (meaning/recruiter.py): the
rules the meaning model is given, each with the trap it is there for."""

from __future__ import annotations

import pytest

from hiringplug_ai.meaning.families import FAMILY_WORDS, NEAR
from hiringplug_ai.meaning.recruiter import (
    expand,
    family,
    family_of_work,
    field,
    level,
    read_search,
    reading,
    reading_in_work,
    reading_with_work,
    said_level,
    search_reading,
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
        ("HR Executive (Recruitment)", "recruitment"),  # a broad family narrowed in brackets
        ("HR Executive (Payroll)", "payroll"),
        ("Admin Executive (Purchasing)", "procurement"),
        ("HR Executive", "hr"),
        ("Project Manager (IT)", "project management"),  # brackets that only say where
        ("SAP FICO Consultant", "erp finance"),
        ("ERP Finance Consultant", "erp finance"),  # an ERP consultant, not an accountant
        ("SAP MM Consultant", "erp"),
        # Tools and qualifications, placed as a recruiter places them.
        ("S/4HANA", "erp"),
        ("HACCP", "manufacturing quality"),  # food quality, not workplace safety
        ("GMP", "manufacturing quality"),
        ("Selenium", "software testing"),
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


def test_a_title_placed_by_its_work_is_not_explained_the_other_way() -> None:
    books = reading_in_work("Account Executive", "Full set of accounts and month-end closing.")
    assert books.startswith("accounting:")
    assert "sales" not in books  # keeping the books, not selling
    assert reading_in_work("Account Executive", "") == "account sales"  # what most mean by it


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
        ("headhunter for finance roles", ("headhunter", "finance roles", False, "recruitment", None)),
        # A role that cannot say, placed by the rest of the search.
        ("account executive selling software", ("account executive", "selling software", False, "sales", 2)),
        (
            "quality assurance in a food factory",
            ("quality assurance", "a food factory", False, "manufacturing quality", None),
        ),
        ("Finance Manager", ("Finance Manager", "", False, "finance management", 5)),
        ("software tester", ("software tester", "", False, "software testing", None)),  # any level
        ("senior recruiter", ("senior recruiter", "", False, "recruitment", 3)),
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


@pytest.mark.parametrize(
    ("title", "want"),
    [
        ("Headhunter", None),  # the job, not a level: any level will do
        ("Software Engineer", None),
        ("Senior Recruiter", 3),
        ("Accounts Executive", 2),
        ("Finance Manager", 5),
        ("Head of Talent Acquisition", 5),
        ("Sr. Accountant", 3),
    ],
)
def test_a_search_asks_for_a_level_only_when_it_names_one(title: str, want: int | None) -> None:
    assert said_level(title) == want


def test_the_role_asked_for_is_read_with_the_family_the_rest_of_the_search_gives() -> None:
    assert search_reading(read_search("account executive selling software")).startswith("sales:")
    food = search_reading(read_search("quality assurance in a food factory"))
    assert food.startswith("quality assurance in factories")
    assert search_reading(read_search("accounts executive")) == reading("accounts executive")


def test_neighbouring_families_are_real_and_go_both_ways() -> None:
    for name, near in NEAR.items():
        assert name in FAMILY_WORDS
        assert name not in near
        for other in near:
            assert other in FAMILY_WORDS
            assert name in NEAR[other]
    assert "hr" in NEAR["recruitment"]  # an HR generalist hires too
    assert "finance management" in NEAR["accounting"]
    assert "sales" not in NEAR["accounting"]  # Account Executive (sales) is not Accounts Executive
    assert "sales" not in NEAR["erp"]  # an ERP sales director is not an SAP consultant
    assert "executive assistant" not in NEAR["corporate secretarial"]

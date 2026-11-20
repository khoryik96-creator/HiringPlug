"""The work history read by the AI: employers told apart from titles, clients
and teams, every value quoted from the CV, and the newest employer's industry
picked from the company's list. Every name here is invented."""

from __future__ import annotations

from hiringplug_ai.jobs import cv_history
from hiringplug_ai.parsing import parse_cv as rules
from hiringplug_ai.providers import ProviderError

from .test_cv_ai import FakeAI

CV = (
    "NUR AINA BINTI KAMAL\nnur.aina@example.com\n\nWORK EXPERIENCE\n"
    "CONTOH GDC MALAYSIA | Mar 2018 - Present\nClient: Negara Transport Agency\nSystems Engineer\n"
    "Senior Analyst, Contoh Bank Berhad | Jan 2015 - Feb 2018\n"
)
INDUSTRIES = [
    "Financial Services",
    "FSI \u2013 Banking",
    "Information Technology & Services",
    "Tech \u2013 Telecommunications",
]


def answer(**over: object) -> dict[str, object]:
    return {
        "jobs": [
            {
                "company": "CONTOH GDC MALAYSIA",
                "title": "Systems Engineer",
                "dates": "Mar 2018 - Present",
                "start": "2018-03",
                "end": None,
                "current": True,
            },
            {
                "company": "Contoh Bank Berhad",
                "title": "Senior Analyst",
                "dates": "Jan 2015 - Feb 2018",
                "start": "2015-01",
                "end": "2018-02",
                "current": False,
            },
            {
                "company": "Negara Transport Agency Group",
                "title": "Engineer",
                "dates": None,
                "start": None,
                "end": None,
                "current": False,
            },
        ],
        "industry": "Information Technology & Services",
        **over,
    }


def test_the_ai_tells_the_employer_from_the_title_and_the_client() -> None:
    ai = FakeAI(answer())
    got = cv_history.read(rules(CV), CV, ai, INDUSTRIES)
    p = got.profile
    assert [
        (j.company.value if j.company else None, j.title.value if j.title else None) for j in p.experience
    ] == [
        ("CONTOH GDC MALAYSIA", "Systems Engineer"),
        ("Contoh Bank Berhad", "Senior Analyst"),
    ], "an employer the CV does not name is dropped"
    assert p.experience[0].current
    assert p.experience[0].endDate is None
    assert (p.experience[1].startDate, p.experience[1].endDate) == ("2015-01", "2018-02")
    assert p.parser.endswith("+ai-history")
    assert p.industry is not None
    assert (p.industry.value, p.industry.quote) == (
        "Information Technology & Services",
        "CONTOH GDC MALAYSIA",
    ), "the newest employer's industry, quoting that employer"
    assert got.tokens == 460
    _, prompt = ai.calls[0]
    assert "- FSI \u2013 Banking" in prompt, "the AI chooses from the company's own list"
    assert "client, the employer is IBM" in prompt


def test_without_a_list_the_ai_names_the_industry_in_its_own_words() -> None:
    ai = FakeAI(answer(industry="Banking"))
    cv_history.read(rules(CV), CV, ai)
    assert "INDUSTRIES:" not in ai.calls[0][1]
    assert "1 to 4 words" in ai.calls[0][1]


def test_nothing_quotable_or_a_provider_error_keeps_the_rules_reading() -> None:
    before = rules(CV)
    made_up = FakeAI(
        {
            "jobs": [{"company": "Imaginary Holdings", "title": "CEO", "dates": "2020 - 2021"}],
            "industry": "Banking",
        }
    )
    got = cv_history.read(before, CV, made_up, INDUSTRIES)
    assert got.profile == before
    assert got.tokens == 460, "the tokens are counted all the same"
    got = cv_history.read(before, CV, FakeAI(ProviderError("busy")), INDUSTRIES)
    assert got.profile == before
    assert got.tokens == 0


def test_partial_ai_history_keeps_an_older_job_outside_the_prompt() -> None:
    text = CV + ("An achievement in the role.\n" * 600) + "Developer, Daya Berhad | Jan 2011 - Dec 2013\n"
    before = rules(text)
    ai = FakeAI(
        answer(
            jobs=[
                {
                    "company": "CONTOH GDC MALAYSIA",
                    "title": "Systems Engineer",
                    "dates": "Mar 2018 - Present",
                    "current": True,
                }
            ]
        )
    )
    after = cv_history.read(before, text, ai).profile
    assert "Daya Berhad" not in ai.calls[0][1]
    assert len(after.experience) == len(before.experience)
    assert any(j.company and "Daya Berhad" in j.company.value for j in after.experience)


def test_industry_is_not_attributed_to_the_next_surviving_employer() -> None:
    before = rules(CV)
    partial = answer(
        jobs=[
            {"company": "Imaginary Holdings"},
            {"company": "Contoh Bank Berhad", "dates": "Jan 2015 - Feb 2018"},
        ]
    )
    assert cv_history.read(before, CV, FakeAI(partial)).profile.industry == before.industry


def test_client_customer_and_vendor_titles_are_preserved() -> None:
    for title in ("Customer Care Representative", "Client Account Manager", "Vendor Management Analyst"):
        text = (
            f"Nur Aina\nWORK EXPERIENCE\nJob Title: {title}\nCompany: Contoh Bank Berhad\nJan 2021 - Present"
        )
        assert any(job.title and job.title.value == title for job in rules(text).experience)

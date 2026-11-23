"""Employers the rules used to get wrong: a client, a team, an aside or a
career break taken for the employer, "Promoted to" in a title, and an
employer written in capitals read as a title. Every name here is invented;
the layouts copy real CVs."""

from __future__ import annotations

from hiringplug_ai.parsing import parse_cv

from .test_parsing import assert_grounded, jobs

HEAD = "NUR AINA BINTI KAMAL\nnur.aina@example.com\n+60 0120000009\n\nWORK EXPERIENCE\n"


def test_a_client_is_not_the_employer() -> None:
    text = HEAD + "CONTOH GDC MALAYSIA | Mar 2018 - May 2021\nClient Negara Transport Agency (Australia)\n"
    p = parse_cv(text)
    assert_grounded(text, p)
    assert jobs(p)[0][:2] == (None, "CONTOH GDC MALAYSIA"), "the employer in capitals, never the client"


def test_a_team_size_or_an_aside_is_not_the_employer() -> None:
    text = HEAD + (
        "Decision Architect · Team:[4] | Sep 2022 - Present\n"
        "(Solutioned & Governed from Kuala Lumpur Hub)\n"
        "Senior Analyst · Team:[3] | Jan 2020 - Aug 2022\n"
    )
    p = parse_cv(text)
    assert [(t, c) for t, c, *_ in jobs(p)] == [("Decision Architect", None), ("Senior Analyst", None)]


def test_a_career_break_is_not_an_employer() -> None:
    text = HEAD + "Deputy Head of Operation | Sabbatical Leave | Sep 2013 - Jan 2015\n"
    assert jobs(parse_cv(text))[0][:2] == ("Deputy Head of Operation", None)


def test_promoted_to_is_not_part_of_the_title() -> None:
    text = (
        HEAD + "Promoted to Assistant Manager, Country Finance | Contoh Bank Berhad | Mar 2007 - Sep 2008\n"
    )
    assert jobs(parse_cv(text))[0][:2] == ("Assistant Manager, Country Finance", "Contoh Bank Berhad")


def test_team_lead_and_project_manager_are_titles() -> None:
    text = HEAD + (
        "Team Lead, Market Risk | Contoh Bank Berhad | Apr 2024 - Present\n"
        "Project Manager | Sistem Contoh Sdn Bhd | Jul 2003 - Dec 2007\n"
    )
    assert [(t, c) for t, c, *_ in jobs(parse_cv(text))] == [
        ("Team Lead, Market Risk", "Contoh Bank Berhad"),
        ("Project Manager", "Sistem Contoh Sdn Bhd"),
    ]


def test_a_headhunter_at_an_executive_search_firm() -> None:
    text = HEAD + (
        "Headhunter | Sampel Executive Search | Aug 2017 - Present\n"
        "Executive Search Consultant | Contoh Partners Sdn Bhd | Jan 2014 - Jul 2017\n"
    )
    assert [(t, c) for t, c, *_ in jobs(parse_cv(text))] == [
        ("Headhunter", "Sampel Executive Search"),  # the firm's name, not a title for its "Executive"
        ("Executive Search Consultant", "Contoh Partners Sdn Bhd"),
    ]

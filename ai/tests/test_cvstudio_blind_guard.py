"""A blind CV keeps what CV Studio's rules made of the formatted one: titles,
dates, sub-headings and one entry per employer (synthetic CVs only)."""

from __future__ import annotations

import io
import zipfile
from typing import Any

from hiringplug_ai import cvstudio
from hiringplug_ai import cvstudio_blind_guard as guard


def reading() -> dict[str, Any]:
    """A formatted reading, as the blind step is given it."""
    return {
        "candidate": {
            "name": "Aina Example",
            "current_position": "Finance Lead",
            "current_company": "Contoh Berhad",
        },
        "summary_bullets": ["Finance lead with 10 years in shared services."],
        "work_experiences": [
            {
                "company": "Contoh Berhad",
                "date_range": "Jan 2020 to Present",
                "roles": [
                    {
                        "title": "Finance Lead, GST, FM",
                        "date_range": "Jan 2020 to Present",
                        "bullets": [
                            "Led the month-end close.",
                            {
                                "heading": "Treasury",
                                "bullets": ["Cash forecasting.", "Bank reconciliations."],
                            },
                        ],
                    }
                ],
            },
            {
                "company": "Sampel Holdings",
                "date_range": "Mar 2017 to Dec 2019",
                "roles": [
                    {
                        "title": "Contoh Berhad Liaison",
                        "date_range": "Mar 2017 to Dec 2019",
                        "bullets": ["Ran audits."],
                    }
                ],
            },
            {
                "company": "Demo Trading",
                "date_range": "Jan 2015 to Mar 2017",
                "roles": [
                    {
                        "title": "Accountant",
                        "date_range": "Jan 2015 to Mar 2017",
                        "bullets": ["Kept ledgers."],
                    }
                ],
            },
        ],
    }


def ai_answer() -> dict[str, Any]:
    """What an AI may answer: names hidden, but titles reworded, a heading
    flattened into a bullet, and two employers both "[Company]"."""
    return {
        "candidate": {
            "name": "Candidate",
            "current_position": "Senior Finance Leader",
            "current_company": "[Company]",
        },
        "summary_bullets": ["Finance lead with 10 years in shared services."],
        "work_experiences": [
            {
                "company": "Leading Malaysian Conglomerate",
                "date_range": "Jan 2020 to Present",
                "roles": [
                    {
                        "title": "Finance Lead, GST, FM.",
                        "date_range": "Jan 2020 to Present",
                        "bullets": [
                            "Led the month-end close.",
                            "Treasury:",
                            "Cash forecasting.",
                            "Bank reconciliations.",
                        ],
                    }
                ],
            },
            {
                "company": "[Company]",
                "date_range": "Mar 2017 to Dec 2019",
                "roles": [
                    {
                        "title": "[Company] Liaison",
                        "date_range": "Mar 2017 to Dec 2019",
                        "bullets": ["Ran audits."],
                    }
                ],
            },
            {
                "company": "[Company]",
                "date_range": "Jan 2015 to Mar 2017",
                "roles": [
                    {
                        "title": "Accountant",
                        "date_range": "Jan 2015 to Mar 2017",
                        "bullets": ["Kept ledgers."],
                    }
                ],
            },
        ],
    }


def test_titles_and_dates_come_back_as_given_unless_one_names_an_employer() -> None:
    steady = guard.steady(ai_answer(), reading())
    assert steady["candidate"]["current_position"] == "Finance Lead", "the last position as formatted"
    roles = [e["roles"][0] for e in steady["work_experiences"]]
    assert roles[0]["title"] == "Finance Lead, GST, FM", "no stray full stop"
    assert roles[1]["title"] == "[Company] Liaison", "a title naming an employer stays masked"
    assert steady["candidate"]["name"] == "Candidate", "nothing else is copied back"


def test_the_checked_blind_cv_keeps_its_sub_headings_and_its_employers_apart() -> None:
    cv = cvstudio.after_blind(ai_answer(), reading())
    first = cv["work_experiences"][0]["roles"][0]["bullets"]
    assert first[0] == "Led the month-end close."
    assert isinstance(first[1], dict), "the flattened heading is a heading again"
    assert first[1]["heading"] == "Treasury", "without the AI's colon"
    assert first[1]["bullets"] == ["Cash forecasting.", "Bank reconciliations."]
    assert all(e.get("_keep_apart") for e in cv["work_experiences"])


def test_two_employers_both_hidden_are_never_joined_in_the_word_file() -> None:
    docx = cvstudio.render(cvstudio.after_blind(ai_answer(), reading()), alignment="left", bullet_levels=[])
    text = cvstudio.word_text(docx)
    assert "Mar 2017 to Dec 2019" in text, "each stint keeps its own dates"
    assert "Jan 2015 to Mar 2017" in text
    assert "Jan 2015 to Dec 2019" not in text, "not one joined stint"
    with zipfile.ZipFile(io.BytesIO(docx)) as z:
        assert b"Treasury" in z.read("word/document.xml")


def test_the_blind_prompt_carries_the_guardrails() -> None:
    for neutral in (False, True):
        prompt = cvstudio.blind_prompt(neutral)
        assert "Titles are never masked" in prompt
        assert 'Use "[Company]" only where an organisation or brand name stood' in prompt

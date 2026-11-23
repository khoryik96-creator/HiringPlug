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


def test_a_title_comes_back_only_when_the_ai_kept_its_words() -> None:
    steady = guard.steady(ai_answer(), reading())
    roles = [e["roles"][0] for e in steady["work_experiences"]]
    assert roles[0]["title"] == "Finance Lead, GST, FM", "only a stray full stop: as formatted"
    assert roles[1]["title"] == "[Company] Liaison", "a masked employer stays masked"
    assert steady["candidate"]["current_position"] == "Senior Finance Leader", "reworded: the AI's stays"
    assert steady["candidate"]["name"] == "Candidate", "nothing else is copied back"


def test_a_client_or_employer_the_ai_hid_in_a_title_never_comes_back() -> None:
    given, answer = reading(), ai_answer()
    given["work_experiences"][2]["roles"][0]["title"] = "SAP Lead for Kopi Example"  # a client
    answer["work_experiences"][2]["roles"][0]["title"] = "SAP Lead for [Company]"
    given["work_experiences"][1]["roles"][0]["title"] = "Analyst (Sampel)"  # the employer's short name
    answer["work_experiences"][1]["roles"][0]["title"] = "Analyst (Leading Bank)"
    given["candidate"]["current_position"] = "Kopi Example Lead"
    answer["candidate"]["current_position"] = "F&B Chain Lead"
    cv = cvstudio.after_blind(answer, given)
    titles = [e["roles"][0]["title"] for e in cv["work_experiences"]]
    assert "Kopi Example" not in " ".join(titles) + cv["candidate"]["current_position"]
    assert "Sampel" not in " ".join(titles)


def test_same_words_ignores_only_punctuation_case_and_spacing() -> None:
    assert guard.same_words("Finance Lead, GST", "finance lead GST.")
    assert not guard.same_words("SAP Lead for Kopi", "SAP Lead for [Company]")
    assert not guard.same_words("", "")


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
        assert "except an organisation, brand, school or person named in one" in prompt
        assert 'Use "[Company]" only where an organisation or brand name stood' in prompt


def test_a_mask_never_cuts_a_word_in_two() -> None:
    """ "Digital Consultant" at an employer named Digi stays "Digital Consultant"."""
    given, answer = reading(), ai_answer()
    given["work_experiences"][0]["company"] = "Digi Example Berhad"
    given["work_experiences"][0]["roles"][0]["title"] = "Digital Consultant"
    answer["work_experiences"][0]["roles"][0]["title"] = "[Company]gital Consultant"  # the AI cut it
    given["work_experiences"][0]["roles"][0]["bullets"][0] = "Ran Digital campaigns for Digi Example Berhad."
    answer["work_experiences"][0]["roles"][0]["bullets"][0] = "Ran [Company]gital campaigns for [Company]."
    cv = cvstudio.after_blind(answer, given)
    role = cv["work_experiences"][0]["roles"][0]
    assert role["title"] == "Digital Consultant"
    assert role["bullets"][0] == "Ran Digital campaigns for [Company].", "the employer itself stays masked"


def test_a_brand_named_product_stays_masked() -> None:
    assert guard.unglued("Launched [Company]Food and [Company]2u", "Launched GrabFood and Maybank2u") == (
        "Launched [Company]Food and [Company]2u"
    )
    assert guard.unglued("[Company]gital Consultant", "Digital Consultant") == "Digital Consultant"

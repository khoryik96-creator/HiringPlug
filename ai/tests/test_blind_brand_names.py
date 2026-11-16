"""Blinding a CV masks a company's name, never the start of an ordinary word.

CV Studio's flaw: Digi (a Malaysian telco) was masked inside "Digital",
leaving "[Company]tal". A name glued to more letters is a brand only when a
new capitalised word or a number follows it (DigiPrepaid, Maybank2u).
"""

from typing import Any

from hiringplug_ai import cvstudio

blind_mask: Any = vars(cvstudio)["blind_mask"]  # CV Studio's module, as the worker loads it


def _cv(company: str, bullet: str) -> dict[str, Any]:
    return {"work_experiences": [{"company": company, "bullets": [bullet]}]}


def _masked(text: str, cv: dict[str, Any]) -> str:
    terms = tuple(blind_mask._blind_collect_org_mask_terms(cv))
    return str(blind_mask._blind_replace_org_terms_in_text(text, terms))


def test_digi_never_eats_digital() -> None:
    cv = _cv("Digi Telecommunications", "Network engineer")
    out = _masked("Led digital transformation and Digital Marketing; Digitalisation; DIGITAL", cv)
    assert out == "Led digital transformation and Digital Marketing; Digitalisation; DIGITAL"


def test_the_company_and_its_branded_products_are_still_masked() -> None:
    cv = _cv("Digi Telecommunications", "Launched Digi Prepaid; built Maybank2u")
    out = _masked("Worked at Digi on DigiPrepaid and Digi Prepaid; Maybank2u rollout", cv)
    assert "Digi" not in out
    assert "Maybank" not in out
    assert "[Company]Prepaid" in out
    assert "[Company]2u" in out


def test_digital_alone_does_not_put_digi_on_the_mask_list() -> None:
    cv = _cv("Acme Sdn Bhd", "Digital marketing lead; digitalised payments")
    assert "Digi" not in blind_mask._blind_collect_org_mask_terms(cv)

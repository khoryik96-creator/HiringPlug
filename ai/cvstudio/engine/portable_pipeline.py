"""Provider-neutral assembly of CV Studio's existing deterministic stages.

New glue for the handover, not an original standalone CV Studio application.
No server, credential access, external service, or paid AI call is included.
"""
from __future__ import annotations

import copy
import json
from pathlib import Path
import re
import subprocess

import cvstudio_cv_normalize as normal
import cvstudio_cv_reconcile as reconcile
import cvstudio_blind_mask as blind
from cvstudio_cv_fidelity import evaluate_cv_fidelity, summarize_fidelity_warning
from cvstudio_docx_extract import _infer_label_bullet_levels, _merge_bullet_level_lists


def prepare_source(text: str) -> str:
    text = re.sub(r"\n{3,}", "\n\n", str(text or ""))
    text = re.sub(r"[ \t]{2,}", " ", text).strip()
    text = normal._cv_pretranslate_iso_dates(text)
    return normal._cv_pretranslate_year_first_month_names(text)


def postprocess_parse(parsed: dict, source_text: str, *, preserve_source_summary=False,
                      correct_language=False) -> dict:
    """Apply the deterministic portion of /parse in its original order.

    Supply the provider's parsed JSON and the source used for that parse.
    This helper doesn't perform the provider's language correction or retries.
    """
    parsed = copy.deepcopy(parsed)
    source = prepare_source(source_text)
    source_summary = blind._blind_source_summary_bullets(source) if preserve_source_summary else []
    selected_summary, removed = normal._cv_strip_pay_from_summary_counted(source_summary)
    candidate = parsed.get("candidate", {})
    spans = reconcile._reference_section_spans(source)
    for field, pattern in (
        ("email", r"[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}"),
        ("phone", r"[+\(]?[\d][\d\s\-\(\)]{7,}[\d]"),
    ):
        if not candidate.get(field):
            found = reconcile._search_outside_reference_sections(pattern, source, spans)
            if found:
                candidate[field] = found.group(0).strip()
                parsed["candidate"] = candidate
    parsed = normal._correct_mistagged_candidate_name(parsed, source)
    parsed = reconcile._reconcile_work_experience_with_authoritative_table(parsed, source)
    parsed = reconcile._order_same_company_roles_newest_first(parsed)
    parsed = reconcile._restore_explicit_project_headings(parsed, source)
    parsed = reconcile._attach_untitled_subsidiary_entries(parsed, source)
    parsed = reconcile._collapse_incomplete_earlier_career(parsed)
    parsed = reconcile._clean_candidate_languages_from_redaction(parsed, source)
    parsed = normal._normalize_candidate_languages(parsed, source)
    parsed = reconcile._drop_reference_sections(parsed)
    parsed = reconcile._restore_labelled_company_qualifiers(parsed, source)
    levels = _infer_label_bullet_levels(parsed)
    parsed = normal._normalize_cv_structured_content(parsed)
    parsed = normal._normalize_cv_data_for_output(parsed, source)
    if source_summary:
        corrected = blind._blind_preserve_summary_corrections(
            parsed, selected_summary, correct_language=correct_language)
        parsed["summary_bullets"] = source_summary
        parsed = blind._blind_prepare_summary_bullets(parsed)
        parsed["summary_bullets"] = corrected
    if isinstance(parsed.get("summary_bullets"), list):
        parsed["summary_bullets"], count = normal._cv_strip_pay_from_summary_counted(parsed["summary_bullets"])
        removed += count
    audit = evaluate_cv_fidelity(parsed, source)
    result = dict(ok=True, data=parsed, bullet_levels=levels, fidelity=audit,
                  summary_pay_removed=removed)
    if not audit.get("ok"):
        result.update(degraded=True, degraded_reason="fidelity_check", warning=summarize_fidelity_warning(audit))
    return result


def postprocess_blind(provider_blinded: dict, original_parsed: dict) -> dict:
    """Deterministic final checks AFTER an anonymizing provider response.

    This is not a replacement for the blind system prompt. Review the result;
    organization and identity masking is bounded, not a de-identification proof.
    """
    value = blind._blind_restore_cv_bullet_structure(copy.deepcopy(provider_blinded), original_parsed)
    value = blind._blind_finalize_summary_bullets(value, original_parsed)
    value = blind._blind_postprocess_company_mentions(value, original_parsed)
    value = blind._blind_scrub_candidate_identity(value, original_parsed)
    return normal._normalize_cv_structured_content(value)


def prepare_export(data: dict, *, alignment="left", summary_box_autofit=True,
                   bullet_levels=None) -> dict:
    """Mirror normal /generate-docx preparation; preserve preview work order."""
    value = copy.deepcopy(data)
    inferred = _infer_label_bullet_levels(value)
    value = reconcile._drop_reference_sections(value)
    value = normal._normalize_cv_structured_content(value)
    value = normal._normalize_cv_data_for_output(value, preserve_work_order=True)
    if isinstance(value.get("summary_bullets"), list):
        # The original request boundary bounds input, then applies the pay filter.
        bullets = [item[:20000] if isinstance(item, str) else item
                   for item in value["summary_bullets"][:60]]
        value["summary_bullets"] = normal._cv_strip_pay_from_summary(bullets)
    value["_document_alignment"] = "justify" if str(alignment).strip().lower() == "justify" else "left"
    value["_summary_box_autofit"] = summary_box_autofit is not False
    source_levels = bullet_levels if isinstance(bullet_levels, list) else value.get("_bullet_levels", [])
    value["_bullet_levels"] = _merge_bullet_level_lists(source_levels, inferred)
    return value


def render(data: dict, output: Path, *, work_folder: Path) -> None:
    """Write a DOCX with the unchanged Node renderer and sanitized template."""
    folder = Path(work_folder).resolve()
    folder.mkdir(parents=True, exist_ok=True)
    payload = folder / "render-input.json"
    payload.write_text(json.dumps(data, ensure_ascii=False), encoding="utf-8")
    subprocess.run(["node", str(Path(__file__).with_name("generate.js")),
                    str(payload), str(Path(output).resolve())], check=True)

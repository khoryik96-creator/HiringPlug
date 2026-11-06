# Integrating the rules into another ATS

## What to transplant directly

| Component | Supplied file | Integration contract |
|---|---|---|
| Parsing rules | `prompts/SYSTEM_PROMPT.txt` | Raw source text → canonical CV JSON, with source-supported facts only |
| Mechanical language correction | `prompts/CV_LANGUAGE_CORRECTION_INSTRUCTION.txt` | Replace the base typo line only when the user enables the toggle |
| Normalization | `engine/cvstudio_cv_normalize.py` | Pure functions of JSON and optional source text |
| Source reconciliation | `engine/cvstudio_cv_reconcile.py` | Pure structured corrections using authoritative source evidence |
| Fidelity and source-grounded review | `engine/cvstudio_cv_fidelity.py` | Nonmutating fidelity report; validated signed review operations |
| Blind CV | `prompts/BLIND_SYSTEM_PROMPT.txt`, `engine/cvstudio_blind_mask.py` | Provider transformation plus deterministic structural and identity checks |
| Summary | `engine/vendor/cvstudio/candidate-summary.js`, `prompts/SUMMARY_*.txt` | Supported summary bullets, optional blind mode, deterministic pay removal |
| DOCX extraction and list depth | `engine/cvstudio_docx_extract.py` | Bounded bytes → source text / bullet-level metadata |
| Document limits and OCR primitives | `engine/cvstudio_document_safety.py` | Provider-independent bounded helpers; converter/OCR dependencies are injected |
| Word renderer | `engine/generate.js`, `engine/template.docx` | Prepared JSON + compatible template → DOCX |
| Provider-neutral assembly | `engine/portable_pipeline.py` | New handover glue demonstrating the existing processing order |

The four core rule modules do not import Flask or the original application. They are the easiest reuse boundary. For a TypeScript/Node ATS, retain them behind a Python service initially, or port their behaviour with the supplied parity and regression tests. The Node renderer can run as a restricted server-side child process in either architecture.

## Boundaries the new ATS must supply

### LLM provider

Implement your own server-side provider adapter, keyed by the ATS's saved provider/model settings. The original route excerpts reference `call_llm`, `_resolve_request_api_key`, usage/cost reporting and request guardrails; those are references to CV Studio services, not included working APIs.

Normalize provider responses into text + usage + finish/truncation information. The original parse route uses deterministic temperature zero and a high requested output budget, then explicit parse/retry/continuation handling. Apply the actual chosen model's token ceiling. Preserve incomplete/truncated/paid-failure reporting. Do not blindly repeat a paid request after a network ambiguity or auto-submit a CV whose fidelity warning has not been reviewed.

The bundle needs no API key to run. No provider was called to create the sample.

### Request contract

You may use different endpoint URLs. Preserve the behaviour rather than copying all of the original application wiring.

| Original endpoint | Important input | Important output/behaviour |
|---|---|---|
| `POST /extract-text` | Uploaded document; optional preview/OCR choices | Extracted source, list-depth metadata and extraction errors/partial results; exact branching is in the extraction reference |
| `POST /parse` | `cv_text`, provider/model, `auto_correct_language`, optional `preserve_source_summary` | `data`, `bullet_levels`, `fidelity`, optional degraded/warning and summary-pay-removal fields |
| `POST /blind` | Parsed `cv_data`, provider/model, optional `neutralize_candidate_gender` | Same core CV shape after provider anonymization and deterministic sweeps |
| `POST /generate-ai` | Feature-specific prompt/settings; summary uses `strip_candidate_pay` | Generated text, usage and summary removal metadata; formatting-review handling also lives here |
| `POST /generate-docx` JSON | `data`, `alignment`, `summary_box_autofit`, `bullet_levels`, optional review proof | Binary DOCX; candidate-pay-removal count may be returned in `X-CV-Summary-Pay-Removed` |
| `POST /generate-docx` multipart | Existing DOCX + selected summary bullets and shape setting | Inserts/replaces a summary in the existing file without reparsing and regenerating the whole CV |

See the original route excerpts for complete details. Do not expose raw provider or CV data in an ATS diagnostic endpoint merely because the reference has development-oriented error paths.

### File conversion and uploads

The portable DOCX extractor delegates ZIP bounds to the supplied document-safety module. The broader original route supports PDF/image OCR and legacy document conversion through external libraries/processes. Supply equivalents for your hosting platform. Keep upload limits, page/time/image bounds, validated Office ZIP entry limits and converter deadlines.

The reference includes original Windows Word/LibreOffice and trusted Antiword boundaries. The new ATS needs its own managed converter installation and trust policy. Do not transplant an owner-machine executable path, installation receipt, Authy material or the protected Windows binary.

### Security, storage and jobs

Use your ATS's own authentication, per-tenant/candidate access control, CSRF protection, storage lifecycle, audit records and cancellation. Formatting modules do not supply a multi-tenant security boundary. Keep provider keys and formatting-review signing keys on the server. A source-bound review approval must be validated before applying any correction and before rendering an approved changed output.

Retain source text/file, canonical parsed JSON, warning/audit result, the selected summary, source list-depth map and output version together. Do not identify a CV only by a mutable global “current candidate.” Long-running/batch processing must retain each file's own source, model, summary and output state.

## Suggested implementation sequence

1. Add a canonical model and a versioned per-CV record. Run normalization and schema/example tests before adding provider calls.
2. Port/adapt bounded extraction. Test nested tables, in-cell controls/textboxes, numbering and source whitespace.
3. Connect parsing to your existing provider adapter using the exact prompt rules; test complete and truncated responses with mocked provider outputs.
4. Apply the deterministic parse order from `portable_pipeline.postprocess_parse`. Compare its output to the original route excerpts. Retain fidelity/warnings and block automatic downstream create/upload for degraded results.
5. Add preview/editing around the same JSON. Keep grouped headings and source list depth. Preserve edited work order at export.
6. Reuse the Node renderer with the supplied compatible template. Match the fictional DOCX before changing design, fonts or branding. Test Word on the target platform.
7. Add summary selection/generation and candidate-pay filtering across preview/export. Add summary insertion into an existing DOCX if required.
8. Add the blind prompt and deterministic sweeps; test that masking never restores the original unblinded strings into a corrected structure.
9. Add source-grounded formatting review with explicit user approval and your own server-held signing key. Reject expired/stale/changed approvals.
10. Integrate batch runs and ATS upload/create only after these individual stages agree.

## Acceptance checklist

- Every supported employer, role, project group, duty and qualification survives a successful parse and export.
- Labelled table employers are never replaced by separators, locations, universities or project clients.
- Dated roles remain dated; year-only precision is retained; separate employer stints stay separate.
- Explicit source subsections and manual preview order survive export.
- Nested bullets and one-child headings stay distinguishable in the preview and Word XML.
- Reference sections disappear, but legitimate technical reference skills remain.
- Candidate contacts do not come from referees; notice periods stay verbatim.
- Education CGPA/major/honors/descriptions survive without invented degree labels.
- Summary salary is removed while payroll/compensation-system duties remain.
- A blind result contains no original direct identifiers reintroduced by structure restoration.
- A truncated or low-fidelity result is visibly flagged and does not automatically create/upload a misleadingly complete ATS record.
- Long summaries respect both Word drawing and VML geometry; details are not pushed out of the first-page design area.
- The output DOCX is valid XML/ZIP and uses the same selected content as preview.

## Known boundaries in this handover

The backend route excerpts intentionally have unresolved application services. `reference/EXTERNAL_DEPENDENCIES.json` lists globally referenced names not defined within each excerpt; some are imported original modules and some are adapters you must provide. The portable glue is a demonstration of the deterministic layer, not a replacement for those full production routes, paid-call controls, OCR jobs or approval security.

The cleaned template and example retain Hyppies branding and renderer-generated legal terms. These are configurable design/business assets, not universal ATS rules. Do not claim pixel-identical Microsoft Word output from the included Linux LibreOffice PDF; pagination and fonts need target-platform checks.

## Inherited details to review during the port

- The original prompt's illustrative schema contains unescaped quotes inside some explanatory example strings. It is prompt prose, not a parseable JSON Schema. Use the supplied valid JSON contract; you can clean the illustrative quoting in your ATS prompt without changing the rules.
- The original missing-phone fallback is a broad numeric regex. When a CV has no telephone number, it can misread a date range such as `2015 - 2019` as a telephone number. The reference and demonstration preserve that original code; the example includes an explicitly fictional phone. In the ATS, validate telephone candidates against date-like patterns and source ownership before accepting a fallback. This is an inherited implementation limitation, not an intended formatting rule.
- Some renderer comments label half-point Word XML values as full points. Follow the actual values and the measurements table in FORMATTING_RULES.md.

# The AI worker (Python)

`ai/` processes jobs on the `ai` queue. It reads CVs today; parsing,
formatting, blind CVs, matching and the assistant are designed below. It never
touches business tables: it receives a job, does the work, and returns a result
job that the Go worker saves.

```
ai/src/hiringplug_ai/
  worker.py          the loop: claim → validate payload → handler → complete (or fail)
  queue.py           calls the shared SQL job functions; this role can do nothing else
  jobs/              one module per job kind (extract_text.py today)
  extraction/        reading documents: sniff, pdf, ocr, docx_reader, zipsafety, text, limits
  contracts.py       GENERATED models from contracts/openapi.yaml
```

## Reading CVs (built)

The mechanisms come from **CV Studio**, where they were tuned on real CVs,
rebuilt here as small, separately tested modules.

1. **Identify the file from its bytes**, never from its name or claimed type
   (`sniff.py`). PDF and Word (.docx) today; images and old `.doc` (via antiword)
   next.
2. **PDF, page by page** (`pdf.py`):
   - read each page's text layer (pdfplumber);
   - decide per page whether to OCR it. *Unextractable*: the text is glyph
     noise such as `(cid:415)`, control codes or replacement characters, from
     fonts without a Unicode map. *Sparse-visual*: little text, but the page is
     mostly an image (a scan) or dense vector outlines (text turned into shapes);
   - a short page with no visual evidence is **not** OCR'd: short contact or
     skills pages are real, and OCR could damage emails, phone numbers or "C++";
   - OCR only the chosen pages; for each, keep the exact text unless OCR found
     clearly more (at least 1.5× and 10 more letters);
   - **never lose content silently.** Pages OCR could not read become a warning
     on the document ("OCR could not read page 3 ..."); visibly blank pages
     don't count.
3. **OCR** (`ocr.py`): pages are rendered with PDFium at 220 DPI and read by
   Tesseract, with limits on pages (30), pixels, per-page time and total time.
4. **Word** (`docx_reader.py`): page headers first (names often live there),
   list paragraphs keep a `•` marker (including numbering inherited from
   styles), tables keep their rows and multi-line cells, and text boxes are read
   where they appear, with Word's duplicate fallback copies removed.
5. **Safety** (`limits.py`, `zipsafety.py`): page limits, zip-bomb checks for
   Word files, pixel limits for images. A damaged or unsafe file becomes a
   `failed` result with a readable message, not a retry loop.
6. **Clean-up** (`text.py`): known ligature codes are fixed
   (`Accoun(cid:415)ng` → `Accounting`), NUL characters (which PostgreSQL cannot
   store) are removed, and whitespace is tidied. Real words are never rewritten.

**Improvements over CV Studio:**

- **Sideways and upside-down scans are fixed.** A page that OCR reads as almost
  no words is read again turned 90, 180 and 270 degrees, and the clearly best
  reading wins.
- **Scale.** OCR runs in worker processes fed by the queue. Instead of one OCR
  at a time inside the web server, you add workers.
- **Visible outcomes.** Status (`extracted`, `needs_ocr`, `failed`), the
  extractor and version (`pdfplumber@0.11.10+tesseract@5.3.4`) and warnings are
  stored on the document, so a recruiter can see why a CV looks incomplete.

Tests build every document in code (text PDFs, scans, sideways scans, mixed
PDFs, Word files with headers, tables and text boxes), so no real CV with
personal data is ever committed.

## CV parsing (M2): grounded in the source

The AI turns the text into a structured profile: contact details, work history
(company, title, dates, duties), education, certifications, skills, languages,
and current/expected salary and notice period when stated.

The difference from CV Studio: CV Studio fixes AI mistakes with a long chain of
special-case rules, each patched after a real CV broke it. HiringPlug uses one
general rule instead:

- **Every extracted value must carry the exact text it came from** (a quote
  from the CV), using the model's structured-output mode with a schema.
- **A deterministic checker** confirms each quote exists in the CV text. Values
  without a matching quote are dropped or flagged for review. This catches
  invented employers, wrong dates and mixed-up sections in one place.
- Values are **suggestions**: the recruiter accepts them into the profile.
  Nothing silently overwrites what a person entered.
- Each result records the model and prompt version, so it can be re-run when
  models improve.

**Quality has its own regression test:** a set of anonymised sample CVs with
expected results runs in CI. Ordinary tests catch crashes; this catches "the
parser got worse". It replaces CV Studio's written QA reports with automatic
checks.

**Two-column CVs** (a sidebar next to the main column) are the cause of many
CV Studio bugs, because PDF text comes out with the columns interleaved. M2
adds column-aware reading order (detecting the gap between columns from word
positions). It needs a set of anonymised sample CVs to validate against.

## CV formatting and blind CVs (M4)

- **Templates are Word files with placeholders** (`{{ candidate.name }}`,
  `{% for job in experience %}`) rendered with docxtpl. Each company edits its
  own branded template in Word, uploaded in the app. CV Studio instead uses one
  built-in template filled by code.
- The formatted CV is generated **from the structured, recruiter-checked
  profile**, not by editing the original file, and saved as a `formatted`
  document. PDF versions come from LibreOffice running headless.
- **Blind CVs** leave identity fields out of the data before rendering (name,
  contacts, photo, address, optionally current employer). An automatic leak
  check then searches the rendered text for the candidate's name, email, phone
  and profile URLs. This is more reliable than redacting free text.
- Options per export: hide contact details, hide salary, blind, include the
  recruiter's summary.

## CV-to-job matching (M4)

A score with reasons. Every reason quotes the CV and the job ad, using the same
source-grounding check as parsing. Embeddings (stored in PostgreSQL with
pgvector) shortlist candidates for a job; the model explains the top matches.
It suggests; recruiters decide.

## AI assistant sidebar (M4)

See [features.md](features.md#35-ai-interpretation-sidebar-deepseek): the
context is built by Go from what the user may see, the provider is a setting
per company (DeepSeek through a host and region you choose, or another
provider), contact details are removed, and usage and cost are tracked with
limits.

## Adding a job to the AI worker

1. Declare it in the module contract: `x-jobs: { thing.do: { queue: ai, payload: ThingDoJob } }`
   with the payload schema; run `task gen`.
2. Add `jobs/thing_do.py` with `run(payload, deps) -> list[FollowUp]`.
   Return the result job(s); don't write to the database.
3. Register it in `jobs/__init__.py` (`HANDLERS`). The contract-wiring tests
   fail until you do.
4. Test the logic with plain unit tests, and the handler with the fakes in
   `tests/test_jobs.py`.

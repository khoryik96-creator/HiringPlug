# The AI worker (Python)

`ai/` processes jobs on the `ai` queue. It reads and parses CVs today;
formatting, blind CVs, matching and the assistant are designed below. It never
touches business tables: it receives a job, does the work, and returns a result
job that the Go worker saves.

```
ai/src/hiringplug_ai/
  worker.py          the loop: claim → validate payload → handler → complete (or fail)
  queue.py           calls the shared SQL job functions; this role can do nothing else
  jobs/              one module per job kind (extract_text.py, parse_cv.py)
  extraction/        reading documents: sniff, pdf, ocr, docx_reader, zipsafety, text, limits
  parsing/           reading profile details out of CV text: layout, dates, contacts, history, skills
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

## CV parsing: grounded in the source

**Built (rules@1).** When Go saves a CV's text it queues `document.parse_cv`
with that saved text (formatted CVs are skipped). The worker returns
`document.cv_parsed`: contact details, nationality, notice period, skills,
work history (title, company, dates, current or not) and education.

**One rule for every reader: values must quote the CV.** Each value is a
`{value, quote}` pair. Before saving, Go (`documents/grounding.go`, rules in
`domain.go`) keeps a value only if

- its quote is in the CV text (ignoring case and spacing), and
- the value can be read from the quote: the same letters and digits in order
  ("Siti Aisyah" from "SITI AISYAH", "Excel" from "Excel (Advanced)"), or a
  fixed format for coded fields (country code, "1 month").

Everything else is dropped and counted (`dropped`), so invented employers,
wrong dates or values from the wrong line never reach a recruiter. The check
does not care who produced the value, so the AI model added later (DeepSeek,
once its host and region are chosen) goes through the same gate: rules
handle what patterns read best (emails, phones, links, dates), the model
fills what they miss (messy work history), and both are checked. Values are
**suggestions**: the recruiter reviews them next to the profile and ticks
what to use; nothing a person entered is replaced unless they tick it.

**The rules** (`parsing/`) come from Kano's CV scanner, rebuilt as small
tested modules:

- `layout.py`: lines and sections (English and Malay headings, headings with
  content on the same line such as "Skills: Excel, SAP").
- `contacts.py`: email; phone (labelled numbers first; never a year range such
  as 1999-2001; never joined across lines; bare 8-digit runs only when they look
  like Singapore mobiles); LinkedIn; name ("Name:" labels, JobStreet exports,
  a name sharing the first line with a phone, ALL CAPS to Title Case, OCR's
  "BINT]" for "BINTI"); location; nationality; notice period.
- `dates.py`: "Jan 2020 – Present", "03/2019 - 12/2021", "2015 to 2018",
  Malay month names.
- `history.py`: a job is anchored on its date range; title and company come
  from the same line ("Title | Company | dates", "Title at Company") or the
  short lines around it, told apart by company words (Sdn Bhd, Pte Ltd,
  Berhad, ...) and role words. Education by qualification and institution
  words.
- `skills.py`: items of the skills section only (never guessed from prose),
  without levels ("Excel (Advanced)" → Excel), languages or personal facts.

Each result records the parser version (`rules@1`), and **Read details
again** re-runs it, so CVs can be re-parsed when the parser improves.

**Next:**

- **A quality set**: anonymised sample CVs with expected results, run in CI,
  so a rule change that makes parsing worse fails the build. This needs real
  CVs (10–30, including two-column and scanned ones).
- **Two-column CVs** (a sidebar next to the main column) come out of PDFs with
  the columns interleaved. Column-aware reading order (finding the gap
  between columns from word positions) needs the same sample set to validate.
- **The AI reader**, behind the same quote check.

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

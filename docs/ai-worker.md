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
   (`sniff.py`). PDF, Word (.docx) and old Word (.doc); images next.
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
5. **Old Word (.doc)** (`doc_reader.py`): antiword first (small and fast);
   files it cannot read (other Word versions, files saved by other programs)
   are converted by LibreOffice to .docx and read by the Word reader. Each
   tool runs in its own temporary folder with a time limit.
6. **Safety** (`limits.py`, `zipsafety.py`): page limits, zip-bomb checks for
   Word files, pixel limits for images. A damaged or unsafe file becomes a
   `failed` result with a readable message, not a retry loop.
7. **Clean-up** (`text.py`): known ligature codes are fixed
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
with that saved text (formatted CVs too, for comparing with the original). The worker returns
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

**The rules** (`parsing/`) come from Kano's CV scanner and CV Studio's
normaliser, rebuilt as small tested modules and tuned on 25 real CVs
(JobStreet and LinkedIn exports, Word and PDF, designer two-column layouts,
old .doc files):

- `layout.py`: lines and sections. English and Malay headings, plurals
  ("Experiences"), letter-spaced headings ("C ON TA CT"), headings with content
  on the same line ("Skills: Excel"; JobStreet's "Career history Senior
  Engineer at ..."). Page footers and page numbers are dropped. One-word
  headings that also end company names ("... at Dell" + "Technologies") count
  only in capitals or with a colon.
- `names.py`: candidates from the top of the CV (a line, the start of a line
  shared with contacts or a title, a labelled value), checked against CV
  Studio's rules (no digits, "@", commas, address words or places) and against
  job titles and company words. The candidate that shares words with the email
  address or LinkedIn link wins. Honorifics ("Ts.", "Dr."), letters after the
  name (", PMP"), nicknames ("(Aaron)") and OCR's "BINT]" are handled.
- `contacts.py`: email; phone (labelled anywhere, including a label on the line
  above; never a year range; never joined across lines; bare 8-digit runs only
  as Singapore mobiles); LinkedIn; location; nationality; notice period.
- `dates.py`: "Jan 2020 – Present", "03/2019 - 12/2021", "2015 to 2018",
  year-first "2022-Oct", days of the month, "Dec – Jan 2026", two-digit
  years, "till date", Malay months. CV Studio's date cases are the test.
- `experience.py`: a job is anchored on its date range; title and company come
  from the same line ("Title | Company", "Title at Company", "Company - Title
  (dates)", "Title, Dept | Company, City"), labelled lines ("Company:",
  "Role:"), or the short lines around it, before or after the dates as the CV's
  first job shows. Places, durations ("(2 years 3 months)"), numbering,
  "Employer client: X", duties after a title and dates broken over two lines
  are handled. A dated line under another heading counts when it plainly holds
  a title and a company (sidebars and page breaks move work history).
- `education.py`, `skills.py`: by qualification and institution words; skills
  from skills sections only, without levels, rating bars, prose or job lines,
  keeping brackets together ("AWS (EC2, EKS, S3)").

**Two-column PDFs** (`extraction/columns.py`): a page is split at the gap
between its columns, found from word positions, and each column is read top
to bottom (the one that starts higher first). It splits only when almost no
line crosses the gap and both sides run down much of the page with real text,
so right-aligned dates on a one-column CV keep the page whole.

**Quality.** The real CVs stay private (they are never committed). Every
layout they showed is copied into an invented CV in `tests/test_parsing_layouts.py`
and `tests/test_columns.py`, so a rule change that breaks one fails the build.

Each result records the parser version (`rules@1`), and **Read details
again** re-runs it, so CVs can be re-parsed when the parser improves.

**Next:** the AI reader (DeepSeek, once its host and region are chosen),
behind the same quote check, for what rules still miss: unusual layouts and
very messy two-column CVs.

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

# The AI worker (Python)

`ai/` processes jobs on the `ai` queue: it reads and parses CVs, renders
PDFs, answers the assistant and scores CV vs job fit. It never
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
7. **Hidden links** (`links.py`, from Kano): addresses behind links in the
   file (PDF link annotations, Word hyperlinks) are added at the end of the
   text under "Links in the file:", LinkedIn first, unless the text already
   shows them, so a CV whose "LinkedIn" icon links to the profile still gives
   its address (and the parser can quote it).
8. **Clean-up** (`text.py`): known ligature codes are fixed
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
- `residence.py`: country of residence (address, places near the top, phone's
  country code); `industry.py`: the latest employer's industry (Kano's
  industries; known names, then words, then the title), quoted with the words
  it came from (Go checks the quote is in the CV; the value is an inference).
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

**Kano's detection, with the AI (built; used once the AI is on).** The rules
read every CV first. If they find the five core details (name, email, phone,
current title and company), the full AI read is skipped, as in Kano, and only
the latest employer's industry is asked for when the rules found none (Kano's
industry prompt and list). Otherwise the AI fills what the rules left empty
(`jobs/cv_ai.py`), never replacing what they found. Every AI value carries
its quote and goes through the same check in Go, so an invented value is
dropped like any other; an industry outside Kano's list is dropped in Python.
Go offers the AI only while it is on and the company has allowance left
(`useAi` on the job), with a copy of the text from which salary figures are
taken out (`aiText`, `redact.WithoutSalary`): salary never goes to the AI.
Unlike the assistant, the CV reader does send names and contact details, since
reading them is its job. An unexpected error in the AI step keeps the rules'
reading. A job title only counts towards the industry for IT work (as Kano's
ERP rules), and only header lines that read like an address set the country
of residence; the tokens it used come back and count in the one
monthly allowance the assistant and matching share. The parser then reads
`rules@1+ai`.

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

## CV-to-job matching (built)

`match.score` carries the job's text (title, details, description and its
best ad) and the CV's text, both already without names, contact details or
salary. `jobs/match_score.py` asks for at most 10 requirements as JSON, each
copied word for word from the job text with a verdict (met / partly / missing)
and evidence copied from the CV, plus a one-line summary, and, as CV Studio's
CV scoring does, a 0-100 rating with a note on each of six dimensions (skills,
experience, industry, education, leadership, keywords; CV Studio's own names
are understood too) and up to 3 strengths and gaps. It only reads the
answer into the contract's shape (unknown verdicts and ratings skipped, scores
such as "85/100" read and held to 0-100, values clipped,
code fences and chatter around the JSON tolerated); an answer that is not the
JSON asked for becomes a readable error, with its tokens still counted.

Go trusts none of it (`matching/domain.go`): a requirement must quote the job
text and a met or partly verdict must quote the CV (`platform/quote`, the
check CV parsing uses), or it counts as not shown; requirements about age,
gender, race, religion, marital status or nationality are dropped; the score
is worked out from what passes (met 1, partly ½). It suggests; recruiters
decide.

Scores are asked for automatically too (on joining a job, on a new CV), and
marked outdated when the job's text changes (`job_match.job_hash`). Possible
candidates for a job come from a PostgreSQL full-text search of CVs
(`cv_search`, kept by a trigger), not from the model.

**Next:** embeddings (PostgreSQL with pgvector) to find possible candidates by
meaning; the model then explains the top ones the same way.

## AI assistant (built)

`assistant.ask` carries the question and the context Go gathered (sections of
text, already without names, contact details or salary). `jobs/assistant_ask.py`
builds the prompt (one system prompt, one task per kind) and calls the
provider; the answer, the model and the tokens go back as
`assistant.answered`. A provider failure becomes a readable error on the
answer ("the AI provider did not answer in time"), not a retry loop.

`providers/` is the one interface (`complete(system, user, max_tokens)`).
AI jobs carry the provider the company chose (`ai`: provider, model, sealed
key); `Deps.provider` opens the key (`keys.py`, `HP_AI_KEYS_SECRET`) and
builds `providers/messages.py` (Claude, and DeepSeek's Anthropic-compatible
endpoint, with Anthropic's SDK) or `providers/responses.py` (ChatGPT, OpenAI's
Responses API). A job without `ai` uses the server's own provider,
`OpenAICompatible` (DeepSeek's API, hosts of its open model, self-hosted
servers): `HP_AI_PROVIDER` (off / openai_compatible), `HP_AI_PROVIDER_URL`,
`HP_AI_PROVIDER_KEY` (never logged or printed), `HP_AI_MODEL`,
`HP_AI_TIMEOUT_SECONDS`. `aiprovider.check` sends a provider a tiny request
when a company saves a key and reports back as `aiprovider.checked`. Paid
requests are sent again only on 429/529; provider error texts (which can echo
part of a key) are replaced by fixed messages. See
[features.md](features.md#35-ai-assistant-sidebar--off-until-an-ai-provider-is-set-up).

## Adding a job to the AI worker

1. Declare it in the module contract: `x-jobs: { thing.do: { queue: ai, payload: ThingDoJob } }`
   with the payload schema; run `task gen`.
2. Add `jobs/thing_do.py` with `run(payload, deps) -> list[FollowUp]`.
   Return the result job(s); don't write to the database.
3. Register it in `jobs/__init__.py` (`HANDLERS`). The contract-wiring tests
   fail until you do.
4. Test the logic with plain unit tests, and the handler with the fakes in
   `tests/test_jobs.py`.

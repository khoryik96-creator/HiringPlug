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
  extraction/        reading documents: sniff, pdf, ocr, image, docx_reader, zipsafety, text, limits
  parsing/           reading profile details out of CV text: layout, dates, contacts, history, skills
  contracts.py       GENERATED models from contracts/openapi.yaml
```

## Reading CVs (built)

The mechanisms come from **CV Studio**, where they were tuned on real CVs,
rebuilt here as small, separately tested modules.

1. **Identify the file from its bytes**, never from its name or claimed type
   (`sniff.py`). PDF, Word (.docx), old Word (.doc) and pictures of a CV
   (PNG, JPEG, TIFF, BMP, WEBP; `image.py`: turned upright as the camera
   recorded, read in greys, enlarged under 1,600 pixels wide, read turned when
   sideways, with Tesseract). Text files, RTF and OpenDocument text never
   reach the worker: Go reads them at upload (`platform/filekind`), as CV
   Studio accepts them, but following each format (its RTF and ODT readers
   strip markup with a pattern, which leaves font names and joins
   paragraphs).
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
  country code). Employer-sector classification runs in the Go industries
  module after extraction/grounding; see [industries.md](industries.md).
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

**Rule extraction with an optional AI supplement.** Python reads each CV first.
If the core contact details and employer evidence are already present, no AI
request is made. Otherwise `jobs/cv_ai.py` fills missing contact details and,
when no jobs were extracted, keeps explicitly associated title/company evidence
as undated employment. It never asserts current employment without evidence or
fills a first-listed historical job with a different employer's details.
Every returned value carries an exact quote checked by Go. An unexpected AI
error leaves the local extraction available.

Go selects the current/latest dated employer and recognises its business using
the agency's shared industry catalogue. Unknown, concurrent and undated cases
stay available for manual review. Python no longer classifies industries or
makes an industry-only request. A job title alone never establishes a sector.

Go offers AI extraction only when enabled and within the agency's allowance,
using a copy with salary removed (`aiText`, `redact.WithoutSalary`). The reader
receives identity/contact information because extracting it is its purpose;
salary never goes to the provider. Token usage counts towards the shared
monthly allowance. The parser records `rules@1+ai` when AI added evidence.

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

### CV Studio's way (built: `cv.format_studio`)

The other way to make a formatted CV: **from one of the candidate's own CVs**,
with CV Studio's engine, vendored unchanged in `ai/cvstudio` (its rule modules,
prompts, Node renderer and template; `src/hiringplug_ai/cvstudio.py` is the
only code that touches it). In CV Studio's own order:

1. The CV's text is read (PDF, Word or old .doc, with OCR), and **lines stating
   the candidate's own pay are removed first** (CV Studio's pay filter, line by
   line: payroll duties and other figures stay), so salary never reaches the AI
   or the formatted CV.
2. The AI reads it into CV Studio's structure with CV Studio's prompt. With
   "correct spelling" the prompt's spelling line is swapped for CV Studio's
   fuller language-correction instruction, as CV Studio does.
3. CV Studio's rules tidy and reconcile it with the text and run its fidelity
   check; anything that may be lost or changed comes back as a **warning** the
   recruiter sees ("compare it with the CV before sending").
4. For a blind CV, exactly as CV Studio's `/blind` route: the reading in step 2
   keeps the CV's own About / Summary section as written (CV Studio's
   `preserve_source_summary`); the summary is prepared (taken from an About /
   Summary group of skills when there is no other) and lines stating pay are
   removed from it; the AI is sent "Anonymise this parsed CV JSON:" and the
   prepared CV with CV Studio's blind prompt (optionally "the candidate"
   instead of he or she); then CV Studio's five checks run in its order
   (restore structure, summary, employers, the candidate's identity,
   normalize) against the prepared CV. When CV Studio refuses an answer (a
   summary bullet lost or still naming the candidate) its own reason is shown.
5. CV Studio's renderer (`node generate.js`, a time limit) writes the Word file,
   with the Word file's own bullet depth when the source was Word.
6. A blind file is checked as the client would open it, twice: the AI worker
   runs CV Studio's identity sweep over all of the file's text (body,
   headers, footers, properties, link addresses; runs joined as Word shows
   them) and refuses it when the sweep would still change anything; Go then
   checks it for the candidate's name, email and phone as HiringPlug holds
   them, by the same name rules (below), and never saves one that shows them.
   File names follow CV Studio's ("Hyppies CV - Name.docx"): "Contoh
   Recruitment CV - Siti Aisyah.docx", and for a blind CV "Contoh Recruitment
   CV - Candidate C-1A2B3C (Blinded).docx" (CV Studio keeps the real name
   there; the reference stands in for it so a file sent to a client does not
   name the candidate).

**About Him / Her.** As CV Studio does while formatting (cv-format.js,
`requestFormattingSummary`), the summary box is written by the AI from the
CV unless the person chooses "Only the CV's own summary": normal (6-7
points) or detailed (8-10), with CV Studio's own prompt and its reading of
the answer, both run from its unchanged `candidate-summary.js` by Node
(`engine/summary_cli.cjs`), sent alone, 1,000 tokens; lines stating pay are
then removed. Never for a blind CV (it keeps the CV's own summary). Without
it the template's `*Summary*` placeholder was left in the box.

**Valid Word files.** CV Studio's renderer writes some settings in an order
the Office Open XML schema forbids, and the privacy-cleaned template keeps
empty document properties: LibreOffice opens such files, Word says "Word
found unreadable content". `docx_repair.py` puts them in the schema's order
after rendering (no text changes); Microsoft's Open XML SDK validator found 61
errors in a formatted CV before and none after. The built-in template of
CVs made from the profile had the same fault in its styles (fixed).

**Names follow CV Studio's rules everywhere** (`platform/redact`, used by the
blind checks, blind CVs from the profile, blind job ads and what the AI
assistant and matching see). The whole name is found in any case; a single
name word only where it reads as the person: written with a capital, three
letters or more, not one of CV Studio's everyday, month, place, lineage or
technology words, and not after a lineage word ("binti Rahman" is the
father). So "May 2023", "Petaling Jaya", "a grace period" and "bin Abdul" no
longer count as the candidate (before, such a blind CV was refused, or the
words were replaced). The word lists are `redact/name_words.json`, kept equal
to the engine's by `ai/tests/test_name_words.py`.

**Long CVs.** Reading a whole CV into JSON and writing it again anonymised
takes minutes for a long CV, so CV Studio's two AI calls may each take
`HP_AI_LONG_TIMEOUT_SECONDS` (300; CV Studio allows 180 for DeepSeek and
ChatGPT), and before each long step the worker extends its claim on the job
(`job_extend`), so no second worker takes it and pays for the same calls.

Go saves it as the candidate's `formatted` CV (a PDF follows) and counts the
tokens in the company's monthly allowance. Every failure comes back as a
reason, never an exception. LibreOffice cannot open a document that ends on a
framed paragraph (CV Studio pins its closing terms that way), so the PDF step
retries with an empty paragraph after it; the Word file is kept as made.

### Blind job ads, CV Studio's way (built: `jd.blind`)

CV Studio's JD blinding is browser JavaScript; its code is vendored unchanged
in `ai/cvstudio/jd` (its local checks, `jd_guardrails.cjs`, and the exact
prompt assembly from `generateAnonJD`, `prompts/JD_PROMPT_BUILDER.js.txt`)
and run by Node through a small adapter, `jd_cli.cjs`, as its Word renderer
is. `src/hiringplug_ai/cvstudio_jd.py` calls it; `jobs/blind_jd.py` follows
CV Studio's order: repair PDF damage in the text (at least 80 characters
needed), build its prompt from the ad (title and text), the client's name,
the blind location, work mode, client description and what else to remove
(the client's website and hiring contact are added), send it alone (no
system prompt, 4,000 tokens, as CV Studio does), read the JSON, ask once more
when responsibilities or requirements copy long runs of the ad (the second
answer kept only when it copies less), then CV Studio's checks: tech terms
from the ad put back in the stack, company scale softened, the client's name
scrubbed. An answer that is not JSON fails (CV Studio would show plain text;
a blind ad needs its structure). `test_the_prompt_is_cv_studios_own` checks
the prompt against CV Studio's own example, byte for byte, and its offline
checks (`check_jd.cjs`) run in the tests. Go (`jd.blinded`) makes the result
the ad's blind version and refuses one that still names the client.

## AI candidate search (built)

`talent.rank` carries what the job needs (the JD without salary, or the
recruiter's boolean and filters when there is none) and up to 20 candidates
by reference (C1, C2...), each with a profile line and their CV's text cut to
3,500 characters, without names, contact details or salary.
`jobs/talent_rank.py` asks, as CV Studio's AI Crawler and Job Fit do, to read
the responsibilities, requirements, tech stack and nice-to-haves (not the
company, benefits or culture) and to rate every candidate 0-100 with at most
4 reasons and 4 gaps, plus sourcing notes, must-haves, nice-to-haves and a
boolean for the job; "strict" caps a candidate missing a must-have at 50. It
reads the answer into the contract's shape (unknown references skipped,
fits held to 0-100, a missing verdict taken from the fit). Go checks every
reference against who it sent before saving (`talent.ranked`).

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
`HP_AI_TIMEOUT_SECONDS` (`HP_AI_LONG_TIMEOUT_SECONDS` for CV Studio's whole-CV
answers). `aiprovider.check` sends a provider a tiny request
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

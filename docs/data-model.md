# Data model

IDs are a type prefix plus a time-sortable code (`cand_01jbz6w0...`), so an ID
in a log says what it is. Every table with company data has `tenant_id` and
row-level security. Times are UTC (`timestamptz`).

## Built (milestone 1)

```
tenant ─┬─ app_user (role)
        ├─ candidate ─┬─ candidate_compensation   (salary; own security policy)
        │             └─ document                 (CV files and extracted text)
        ├─ audit_log
        └─ job                                    (the job queue)
```

| Table | Key fields |
|---|---|
| `tenant` | name, careers page address (`careers_slug`: unique, made from the name when the company is created) |
| `app_user` | email, name, role, active |
| `candidate` | names, email (unique per company), phone (E.164), nationality, work authorization + basis + note, notice period (value, unit), industry, specialization, current title/company, location, source, tags and skills (text arrays, indexed for case-insensitive filters), `version`, search text |
| `company` | name (unique per company, ignoring case and accents), type (client/prospect/internal), website (https), industry, size, phone, address, city, country, registration number, about, `version`, search text |
| `contact` | company (same tenant, foreign key), names, job title, email (unique per company) with check status + reasons + date, office phone, mobile, LinkedIn, status (active/left), notes, `version`, search text |
| `job_order` | company and optional hiring contact (same tenant, foreign keys), title, status (open/on_hold/filled/closed), location, work type, arrangement, openings, description, `version`, search text. (Not `job`: that is the background job queue.) |
| `job_ad` | job, title, location, work type/arrangement, salary text (published as written), text, closes on, status (draft/published/closed; published needs text), published at, `version` |
| `job_order_salary` | the salary budget (min, optional max, currency, period, `version`); row-level security needs `jobs.salary:read` to see and `jobs.salary:update` to write; clearing deletes the row |
| `application` | job and candidate (same tenant, foreign keys; once per job), stage (applied/screening/submitted/interview/offer/placed/rejected/withdrawn), reason + note exactly when rejected or withdrawn, when it entered its stage, channel (where it came from: careers page, JobStreet, LinkedIn, Indeed, ...; empty when a recruiter added it), `version` |
| `interview` | application, title, starts/ends at, length, mode (in person/video/phone), location or link, instructions, interviewers (user ids), status (scheduled/completed/cancelled/no show), the start time interviewers were last reminded of, `version` |
| `interview_feedback` | interview, interviewer, recommendation (strong yes/yes/no/strong no), comments, when; one per interviewer |
| `email_template` | name (unique per company, any case), subject, body with placeholders (`{{candidate.firstName}}`, ...; never salary), `version`; four starters added the first time a company opens its templates |
| `email` | candidate (moves with a merge; deleted with them), application id (kept as written), to, cc, subject (no line breaks), body, sender (user id, name and reply-to address), status (queued/sent/failed), error, sent at |
| `application_stage_change` | application, from stage, to stage, when; written by a database trigger on every stage change (history before it rebuilt from the audit log) |
| `offer` | application (one open offer each), status (draft/pending approval/approved/sent/accepted/declined/withdrawn), start date, answer-by date, other terms, who asked for approval, who approved (or sent back) and when with a note, sent and answered times, `version` |
| `offer_salary` | offer, amount/currency/period; row-level security: only with `offers.salary:read` (write: `:update`) |
| `api_key` | name, role (never owner), SHA-256 of the key, its first characters, created by, last used (to the minute), revoked at; looked up before any company is known by `app_api_key` |
| `webhook` | address, events, signing secret, on/off, description, `version` |
| `webhook_delivery` | webhook, event, payload (as sent), status (pending/delivered/failed), attempts, last answer or error, next attempt; written by a trigger on `audit_log` |
| `lead_source` | kind (greenhouse/lever/ashby/smartrecruiters/careers_page), board name or page address (once each), company name and record, on/off, open openings, last check and its error, next check, `version` |
| `job_lead` | source (kept when it is removed) and the opening's id there, company name and record, title, location, address, posted, first and last seen, closed, status (new/contacted/client/ignored), notes, `version` |
| `placement` | application (one each; moved to placed when recorded), recruiter credited, start date, guarantee days, status (active/fell through/cancelled) with the day they left (only a fall-through, within the guarantee), note, replacement (none, or owed/provided/refunded after a fall-through), invoice status (not invoiced/invoiced/paid) with number and dates, `version` |
| `placement_money` | placement, agreed salary (amount/currency/period), fee percent, fee amount and currency; row-level security: only with `placements.fees:read` (write: `:update`) |
| `note` | subject (candidate/job/company/contact + id, checked by the service through its module), kind (general/screening/interview/call), body, sensitive (row-level security: only with `notes.sensitive:read`), author id + name |
| `candidate_work` | candidate, position (0 = newest), title, company, start/end (YYYY-MM or YYYY), current, description; a title or company required |
| `candidate_education` | candidate, position, qualification, institution, year finished |
| `candidate_compensation` | current amount/currency/period, expected min/max/currency/period, `version` |
| `document` | candidate, source (jobstreet/linkedin/original/formatted/other), file name, type, size, SHA-256, storage key, status (queued/extracted/needs_ocr/failed), extracted text, page count, extractor, warnings, error; parse status (queued/parsed/failed), parsed profile (JSON, only values whose quote is in the text), parser |
| `audit_log` | actor, action (`candidate.updated`), entity, changes (from → to; sensitive fields redacted), request ID |
| `job` | queue (core/ai), kind, payload, status (pending/running/done/dead), attempts, lease, last error, request ID |

The database repeats the important rules as constraints: the work-authorization
basis only with `expat_eligible`, a complete notice period, complete salary
figures, an expected range that does not run backwards, and the document's
candidate belonging to the same company.

## Planned

| Module | Records | Milestone |
|---|---|---|
| `candidates` | managed taxonomy lists (industries, specializations), saved searches, consent and retention | M2 |
| `notes` | note templates per company, notes on an application, editing | Phase 2 |
| `identity` | custom roles, team, data scope, invites | M2 |
| `companies` | enrichment suggestions with source (auto-fill) | Phase 5 |
| `jobs` | job ad (advertised text, channels), owner recruiter | Phase 3 |
| `pipeline` | stage templates per company | M3 |
| `interviews` | interview (application, time, interviewers, location/link), feedback | M3 |
| `documents` | formatted CV (template, options), templates per company | M4 |
| `assistant` | conversation, message, AI usage (tokens, cost) | M4 |
| `matching` | match (candidate × job, score, reasons with quotes, model version) | M4 |
| `communications` | inbox sync (replies on the timeline), threads, sequences | 4.3 |
| `leads` | licensed job-data providers; contacts found for a lead's company | later |
| `integrations` | API key, webhook subscription and delivery | M6 |

Each planned module gets its own contract file, migration, queries and code via
`task new:module`, following [the playbook](playbooks/add-a-module.md).

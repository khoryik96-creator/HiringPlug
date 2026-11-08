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
| `app_user` | email, name, role, team, active |
| `field_option` | a company's dropdown options: list (candidate_source, notice_period, industry, work_type, work_arrangement), key, label, hidden, position; `field_list_saved` marks a list as saved (no rows = the built-in options) |
| `field_rule` | a company's choice for a form field (candidate/job + field): mandatory or not |
| `candidate` | names, email (unique per company), phone (E.164), nationality, country of residence, LinkedIn link (https), status (looking/passive/not_looking/placed) and when it was set, work authorization + basis + note, notice period (value, unit), industry, specialization, current title/company, location, source (a key from the company's source list), tags and skills (text arrays, indexed for case-insensitive filters), owner (a user, or none), `version`, search text |
| `company` | name (unique per company, ignoring case and accents), type (client/prospect/internal), website (https), industry, size, phone, address, city, country, registration number, about, `version`, search text; how the last website reading went (`autofill_status` idle/queued/done/failed, error, when) |
| `company_suggestion` | a detail read from the company's website: company, field, value, source (what and which page), status (pending/accepted/dismissed; who decided is in the audit log) |
| `contact` | company (same tenant, foreign key), names, job title, email (unique per company) with check status + reasons + date, office phone, mobile, LinkedIn, status (active/left), notes, `version`, search text |
| `job_order` | company and optional hiring contact (same tenant, foreign keys), title, status (open/on_hold/filled/closed), location, country, work type, arrangement, openings, description, intake date, owner (a user, or none), `version`, search text. (Not `job`: that is the background job queue.) |
| `job_ad` | job, title, location, country, work type/arrangement, salary text (published as written), text, closes on, status (draft/published/closed; published needs text), published at, client description, publish blind, a person's own blind title and text (with a hash of the ad they were written for; otherwise the blind version is made when read), `version` |
| `job_ad_blind_rewrite` | ad, status (queued/done/failed), tone, rewording strength, why it failed, what to check (warnings), whether the AI was asked twice, model, tokens (counted in the monthly AI allowance); a done one became the ad's blind title and text |
| `job_order_salary` | the salary budget (min, optional max, currency, period, `version`); row-level security needs `jobs.salary:read` to see and `jobs.salary:update` to write; clearing deletes the row |
| `application` | job and candidate (same tenant, foreign keys; once per job), stage (applied/screening/submitted/interview/offer/placed/rejected/withdrawn), reason + note exactly when rejected or withdrawn, when it entered its stage, channel (where it came from: careers page, JobStreet, LinkedIn, Indeed, ...; empty when a recruiter added it), `version` |
| `interview` | application, title, starts/ends at, length, mode (in person/video/phone), location or link, instructions, interviewers (user ids), status (scheduled/completed/cancelled/no show), the start time interviewers were last reminded of, `version` |
| `interview_feedback` | interview, interviewer, recommendation (strong yes/yes/no/strong no), comments, when; one per interviewer |
| `email_template` | name (unique per company, any case), subject, body (plain text) and formatted body (HTML with its signature and banner) with fields (`[Expected salary]`, `{{candidate.firstName}}`, ...), `version`; four starters added the first time a company opens its templates |
| `email_banner` | a company's banner image: name, type (PNG/JPEG/GIF), size, stored file; deleted ones kept for emails already sent |
| `email_attachment` | a file sent with an email (an upload, or a copy of the candidate's CV): name, type, size, stored file; uploads not sent within a day are removed |
| `tenant_policy` | a company's choices on sensitive information: who sees emails that show salary |
| `email` | candidate (moves with a merge; deleted with them), application id (kept as written), to, cc, bcc, subject (no line breaks), body, formatted body, shows salary (and hidden from roles that may not see salaries, when the company says so: row-level security), sender (user id, name and reply-to address), status (queued/sent/failed), error, sent at |
| `application_stage_change` | application, from stage, to stage, when, who (the user, none when the app moved it); written by a database trigger on every stage change (history before it rebuilt from the audit log) |
| `offer` | application (one open offer each), status (draft/pending approval/approved/sent/accepted/declined/withdrawn), start date, answer-by date, other terms, who asked for approval, who approved (or sent back) and when with a note, sent and answered times, `version` |
| `offer_salary` | offer, amount/currency/period; row-level security: only with `offers.salary:read` (write: `:update`) |
| `api_key` | name, role (never owner), SHA-256 of the key, its first characters, created by, last used (to the minute), revoked at; looked up before any company is known by `app_api_key` |
| `webhook` | address, events, signing secret, on/off, description, `version` |
| `webhook_delivery` | webhook, event, payload (as sent), status (pending/delivered/failed), attempts, last answer or error, next attempt; written by a trigger on `audit_log` |
| `lead_source` | kind (greenhouse/lever/ashby/smartrecruiters/careers_page), board name or page address (once each), company name and record, on/off, open openings, last check and its error, next check, `version` |
| `job_lead` | source (kept when it is removed) and the opening's id there, company name and record, title, location, address, posted, first and last seen, closed, status (new/contacted/client/ignored), notes, `version` |
| `team_group` | a team of users (e.g. a desk): name (unique per company, ignoring case), `version` |
| `role_setting` | a company's own settings for a role (none: the defaults): permissions, candidate scope (all/team/own), also candidates on owned jobs, `version`; never for the owner |
| `user_preference` | one per user: their own settings, such as the candidate list's columns in order (none: the default); only that user reads or changes it (row-level security on user too) |
| `app_user` (sign-in) | password hash (Argon2id), invited (not joined yet), sessions ended at, last signed in |
| `user_link` | user, kind (invite/reset), SHA-256 of the link token, expires, used; looked up before any company is known by `app_user_link`, accounts by `app_accounts_by_email` |
| `assistant_answer` | a question to the AI assistant: who asked, about what (candidate/job/application + id), kind, question, status (queued/done/failed), answer or error, model, input and output tokens, asked and answered; shown only to the asker |
| `candidate_consent` | one per candidate: given or withdrawn (withdrawn = do not contact), how (careers form, email, phone, in person, document, on request), the day, note, recorded by, `version` |
| `privacy_request` | a candidate's request about their data: kind (access, correction, withdraw consent, removal, other), received on, details, status (open/done/declined), outcome (required to close), closed by and when, `version`; never deletes data |
| `job_match` | a fit score of an application: application, job, status (queued/done/failed), score 0-100, summary, CV Studio's six ratings (JSON: name, score, note), strengths and gaps (at most 5 each), reasons (requirement quoted from the job, verdict, evidence quoted from the CV, note), requirements dropped, error, model, input and output tokens, asked by and when, scored, a hash of the job text (to tell when it is outdated); what the AI was sent is kept only while queued |
| `cv_search` | the words of each candidate's CV (not formatted ones), as a full-text index, for possible candidates; kept in step with `document` by a trigger |
| `cv_template` | the company's Word CV templates: name, stored file, uploaded by (the built-in standard one is not stored) |
| `document` (PDF) | for formatted CVs made by the app: PDF status (queued/ready/failed), stored PDF, error |
| `placement` | application (one each; moved to placed when recorded), recruiter credited, start date, guarantee days, status (active/fell through/cancelled) with the day they left (only a fall-through, within the guarantee), note, replacement (none, or owed/provided/refunded after a fall-through), invoice status (not invoiced/invoiced/paid) with number and dates, `version` |
| `placement_money` | placement, agreed salary (amount/currency/period), fee percent, fee amount and currency; row-level security: only with `placements.fees:read` (write: `:update`) |
| `note` | subject (candidate/job/company/contact + id, checked by the service through its module), kind (general/screening/interview/call), body, sensitive (row-level security: only with `notes.sensitive:read`), author id + name |
| `cv_import` | an import of CVs or LinkedIn PDFs: source given to the new candidates, who started it (only they see it) |
| `cv_import_item` | one file of an import: file name, status (reading/created/added/unread/failed), candidate, document, error |
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
| `candidates` | managed taxonomy lists (industries, specializations), saved searches | M2 |
| `notes` | note templates per company, notes on an application, editing | Phase 2 |
| `identity` | new named roles, single sign-on | 7 |
| `companies` | enrichment suggestions with source (auto-fill) | Phase 5 |
| `jobs` | job ad (advertised text, channels), owner recruiter | Phase 3 |
| `pipeline` | stage templates per company | M3 |
| `interviews` | interview (application, time, interviewers, location/link), feedback | M3 |
| `assistant` | conversation, message, AI usage (tokens, cost) | M4 |
| `matching` | possible candidates by meaning (embeddings) | later |
| `communications` | inbox sync (replies on the timeline), threads, sequences | 4.3 |
| `leads` | licensed job-data providers; contacts found for a lead's company | later |
| `integrations` | API key, webhook subscription and delivery | M6 |

Each planned module gets its own contract file, migration, queries and code via
`task new:module`, following [the playbook](playbooks/add-a-module.md).

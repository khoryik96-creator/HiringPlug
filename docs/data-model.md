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
| `candidate` | names, email (unique per company), phone (E.164), nationality, country of residence, LinkedIn link (https), status (looking/passive/not_looking/placed) and when it was set, work authorization + basis + note, notice period (value, unit), industry, specialization, current title/company, location, source (a key from the company's source list), tags, skills and certifications (text arrays; tags and skills indexed for case-insensitive filters), owner (a user, or none), `version`, search text, profile_terms (a text search of title, company, skills, tags, industry, specialization and location, for advanced search) |
| `company` | name (unique per company, ignoring case and accents), type (client/prospect/internal), website (https), industry, size, phone, address, city, country, registration number, about, `version`, search text; how the last website reading went (`autofill_status` idle/queued/done/failed, error, when) |
| `company_suggestion` | a detail read from the company's website: company, field, value, source (what and which page), status (pending/accepted/dismissed; who decided is in the audit log) |
| `contact` | company (same tenant, foreign key), names, job title, email (unique per company) with check status + reasons + date, office phone, mobile, LinkedIn, status (active/left), role (a key of the Settings contact_role list, or none), notes, `version`, search text |
| `job_order` | company and optional hiring contact (same tenant, foreign keys), title, status (open/on_hold/filled/closed), location, country, work type, arrangement, openings, description, intake date, owner (a user, or none), `version`, search text. (Not `job`: that is the background job queue.) |
| `job_ad` | job, title, location, country, work type/arrangement, salary text (published as written), text, closes on, status (draft/published/closed; published needs text), published at, client description, publish blind, a person's own blind title and text (with a hash of the ad they were written for; otherwise the blind version is made when read), `version` |
| `job_ad_blind_rewrite` | ad, status (queued/done/failed), tone, rewording strength, why it failed, what to check (warnings), whether the AI was asked twice, model, tokens (counted in the monthly AI allowance), the ad's source hash when it was asked; a done one became the ad's blind title and text |
| `job_order_salary` | the salary budget (min, optional max, currency, period, `version`); row-level security needs `jobs.salary:read` to see and `jobs.salary:update` to write; clearing deletes the row |
| `application` | job and candidate (same tenant, foreign keys; once per job), stage (applied/screening/submitted/interview/offer/placed/rejected/withdrawn), reason + note exactly when rejected or withdrawn, when it entered its stage, channel (where it came from: careers page, JobStreet, LinkedIn, Indeed, ...; empty when a recruiter added it), `version` |
| `interview` | application, title, starts/ends at, length, mode (in person/video/phone), location or link, instructions, interviewers (user ids), status (scheduled/completed/cancelled/no show), the start time interviewers were last reminded of, `version` |
| `interview_feedback` | interview, interviewer, recommendation (strong yes/yes/no/strong no), comments, when; one per interviewer |
| `email_template` | name (unique per company, any case), subject, body (plain text) and formatted body (HTML with its signature and banner) with fields (`[Expected salary]`, `{{candidate.firstName}}`, ...), `version`; four starters added the first time a company opens its templates |
| `email_banner` | a company's banner image: name, type (PNG/JPEG/GIF), size, stored file; deleted ones kept for emails already sent |
| `email_attachment` | a file sent with an email (an upload, or a copy of the candidate's CV): name, type, size, stored file; uploads not sent within a day are removed |
| `tenant_policy` | a company's choices on sensitive information: who sees emails that show salary |
| `email` | candidate (moves with a merge; deleted with them), application id (kept as written), to, cc, bcc, subject (no line breaks), body, formatted body, shows salary (and hidden from roles that may not see salaries, when the company says so: row-level security), to a candidate (an email about several candidates written to one of them: it may show only their salary), sender (user id, name and reply-to address), status (queued/sent/failed), error, sent at |
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
| `tenant` (Microsoft sign-in) | signs in with Microsoft, its Microsoft organisation (Entra tenant ID; never the personal-accounts one), requires Microsoft (passwords only for owners) |
| `user_microsoft` | the Microsoft account (organisation + object ID, unique per company) a user signed in with first, linked and last used |
| `microsoft_sign_in_flow` | a Microsoft sign-in under way (at most 15 minutes): SHA-256 of its state, of the starting tab's value and of its one-time ticket, the company (none: any, through the server's app), app and authority, the sealed PKCE verifier and nonce (or the checked account awaiting a company choice), the sealed code; no policy lets the app touch it: only the `app_microsoft_flow_*` functions, and `app_microsoft_*` find companies and people before any company is known |
| `assistant_answer` | a question to the AI assistant: who asked, about what (candidate/job/application + id), kind, question, status (queued/done/failed), answer or error, model, input and output tokens, asked and answered; shown only to the asker |
| `candidate_consent` | one per candidate: given or withdrawn (withdrawn = do not contact), how (careers form, email, phone, in person, document, on request), the day, note, recorded by, `version` |
| `privacy_request` | a candidate's request about their data: kind (access, correction, withdraw consent, removal, other), received on, details, status (open/done/declined), outcome (required to close), closed by and when, `version`; never deletes data |
| `job_match` | a fit score of an application: application, job, status (queued/done/failed), score 0-100, summary, CV Studio's six ratings (JSON: name, score, note), strengths and gaps (at most 5 each), reasons (requirement quoted from the job, verdict, evidence quoted from the CV, note), requirements dropped, error, model, input and output tokens, asked by and when, scored, a hash of the job text (to tell when it is outdated); what the AI was sent is kept only while queued |
| `talent_search` | an AI candidate search (CV Studio's AI Crawler): who asked (shown only to them), status (queued/done/failed), job description, boolean, filters (the advanced search), strict must-haves, how many the search found, the candidates sent to the AI in order (C1, C2...; never their names), its ranking (candidate, fit 0-100, verdict, reasons, gaps), sourcing notes, must-haves, nice-to-haves, suggested boolean, error, model, tokens (counted in the monthly AI allowance) |
| `cv_search` | the words of each candidate's CV (not formatted ones), as a full-text index, for possible candidates; kept in step with `document` by a trigger |
| `cv_template` | the company's Word CV templates: name, stored file, uploaded by (the built-in standard one is not stored) |
| `document` (PDF) | for formatted CVs made by the app: PDF status (queued/ready/failed), stored PDF, error |
| `placement` | application (one each; moved to placed when recorded), recruiter credited, start date, guarantee days, status (active/fell through/cancelled) with the day they left (only a fall-through, within the guarantee), note, replacement (none, or owed/provided/refunded after a fall-through), invoice status (not invoiced/invoiced/paid) with number and dates, `version` |
| `placement_money` | placement, agreed salary (amount/currency/period), fee percent, fee amount and currency; row-level security: only with `placements.fees:read` (write: `:update`) |
| `job_retainer`, `job_retainer_stage` | a job's retainer (currency, signed on) and its stages (label, amount, due on a date or days after signing or the start, invoice status, number and dates); row-level security like `placement_money` |
| `automation` | a When · If · Then rule: name, on/off, trigger (and stage), days after or before, client (optional), 1-3 actions (email the candidate, client or recruiter: subject and message with fields; or a task), send on its own or ask first, the ready-made one it started as, `version` |
| `automation_run` | one per automation and record (application, interview or placement): when it is due, status (scheduled, to approve, done, skipped, failed), what happened, who approved; queued by a trigger on `audit_log` (`automation.event`) |
| `recruiter_task` | a task for a user from an automation: title, due day, application, done when |
| `automation_settings` | the company's default for new automations (send or ask first), and that the ready-made ones were added |
| `note` | subject (candidate/job/company/contact + id, checked by the service through its module), kind (general/screening/interview/call), body, sensitive (row-level security: only with `notes.sensitive:read`), author id + name |
| `folder` | a folder of candidates, contacts or companies (kind): name (1-100 characters), owner (who made it), `version` |
| `folder_share` | a folder and someone of the agency it is shared with (they see it and add or remove records; only the owner renames, deletes or shares it) |
| `folder_item` | a folder and a record in it (candidate, contact or company id, checked by its kind), who put it there and when; a candidate merge moves the duplicate's places to the one kept; row-level security: a candidate's row only for those whose scope sees them |
| `cv_import` | an import of CVs or LinkedIn PDFs: source given to the new candidates, who started it (only they see it) |
| `cv_import_item` | one file of an import: file name, status (reading/created/added/unread/failed), candidate, document, error |
| `candidate_work` | candidate, position (0 = newest), title, company, start/end (YYYY-MM or YYYY), current, description; a title or company required |
| `candidate_education` | candidate, position, qualification, institution, year finished |
| `candidate_compensation` | current amount/currency/period, expected min/max/currency/period, `version` |
| `document` | candidate, source (jobstreet/linkedin/original/formatted/other), file name, type, size, SHA-256, storage key, status (queued/extracted/needs_ocr/failed), extracted text, page count, extractor, warnings, error; parse status (queued/parsed/failed), parsed profile (JSON, only values whose quote is in the text), parser |
| `claim` | an expense claim, one per receipt: claimant, status (draft/submitted/approved/rejected/paid), the receipt (file name, type, size, SHA-256: the same receipt once per person, storage key), reading it (reading/done/failed, error, reader), receipt date, amount (numeric) and currency, merchant, receipt type and claim type (Settings options), purpose, the client or candidate it was for (company, contact or candidate id), the fields read from the receipt and not changed since, claimed, decided (by, note) and paid dates, `version`; row-level security: the claimant's own, everyone's with `claims:approve`; its upload to the claims storage (none/pending/done/failed, path, link, error, provider, account and the file's id there, a sequence so an older upload job does nothing) |
| `claim_storage` | the company's claims storage: provider (google_drive/onedrive/dropbox), account, top folder, its tokens sealed (AES-GCM), the app it was connected through, status (disconnected/ok/reconnect), error, `version`; row-level security: `claims.storage:manage` |
| `claim_storage_state` | a sign-in in progress, one per person: the PKCE verifier and then the provider's code (both sealed), a one-time ticket; row-level security: the person who started it |
| `email_signature` | each person's email signature (formatted, with banners by id), `version`; row-level security: its owner only |
| `audit_log` | actor (a user, an API key or `system`), action (`candidate.updated`), entity, changes (from → to; sensitive fields redacted), request ID; indexed by company and time, and by company, actor and time, for the audit log page |
| `salary_view` | who was shown a salary (auditlog module): user (a person or an API key; never `system`), kind (candidate/candidate_list/export/email_fields/email_sent/email_read/cv/screening/offer/placement), candidate (none for a list or export, whose candidates are kept in `candidate_ids`), day (the agency's), count, first and last time; one row per user, kind, candidate and day; never an amount; written only by `app_record_salary_view`; the app never changes or deletes one itself, except that a candidate merge (`app_move_salary_views`, needs `candidates:merge`) moves the duplicate's lines to the kept candidate: a line for the same person, kind and day takes the duplicate's count and the duplicate's line is deleted, others are re-pointed, and list and export lines name the kept candidate instead; read with `auditlog.salary_views:read` within the candidate scope |
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
| `pipeline` | stage templates per company (`stage_template`), each job's own stages (`job_stage_plan`), the job's stage an application is in (`application.step`) | ✅ |
| `interviews` | interview (application, time, interviewers, location/link), feedback | M3 |
| `assistant` | conversation, message, AI usage (tokens, cost) | M4 |
| `meaning` | how each candidate and search was read by meaning (`meaning_candidate`, `meaning_role`, `meaning_passage`: vectors of 384 numbers in pgvector, HNSW indexes; `meaning_query`, `meaning_query_need`: a search's or job's reading, kept 90 days after its last use), and the background read-through (`meaning_sweep`, `meaning_pending`); row-level security: the candidate tables are seen only while the candidate is (in scope, not in the Bin); not in backups (read again after a restore) | ✅ 1.40.0, 1.40.3 |
| `interviewprep` | questions asked at a job's interviews (`interview_question`), the AI's interview prep (`interview_prep`) | ✅ |
| `notifications` | per user (`notification_state`): when they last opened the bell, the kinds they turned off, the morning email on/off, its last day and when its next job is due; the notifications themselves are worked out from other modules' records; row-level security: each user's own row | ✅ |
| `shortlists` | client shortlist links (`client_shortlist`: the job, a hash of the link's secret, message, expiry, revoked, first and last visit, who it was emailed to) and who is on each (`client_shortlist_candidate`: application, candidate, the CV shown, blind, the label the client sees, the recruiter's note, the client's answer and comment); `cvformats` keeps how each formatted CV was made (`formatted_cv`: blind, with salary); each row goes with the candidate, application, CV or job it points to when that is deleted for good; row-level security, candidate scope | ✅ |
| `communications` | inbox sync (replies on the timeline), threads, sequences | 4.3 |
| `leads` | licensed job-data providers; contacts found for a lead's company | later |
| `integrations` | API key, webhook subscription and delivery | M6 |

Each planned module gets its own contract file, migration, queries and code via
`task new:module`, following [the playbook](playbooks/add-a-module.md).

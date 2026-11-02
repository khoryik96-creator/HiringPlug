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
| `tenant` | name |
| `app_user` | email, name, role, active |
| `candidate` | names, email (unique per company), phone (E.164), nationality, work authorization + basis + note, notice period (value, unit), industry, specialization, current title/company, location, source, tags and skills (text arrays, indexed for case-insensitive filters), `version`, search text |
| `company` | name (unique per company, ignoring case and accents), type (client/prospect/internal), website (https), industry, size, phone, address, city, country, registration number, about, `version`, search text |
| `contact` | company (same tenant, foreign key), names, job title, email (unique per company) with check status + reasons + date, office phone, mobile, LinkedIn, status (active/left), notes, `version`, search text |
| `job_order` | company and optional hiring contact (same tenant, foreign keys), title, status (open/on_hold/filled/closed), location, work type, arrangement, openings, description, `version`, search text. (Not `job`: that is the background job queue.) |
| `job_ad` | job, title, location, work type/arrangement, salary text (published as written), text, closes on, status (draft/published/closed; published needs text), published at, `version` |
| `job_order_salary` | the salary budget (min, optional max, currency, period, `version`); row-level security needs `jobs.salary:read` to see and `jobs.salary:update` to write; clearing deletes the row |
| `application` | job and candidate (same tenant, foreign keys; once per job), stage (applied/screening/submitted/interview/offer/placed/rejected/withdrawn), reason + note exactly when rejected or withdrawn, when it entered its stage, `version` |
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
| `pipeline` | application (candidate × job, stage, status, rejection reason, `version`), stage templates, stage history | M3 |
| `interviews` | interview (application, time, interviewers, location/link), feedback | M3 |
| `documents` | formatted CV (template, options), templates per company | M4 |
| `assistant` | conversation, message, AI usage (tokens, cost) | M4 |
| `matching` | match (candidate × job, score, reasons with quotes, model version) | M4 |
| `communications` | email message, thread, template, sequence | M5 |
| `placements` | placement (agency: fee, start date, guarantee period); offer (in-house) | M5 |
| `job_leads` | job lead (source, URL, company, parsed fields, status) | M6 |
| `integrations` | API key, webhook subscription and delivery | M6 |

Each planned module gets its own contract file, migration, queries and code via
`task new:module`, following [the playbook](playbooks/add-a-module.md).

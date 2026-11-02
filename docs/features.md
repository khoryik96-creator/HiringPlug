# Features: your list, mapped to the design

Context from your answers: **agency and in-house**, **your own team and other
companies (SaaS)**, **local first, cloud later**, and every version 1
must-have (OCR/CV parsing, careers page, email, job boards, AI matching, data
import).

Status: ✅ built in milestone 1 · 🧩 designed, scheduled (see [roadmap](roadmap.md)).

| # | Feature | Module(s) | Status |
|---|---|---|---|
| 1 | Contacts with built-in email verifier | `companies`, `contacts` | 🧩 M3 |
| 1.5 | Job ads puller | `job_leads` | 🧩 M6 |
| 2 | Company: auto-fill details | `companies` | 🧩 M3 |
| 3 | Candidate profile fields | `candidates` | ✅ |
| 3 | JobStreet / LinkedIn / original / formatted CVs | `documents` | ✅ upload + text; formatting 🧩 M4 |
| 3.5 | AI interpretation sidebar (DeepSeek) | `assistant` + AI worker | 🧩 M4 |
| 4 | Pipeline dashboard | `pipeline`, `reporting` | 🧩 M3 |
| 5 | Job ads: applicants, interview scheduling, CV vs job match | `jobs`, `pipeline`, `interviews`, `matching` | 🧩 M3–M5 |
| 5 | Notes: screening, CV and interview notes | `notes` | 🧩 M2 |
| 6 | API documentation for integration | contract → `/docs` | ✅ |
| 7 | User access: view / modify / write, sensitive (salary) | `identity` + platform | ✅ roles; custom roles 🧩 M2 |

## 3. Candidates

Profile fields (all in the contract, `contracts/modules/candidates.yaml`):

| Field | Notes |
|---|---|
| First and last name | Spaces tidied; full name derived |
| Email | Lower-cased; unique per company (duplicates are refused and point to the existing candidate) |
| Phone | Any common format in; stored internationally (`012-345 6789` → `+60123456789`, default country configurable) |
| Nationality | ISO country code (`MY`, `SG`, ...) |
| Work authorization | `local`; `expat_eligible` with basis `rp_t` (Residence Pass-Talent), `permanent_resident`, `spouse_visa` or `other`; `requires_visa`. The basis is required exactly when the status is `expat_eligible` (checked in Go and again by the database). |
| Notice period | Value and unit (days, weeks, months); 0 means immediate |
| Industry, specialization | Free text now; a shared list (taxonomy) in M2 so search and reports are consistent |
| Current title and company, location, source | Source: JobStreet, LinkedIn, referral, careers page, direct, other |
| **Current salary**, **expected salary** | Amount, currency (`MYR`, `SGD`, ...) and period (hour/day/month/year) per figure; expected can be a range. **Sensitive:** see [access control](access-control.md). |

The web app edits every field above. Salary has its own form, shown only to
roles with `candidates.compensation:update`; viewers get no edit buttons. Edits
are protected against lost updates (`version`): a form saves with the version
it was opened with, so if someone else saved first (even if the page has
refreshed since), the form says so and offers to reload. Validation messages
appear next to the field (e.g. "Pass held" is required for an expat who can
work here); any the form cannot place are listed under it, so a failed save is
never silent. Saving without changes writes nothing. Amounts accept thousands
separators (`8,500`, `1,00,000`); a decimal comma (`8500,50`) is refused rather
than guessed. Every change is in the audit log, and search ignores case and
accents (`jose` finds `José`).

**CV versions.** A candidate has documents, each with a `source`: `jobstreet`,
`linkedin`, `original`, `formatted` or `other`. Each upload keeps its file, its
extracted text and any extraction warnings, so the web app can show the
JobStreet, original and formatted versions side by side.

**CV formatting (M4).** A formatted CV is generated from the candidate's
structured profile (from CV parsing, M2) into your branded Word template,
then saved as a `formatted` document. Templates are Word files with
placeholders (docxtpl), so each agency edits its own template in Word without
code changes. Options: hide contact details, hide salary, blind CV. PDFs are
made with LibreOffice. Design: [ai-worker.md](ai-worker.md#cv-formatting-and-blind-cvs-m4).

## 3.5 AI interpretation sidebar (DeepSeek)

A panel on candidate and job pages: a summary, gaps and red flags, "how does
this candidate fit this job?", and free questions.

- **Permission-aware context.** Go builds what the AI may see from the
  signed-in user's permissions. A user who cannot see salary gets an assistant
  that cannot see it either, so the sidebar can never leak a restricted field.
- **Privacy.** DeepSeek's own API stores data in China, which matters under
  Malaysia's PDPA. The provider is a setting per company. Options: DeepSeek's
  open model through a host in a region you choose, or on your own server; or
  another provider. Contact details are removed before anything is sent.
- **Costs.** Tokens and cost are recorded per company and user, with monthly
  limits (as CV Studio's AI cost guardrails do).
- **Flow.** Browser → Go API (permission check, context) → internal Python AI
  service → provider; the answer streams back. All AI code lives in Python
  behind one provider interface.

## 1, 2. Companies and contacts

- **Companies:** clients and prospects (agency) or your own company and its
  departments (in-house). **Auto-fill** from a company name or website: a job
  reads the website (structured data first, then AI extraction), optional data
  providers (Apollo, Google Places), and the business registry where available.
  Results are **suggestions with their source**; a person accepts them, and
  nothing overwrites data you entered.
- **Contacts:** people at companies (hiring managers, HR). The **email
  verifier** checks syntax, the domain's mail servers (MX), disposable and
  role addresses (`hr@`), and optionally the mailbox itself. Each result is
  stored with its evidence and date: valid, risky (catch-all, role, disposable),
  invalid or unknown. Mailbox checks need outbound port 25, which home ISPs and
  cloud providers often block, so a paid verification service can be plugged
  in instead. CV Studio's email verifier notes will be reused.

## 1.5 Job ads puller

Collects job ads to find companies that are hiring (business development
leads), then links them to companies (auto-fill) and contacts (verified emails).

**Legal note:** automatically scraping LinkedIn or JobStreet breaks their terms
of use, and LinkedIn takes scrapers to court. The puller uses sources that are
allowed:

1. **A recruiter saves an ad** they are viewing: paste a link, or a browser
   extension button ("save this ad").
2. **Company careers pages and public job feeds**: many ATSs (Greenhouse,
   Lever, Workable, SmartRecruiters) publish open roles as public feeds.
3. **Licensed job-data providers** and official partner programmes.

Each ad is parsed by the AI worker (title, company, location, salary,
requirements), de-duplicated, and kept with its source and date.

## 4, 5. Jobs, pipeline, interviews, matching

- **Jobs:** job orders for clients (agency) or requisitions (in-house), each
  with job ads (the advertised text).
- **Pipeline:** a candidate on a job is an *application* moving through stages
  (e.g. applied → screened → submitted to client → interview → offer →
  placed/hired, or rejected/withdrawn with a reason). Allowed moves are defined
  in one place (`pipeline/domain.go`) and tested move by move. The dashboard is
  a board per job plus counts, time in stage and conversion per recruiter.
- **Interviews:** scheduling with interviewers, Google and Microsoft calendar
  sync, reminders, and feedback forms.
- **CV vs job match:** the AI worker scores fit and explains it. Every reason
  quotes the CV and the job ad, so a recruiter can check it. It suggests; the
  recruiter decides.

## 5. Notes

Screening notes, CV notes and interview notes, attached to a candidate and
optionally a job or application. Templates hold screening questions. Notes can
be private or shared. A note that records salary is marked sensitive and
follows the salary permission.

## 6. API documentation

`/docs` renders the contract (endpoints, fields, permissions, error codes) and
`/openapi.json` serves it for tools. Integrations will get API keys per company
and webhooks (M6), driven by the same contract.

## 7. User access

Roles have permissions per area: **view** (`:read`), **write** (`:create`),
**modify** (`:update`). **Sensitive** details have their own permissions
(`candidates.compensation:read` / `:update`). Defaults per role are in the
contract. M2 adds custom roles per company, user invites, SSO, and a "who
viewed salary" log. Details: [access-control.md](access-control.md).

## Agency and in-house, own team and SaaS

- **Both modes on one core.** Candidates, jobs, pipeline, notes and documents
  are shared. Agency adds clients, client contacts, submissions and placements
  with fees; in-house adds hiring managers, requisition approval and offers. A
  company setting switches the screens.
- **Many companies from day one.** Every table is isolated per company in the
  database, so your own team is simply the first company.

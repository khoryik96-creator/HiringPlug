# Features: your list, mapped to the design

Context from your answers: **agency and in-house**, **your own team and other
companies (SaaS)**, **local first, cloud later**, and every version 1
must-have (OCR/CV parsing, careers page, email, job boards, AI matching, data
import).

Status: ✅ built in milestone 1 · 🧩 designed, scheduled (see [roadmap](roadmap.md)).

| # | Feature | Module(s) | Status |
|---|---|---|---|
| 1 | Contacts with built-in email verifier | `companies`, `contacts` | ✅ |
| 1.5 | Job ads puller | `job_leads` | 🧩 M6 |
| 2 | Companies | `companies` | ✅ records; auto-fill 🧩 |
| 3 | Candidate profile fields | `candidates` | ✅ |
| 3 | JobStreet / LinkedIn / original / formatted CVs | `documents` | ✅ upload + text; formatting 🧩 M4 |
| 3.5 | AI interpretation sidebar (DeepSeek) | `assistant` + AI worker | 🧩 M4 |
| 4 | Pipeline dashboard | `pipeline` | ✅ board and dashboard; reports 🧩 |
| 5 | Jobs and applicants ✅; interview scheduling, CV vs job match | `jobs`, `pipeline`, `interviews`, `matching` | ✅ jobs, pipeline, interviews; 🧩 matching |
| 5 | Email with templates | `emails` | ✅ sending; inbox sync 🧩 4.3 |
| 5 | Notes: screening, call and interview notes | `notes` | ✅ |
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
| Industry, specialization | Free text; the form suggests values already used in your company so everyone spells them the same way |
| Skills, tags | Short labels (up to 100 skills and 50 tags, 60 characters each); spaces tidied and the same label in another case kept once. The form suggests labels already in use; the candidate list filters by one skill and one tag (ignoring case) and shows tags next to names. |
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

**Work history and education** ✅: jobs (title, company, from/to as month and
year or year only, current) and qualifications (name, institution, year) on
the candidate's page, newest first, edited as a whole with the same lost-update
protection as the profile; filled from a CV in one tick. Merging keeps the
kept candidate's history, or takes the duplicate's when it has none.

**Possible duplicates** ✅: a candidate's page warns about others with the
same phone number or a very similar name ("Siti Aisyah" and "Siti Aishah",
at least 60% alike ignoring case and accents). The same email is refused when
adding. **Merge** a duplicate into the candidate you keep (owners, admins and
recruiters, who can see salaries, so nothing is lost unseen): its CVs, jobs
and notes move over, empty fields are filled from it (and its salary, if the
kept candidate has none), and it is deleted, in one transaction. If both were
on the same job, the kept candidate's application stays.

**CV versions** ✅. A candidate has documents, each with a `source`: `jobstreet`,
`linkedin`, `original`, `formatted` or `other`. Each upload keeps its file, its
extracted text and any extraction warnings; files can be downloaded under their
original name. **Compare CVs side by side** (on the candidate's CV list) shows
two versions next to each other, PDFs as they look and Word files as text, with
the differences read from them on top: fields that disagree, jobs whose title
or dates changed (matched by employer), jobs and skills only one version has.
That is the check before sending a formatted CV to a client.

**CV parsing** ✅: after a CV's text is read (with OCR for scans), the AI
worker reads the profile details out of it: name, email, phone, LinkedIn,
location, nationality, notice period, skills, work history and education.
Every value carries the exact CV text it came from, and the server keeps only
values whose quote is in the CV. On the candidate's page, **Review details
from this CV** lists each suggestion next to what the profile has now, with
the CV's wording: empty profile fields are ticked, filled ones never are, and
**Use ticked details** saves them (with the usual lost-update protection).
New skills are added to the existing ones. **Read details from this CV**
parses CVs uploaded earlier. Formatted CVs are parsed too, for comparing, but not offered for review. The CV's work
history and education are saved on the profile when ticked (ticked by default
only if the profile has none; existing history is replaced only when asked).
Design:
[ai-worker.md](ai-worker.md#cv-parsing-grounded-in-the-source).

![Reviewing details read from a CV (an invented CV)](images/cv-review.png)

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

- **Companies** ✅: clients and prospects (agency) or your own organisation's
  departments (in-house): name, type, website, industry, size, phone, address,
  city, country, registration number (e.g. SSM) and notes. Search by name or
  website (ignoring case and accents), filter by type. Names are unique per
  company (`Nestlé Malaysia` and `nestle malaysia` are the same), and edits use
  the same lost-update protection as candidates. **Auto-fill** (next) from a company name or website: a job
  reads the website (structured data first, then AI extraction), optional data
  providers (Apollo, Google Places), and the business registry where available.
  Results are **suggestions with their source**; a person accepts them, and
  nothing overwrites data you entered.
- **Contacts** ✅: people at companies (hiring managers, HR) with job title,
  email, office phone, mobile, LinkedIn, status (works there / has left) and
  notes; listed on their company's page. The **email check** runs whenever an
  email is added or changed: the format, whether the domain receives mail (its
  MX records, looked up with a 3-second limit outside any database
  transaction), and shared (`hr@`) or disposable addresses. The result is
  **valid**, **risky**, **invalid** or **unknown** (lookup failed: "Check
  again"), with the reasons in words. Next: optionally the mailbox itself. Each result is
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

- **Jobs** ✅: job orders for clients (agency) or requisitions (in-house):
  company, hiring contact (must work at that company), title, status (open,
  on hold, filled, closed), location, work type, on site / hybrid / remote,
  openings and description, listed on the company's page. The **salary
  budget** follows the salary pattern: its own permissions (hiring managers
  may see it, sourcers and viewers may not), its own table that the database
  refuses to others, and no amounts in the audit log. Job ads (the advertised
  text) come in Phase 3.
- **Job ads** ✅: the advertised text of a job, separate from the internal job
  order (a job can have several). Each has its own title, location, work type
  and arrangement, an optional salary line shown as written (separate from the
  job's private budget), a closing date and the ad's text, where lines starting
  with "- " become bullet points (plain text only: nothing typed into an ad can
  run in a reader's browser). A new ad starts from the job's details. Status:
  draft, published or closed. Publishing needs an open job, the ad's text and
  a closing date that has not passed, and says which is missing. A published
  ad stays up while its job is on hold; a job that is filled or closed takes
  its published ads down in the same change. The careers page (3.2) shows
  published ads.
- **Careers page** ✅: every company has a public page (`/#/careers/<address>`,
  linked from each job's ads panel) listing its published ads that have not
  passed their closing date, each with its text and an apply form: name,
  email, phone, CV (PDF or Word) and a consent tick box ("I agree that
  <company> keeps my details to handle my application"). Applying adds the
  candidate (source: careers page), or uses the existing candidate with that
  email without changing their profile; the CV is attached, read and parsed;
  and they join the job's pipeline at "applied", all in one transaction. The
  answer is the same whether the email was known, so the form reveals nothing.
  Robots are kept out by a hidden field people never fill in and a limit per
  connection (5 applications in 10 minutes, 20 a day).
- **Job boards** ✅ (without partner accounts): each published ad has a link
  per board (JobStreet, LinkedIn, Indeed, Facebook; "Links for job boards" on
  the ad). Post the ad on the board with its link; applicants who come
  through it are recorded on the application ("Applied via LinkedIn" on the
  pipeline card) and, for new candidates, as their source. The careers page
  has real addresses (`/careers/<company>/<ad>`, which search engines and
  boards can link to; the `#/` ones keep working), describes each ad for
  Google for Jobs (schema.org JobPosting), and publishes an XML job feed of
  its open ads (`/v1/public/careers/<company>/jobs.xml`, the source/job
  format Indeed and aggregators read). Posting through the boards' partner
  programmes comes once those accounts exist.
- **Interviews** ✅: on each of a candidate's jobs (candidate page), recruiters
  schedule interviews: title, date and time, length, in person / video /
  phone, the address or meeting link, the interviewers (any of the company's
  users) and instructions for the candidate. Scheduling moves the application
  to the interview stage if it is at an earlier one; closed or placed
  applications get no new interviews. An interviewer already booked at that
  time is a clash, shown with the other interview, and "Schedule anyway"
  overrides it. Interviews are changed, marked done, "did not come" or
  cancelled, and every one has "Add to calendar" (an .ics file for Outlook,
  Google or Apple Calendar; importing it again after a change updates the
  event). Interviewers give feedback (strong yes / yes / no / strong no, and
  comments) once the interview has taken place, and see their colleagues'
  feedback only after giving their own, so the first opinion does not steer
  the rest; recruiters see it all. The dashboard shows "My upcoming
  interviews" (two weeks) and "Feedback to give"; "Email candidate" writes
  the invitation.
- **Email** ✅: write email from a candidate's page (or "Email candidate" on
  an interview). Pick a template, optionally for one of their jobs: it is
  filled from the candidate, the job and its company, the interview (time in
  the sender's time zone, length, how, where, instructions) and the sender.
  A placeholder without a value stays visible as `[job.title]` and sending
  waits until it is filled in. Templates with "client" or "submit" in their
  name go to the job's hiring contact; others to the candidate. Every company
  starts with four templates (interview invitation, submit candidate to
  client, application received, unsuccessful application), changed or added
  to on the templates page (`#/email-templates`). Salary is never a
  placeholder. Sending is queued with the email's record and delivered in the
  background by the worker through your mail server (`HP_SMTP_*`): from your
  sending address, shown as "Sender (Company)", with replies going to the
  person who sent it. The candidate's page lists every email with its state
  (sending / sent / not sent, and why: a refused address, no mail server set
  up); a server that is down for a moment is tried again. Locally, Mailpit
  catches all mail at http://localhost:8025. Replies coming back into the app
  arrive with inbox sync (4.3).
- **Pipeline** ✅: a candidate on a job is an *application* moving through
  stages: applied → screening → submitted (to the client or hiring manager) →
  interview → offer → placed, or rejected / withdrawn with a reason (not
  qualified, salary, client said no, took another offer, ...) and a note. Each
  job has a **board** (drag a card to another column, or use its "Move to"
  menu) showing how long each candidate has been in their stage; closed
  applications are listed below with their reasons and can be reopened. A
  candidate's page lists their jobs and adds them to another. The rules are in
  one place (`pipeline/domain.go`) and tested for every from → to move: a
  reason exactly when closing, reopen before placing, and a placed candidate
  only goes back to offer or is withdrawn. Two people moving the same card:
  the second is told and reloads. Next: dashboard counts and conversion.
- **Interviews:** Google and Microsoft calendar sync, and reminders (4.3).
- **CV vs job match:** the AI worker scores fit and explains it. Every reason
  quotes the CV and the job ad, so a recruiter can check it. It suggests; the
  recruiter decides.

## 5. Notes and activity

✅ Notes (general, screening, interview, call) on candidates, jobs, companies
and contacts, shown with the record's **activity** (who changed what and when,
from the audit log; salary changes never show amounts) in one timeline. A
screening note starts with screening questions to fill in. Hiring managers can
write interview notes. A note can be marked **sensitive** (e.g. salary talks):
only roles that may see salaries can read or write it, the database enforces
it, and others see "2 sensitive notes are hidden". You delete your own notes;
owners and admins any. Next: templates per company and notes on an
application.

## Import

✅ **Import** (menu: Import) candidates, companies or contacts from a CSV file
(in Excel: Save As, CSV). Columns are matched to fields by their names
(`Full Name`, `E-mail`, `Mobile Number`, `Designation`, ...) and can be
changed; the first rows are shown. Each row goes through the same API as the
forms, so every check applies: duplicates are reported as "already there" with
a link, invalid values as "to fix" with the reason, and the rows to fix can be
downloaded as a CSV to correct and import again. Country names and
nationalities become codes (`Malaysian` → `MY`); sources like `Job Street`
become `jobstreet`. Contacts find their company by name (ignoring case and
accents), or add it. Excel files (.xlsx) directly: later.

## Dashboard

✅ The home page: open jobs with candidates per stage, totals, candidates
moved this week, placements in the last 30 days, and who has waited more than
14 days in one stage.

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

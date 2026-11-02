# Roadmap

What is built, and what comes next, in the order that makes the system useful
soonest. Each step is one module (or a small group of them), shipped working
and tested like everything before it. Every module is independent, so steps can
be reordered cheaply: tell us what you need first.

Status: ✅ done · 🔜 next · ⬜ later

## Done

| | What you can do today |
|---|---|
| ✅ | Candidates: add, search, view, edit every profile field (work authorization, notice period, nationality, source, ...) |
| ✅ | Salary (current and expected) that only permitted roles see or edit, enforced by the database too |
| ✅ | CV upload (PDF, Word) with text extraction and OCR for scanned pages |
| ✅ | Roles (owner, admin, recruiter, sourcer, hiring manager, viewer) and an audit log of every change |
| ✅ | Companies: clients, prospects or your own departments; search, add, edit |
| ✅ | Contacts at companies with a built-in email check |
| ✅ | Jobs with a hiring contact, status and a permissioned salary budget |
| ✅ | API documentation at `/docs`; one-click local install and update (`update-hiringplug.bat` / `.command`) |

## The plan

### Phase 1: the daily recruiting loop 🔜

The core of any ATS: clients, their jobs, and candidates moving through each
job. Today the app has candidates but nothing to place them into.

| Step | Delivers | Notes |
|---|---|---|
| ✅ **1.1 Companies** | Client companies (agency) or departments (in-house): list, search, add, edit; website, industry, size, address, owner recruiter | Auto-fill from a website comes in Phase 5. |
| ✅ **1.2 Contacts** | People at companies (hiring managers, HR): name, title, email, phones, LinkedIn; built-in email check (format, the domain's mail servers, disposable and role addresses like `hr@`) | Mailbox-level checks later; many networks block them. |
| ✅ **1.3 Jobs** | Job orders / requisitions: title, company, contact, location, work type, headcount, salary range (sensitive, like candidate salary), status (open, on hold, filled, closed), owner, description | |
| 🔜 **1.4 Pipeline** | Add candidates to a job; stages (applied, screened, submitted, interview, offer, placed, plus rejected / withdrawn with a reason); a drag-and-drop board per job; a candidate's jobs on their page | Stages configurable per company; moves checked in one place and tested move by move. |
| ⬜ **1.5 Notes and timeline** | Screening, CV and interview notes on a candidate, job or application, with templates (screening questions); one timeline per record of notes and changes | Notes that mention salary follow the salary permission. |
| ⬜ **1.6 Dashboard** | My open jobs, candidates per stage, applications stuck too long, activity this week | |
| ⬜ **1.7 Sign-in and team** (last, by choice) | Sign in with email and password (or Google / Microsoft); an admin screen to invite users, change roles and deactivate them | Today users paste a token and are created on the command line. Needed before anyone else uses it. |

### Phase 2: getting candidates in faster ⬜

| Step | Delivers |
|---|---|
| 2.1 **CV parsing** | Upload a CV and get the profile filled in (name, contacts, work history, education, skills), each value showing where in the CV it came from; you review and accept |
| 2.2 **Duplicates** | Warn on the same email, phone or a very similar name; merge two candidates with their CVs, notes and applications |
| 2.3 **Import** | Bring in existing candidates, companies and contacts from Excel/CSV, with a preview and a report of rows that need fixing |
| 2.4 **Tags, skills and lists** | Tags, skills, and shared industry / specialization lists; filters and saved searches |
| 2.5 **CV versions side by side** | Original, JobStreet, LinkedIn and formatted CVs of one candidate compared side by side |

### Phase 3: job ads and applicants ⬜

| Step | Delivers |
|---|---|
| 3.1 **Job ads** | The advertised text for a job (separate from the internal job order), with publish status |
| 3.2 **Careers page and apply form** | A public page per company listing open ads; applying creates the candidate, attaches the CV (parsed) and puts them in the job's pipeline |
| 3.3 **Job boards** | Post ads to job boards through their partner programmes (JobStreet, LinkedIn, Indeed) |

### Phase 4: interviews and email ⬜

| Step | Delivers |
|---|---|
| 4.1 **Interviews** | Schedule interviews for an application (interviewers, time, place or link), reminders, and a feedback form per interviewer |
| 4.2 **Email** | Send email from the app with templates (submit a candidate to a client, invite to interview, rejection), logged on the timeline |
| 4.3 **Calendar and inbox sync** | Google / Microsoft calendar sync for interviews; two-way email sync so replies show on the timeline |

### Phase 5: AI and formatted CVs ⬜

| Step | Delivers |
|---|---|
| 5.1 **Formatted CVs** | Your branded Word template filled from the profile; options to hide contact details or salary; PDF export |
| 5.2 **Blind CVs** | Names, photos and identifying details removed, with an automatic check that nothing leaked |
| 5.3 **AI sidebar (DeepSeek)** | On candidate and job pages: summary, gaps and red flags, "how well does this candidate fit this job?", free questions; it only sees what the user may see |
| 5.4 **CV vs job match** | A fit score per application with reasons quoted from the CV and the job ad |
| 5.5 **Company auto-fill** | Fill company details from the website name or address, as suggestions you accept |

### Phase 6: agency business and reports ⬜

| Step | Delivers |
|---|---|
| 6.1 **Placements and fees** (agency) | Placement record with start date, salary, fee and invoice status; guarantee periods |
| 6.2 **Offers** (in-house) | Offer details and approvals |
| 6.3 **Reports** | Pipeline conversion, time to fill, placements and fees per recruiter, source effectiveness |
| 6.4 **Job ads puller** | Collect hiring signals from allowed sources (careers pages, public ATS feeds, ads you save) as business development leads |
| 6.5 **Integrations** | API keys per company and webhooks |

### Phase 7: cloud and selling it ⬜

Cloud hosting (file storage, managed database, backups with restore drills),
single sign-on, alerts and tracing, company sign-up and billing for SaaS,
custom roles and "own / team / all" data scope per recruiter.

## Older milestone names

Other documents still use the first plan's milestone names. They map to the
phases above like this: **M2** (candidate depth) → Phase 2, plus notes (1.5)
and team / custom roles (1.7, 7); **M3** (clients and pipeline) → Phase 1;
**M4** (AI and formatting) → Phase 5; **M5** (communication) → Phases 3, 4 and
6.1–6.2; **M6** (reach) → 3.3 and 6.3–6.5; **M7** (cloud) → Phase 7 and
sign-in (1.7).

## Waiting on you

- **Your stages.** The pipeline stages you use today and the rejection reasons
  (step 1.4).
- **Anonymised sample CVs** (10–30, including two-column and scanned ones) for
  CV parsing quality tests (step 2.1).
- **Your existing data**, if any, as Excel/CSV exports (step 2.3).
- **Your branded CV template** in Word (step 5.1).
- **The AI provider and hosting region** for DeepSeek (step 5.3).

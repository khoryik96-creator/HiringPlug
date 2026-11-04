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
| ✅ | CV upload (PDF, Word .docx and old .doc) with text extraction, OCR for scanned pages and two-column layouts |
| ✅ | Roles (owner, admin, recruiter, sourcer, hiring manager, viewer) and an audit log of every change |
| ✅ | Companies: clients, prospects or your own departments; search, add, edit |
| ✅ | Contacts at companies with a built-in email check |
| ✅ | Jobs with a hiring contact, status and a permissioned salary budget |
| ✅ | Pipeline: candidates on jobs, a drag-and-drop board per job, reasons for rejections |
| ✅ | Notes (sensitive ones protected) and an activity timeline on every record; a dashboard |
| ✅ | Import candidates, companies and contacts from CSV, with a report of rows to fix |
| ✅ | Duplicates flagged and merged; tags and skills with filters |
| ✅ | CV parsing: details read from each CV, checked against its text, reviewed and accepted on the candidate's page |
| ✅ | Work history and education on profiles; CV versions compared side by side |
| ✅ | Job ads with publishing rules; a public careers page with an apply form; job board links, feed and Google for Jobs |
| ✅ | Interviews with clash checks, calendar files and interviewer feedback |
| ✅ | Email from the app with templates, sent through your mail server; interview reminders |
| ✅ | Placements with guarantee periods, invoices and protected salary and fee |
| ✅ | Reports: conversion, sources, recruiters, time to fill |
| ✅ | Offers with approvals; replacements owed after a fall-through |
| ✅ | API keys and signed webhooks for your other systems |
| ✅ | Hiring leads from public job boards, careers pages and saved ads |
| ✅ | An AI sidebar and CV vs job fit scores with quoted reasons (off until an AI provider is set up) |
| ✅ | Sign-in with passwords, invitations, password resets and a Team page |
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
| ✅ **1.4 Pipeline** | Add candidates to a job; stages (applied, screened, submitted, interview, offer, placed, plus rejected / withdrawn with a reason); a drag-and-drop board per job; a candidate's jobs on their page | Stages configurable per company; moves checked in one place and tested move by move. |
| ✅ **1.5 Notes and timeline** | Screening, CV and interview notes on a candidate, job or application, with templates (screening questions); one timeline per record of notes and changes | Notes that mention salary follow the salary permission. |
| ✅ **1.6 Dashboard** | My open jobs, candidates per stage, applications stuck too long, activity this week | |
| ✅ **1.7 Sign-in and team** | Sign in with email and password (Argon2id, limited attempts, several companies per email); invitations and password resets by emailed link; a Team page to invite, change roles and deactivate (nobody locks themselves out; an owner always remains); changing a password ends other sessions | Google / Microsoft single sign-on can be added later (Phase 7). |

### Phase 2: getting candidates in faster ✅

| Step | Delivers |
|---|---|
| ✅ 2.1 **CV parsing** | Upload a CV and get the profile filled in (name, contacts, nationality, notice period, current job, skills, work history, education), each value showing where in the CV it came from; you review and accept. Tuned on 25 real CVs, including two-column and old .doc files. Work history and education are saved on the profile. Next: the AI reader. |
| ✅ 2.2 **Duplicates** | The same email is refused; the same phone or a very similar name is flagged on the candidate's page; merge two candidates with their CVs, jobs, notes and salary |
| ✅ 2.3 **Import** | Bring in existing candidates, companies and contacts from Excel/CSV, with a preview and a report of rows that need fixing |
| ✅ 2.4 **Tags, skills and lists** | Tags and skills on candidates, suggestions from values already used (also for industry and specialization), and list filters. Saved searches come with reports (6.3). |
| ✅ 2.5 **CV versions side by side** | Original, JobStreet, LinkedIn and formatted CVs of one candidate compared side by side, with the differences in their details (changed dates, missing jobs) listed |

### Phase 3: job ads and applicants 🔜

| Step | Delivers |
|---|---|
| ✅ 3.1 **Job ads** | The advertised text for a job (separate from the internal job order), with publish status, a preview, and ads taken down when the job is filled or closed |
| ✅ 3.2 **Careers page and apply form** | A public page per company listing open ads; applying creates the candidate (or finds them by email), attaches the CV (read and parsed) and puts them in the job's pipeline, with consent, a robot trap and a per-address limit. Choosing your own page address comes with the admin screen (1.7). |
| 🔜 3.3 **Job boards** | ✅ Done without partner accounts: links per board for each ad (applicants are counted by board), real careers page addresses, Google for Jobs data on every ad, and an XML job feed for boards and aggregators that read one. Next, with your partner accounts: posting directly through the boards' partner programmes (SEEK/JobStreet, LinkedIn, Indeed). |

### Phase 4: interviews and email 🔜

| Step | Delivers |
|---|---|
| ✅ 4.1 **Interviews** | Schedule interviews for an application (interviewers, time, place or link, clash check), calendar files, a feedback form per interviewer (hidden from colleagues until they give theirs), and the dashboard's upcoming interviews and feedback to give. Reminders come with calendar sync (4.3). |
| ✅ 4.2 **Email** | Send email from a candidate's page or an interview with templates (interview invitation, submit to client, application received, unsuccessful) filled from the candidate, job, interview and sender; sent in the background through the company's mail server, with replies to the sender; every email listed on the candidate's page with its state. Needs your mail server's SMTP settings for real sending. |
| 🔜 4.3 **Reminders, calendar and inbox sync** | ✅ Interviewers emailed a reminder the day before (moved interviews at their new time, cancelled ones not at all). Waiting on you: Google / Microsoft calendar sync for interviews; replies coming back into the app |

### Phase 5: AI and formatted CVs ✅

| Step | Delivers |
|---|---|
| ✅ 5.1 **Formatted CVs** | The profile filled into a Word template (built-in standard, or your own uploaded one with placeholders); contact details hidden unless asked, salary only when asked by those who may see it, a summary, for a particular job; saved as the formatted CV, with a PDF |
| ✅ 5.2 **Blind CVs** | A reference instead of the name; no contact details, nationality or work authorization; names, emails, phones and links scrubbed from text; the finished file checked for leaks before it is saved |
| ✅ 5.3 **AI sidebar** | On candidate and job pages: summary, gaps and red flags, "how well does this candidate fit this job?", free questions; it sees only what the user may see, never names, contact details or salary; any OpenAI-compatible provider (DeepSeek), off until set up; a monthly token allowance per company |
| ✅ 5.4 **CV vs job match** | A fit score per application with reasons quoted from the job (description and ad) and the CV, every quote checked and the score worked out from what passes; requirements about who a person is never scored; on the board (best fit first) and the candidate's page; off until the AI provider is set up, sharing the monthly allowance. Next: shortlisting candidates not yet on the job. |
| ✅ 5.5 **Company auto-fill** | Name, phone, address, country, size, registration number and description read from the company's website (schema.org data, its contact page; robots.txt respected), as suggestions with their source you accept or dismiss |

### Phase 6: agency business and reports ✅

| Step | Delivers |
|---|---|
| ✅ 6.1 **Placements and fees** (agency) | Placement per application (moves it to placed) with start date, guarantee period and fall-throughs, recruiter credited, invoice and payment; agreed salary and fee (a percentage of the yearly salary or fixed) visible only to those allowed; a Placements page with fee totals and guarantees ending soon |
| ✅ 6.2 **Offers** | Offer terms (protected salary, start, answer-by date, other terms) on an application; approval by someone other than who asked (approvers may send straight away); sent, accepted, declined or withdrawn; offers to approve on the dashboard; a replacement owed after a fall-through |
| ✅ 6.3 **Reports** | Pipeline conversion by furthest stage reached, sources (applications, interviews, placements), placements and fees per recruiter, time to fill and to place, for any period |
| ✅ 6.4 **Job ads puller** | Leads from companies' public job boards (Greenhouse, Lever, Ashby, SmartRecruiters) and careers pages with job markup (robots.txt respected), checked daily, openings closed when no longer listed; ads saved by hand; statuses and links to companies |
| ✅ 6.5 **Integrations** | API keys acting as a role (hashed, shown once, revocable); signed webhooks for chosen events, queued with the change, retried, never to private networks; an Integrations page |

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
- **Your existing data**, if any, as Excel/CSV exports (step 2.3).
- **Your branded CV template** in Word (step 5.1): download the standard one
  from the CV templates page, restyle it, and upload it (the built-in one works
  meanwhile).
- **The AI provider and hosting region** for DeepSeek (steps 5.3 and 5.4 are built and switched off until then): its address, key and model name, set as `HP_AI_PROVIDER_URL`, `HP_AI_PROVIDER_KEY`, `HP_AI_MODEL` with `HP_AI_PROVIDER=openai_compatible`.
- **Calendar sync** (step 4.3): a Google Cloud OAuth app and/or a Microsoft
  Entra app registration (client ID and secret), and the address people reach
  HiringPlug at (for the sign-in redirect).
- **Replies in the app** (step 4.3): either each user connects their mailbox
  (Google / Microsoft sign-in), or replies go to one shared inbox the app reads
  (its IMAP settings).
- **Your mail server's SMTP settings** (step 4.2) to send real email.
- **Job board partner accounts** (SEEK/JobStreet, LinkedIn, Indeed) to post ads directly (step 3.3); until then, ads are posted with their tracked links or picked up from the feed.

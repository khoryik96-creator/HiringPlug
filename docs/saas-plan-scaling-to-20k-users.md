# Turning HiringPlug into a SaaS: scaling to 20,000 users

The plan for selling HiringPlug to recruitment agencies and freelance
recruiters as a monthly service: what has to change in the app, how to set
it up at each size, what it costs, and what limits growth.

**The target.** About **1,000 agencies** (freelancers to agencies of 20
recruiters, about 5 on average), **5,000 to 20,000 users online at the same
time**. That is more agencies than Malaysia has of HiringPlug's kind: reaching
it means selling across Southeast Asia (see [The market](#10-the-market-agencies-you-can-sell-to)).
The first goals are much smaller: about 10 agencies covers a lean set-up.

**How sure these numbers are.** The speeds come from a load test on one small
machine (see [What we measured](#what-we-measured)); the sizes and costs are
estimates from those measurements and typical cloud prices (November 2026).
Prices change: check them before buying, and **run a load test on the real
servers before launch**.

**Currency.** All costs are in Malaysian ringgit, converted from the US
dollar prices cloud and AI providers charge at **RM 4.10 to US$1** (the rate
was about RM 4.06 at the end of October 2026; rounded up a little to leave
room). Providers bill in US dollars, so the ringgit cost moves with the rate.

Contents:

1. [Where HiringPlug stands today](#1-where-hiringplug-stands-today)
2. [Changes for one agency, not all](#2-changes-for-one-agency-not-all)
3. [What has to change before selling](#3-what-has-to-change-before-selling)
4. [Set-up at each size, with costs](#4-set-up-at-each-size-with-costs)
5. [The biggest cost: AI](#5-the-biggest-cost-ai)
6. [Python or Go?](#6-python-or-go)
7. [What we measured](#what-we-measured)
8. [Keeping costs down](#8-keeping-costs-down)
9. [Order of work](#9-order-of-work)
10. [The market: agencies you can sell to](#10-the-market-agencies-you-can-sell-to)

---

## 1. Where HiringPlug stands today

Already right for a SaaS:

- **Every agency is walled off by the database.** Each agency is a *tenant*;
  every table holding its data has row-level security, so the database itself
  refuses another agency's rows, and a test fails if a table is added without
  it (`TestEveryTenantTableHasRowLevelSecurity`).
- **A lot is already set per agency**: mandatory fields and dropdown options,
  roles, permissions and teams, company categories, screening call questions,
  email templates and banners, CV templates, benefits and terms templates,
  list columns, the agency's name on formatted CVs, and its own AI keys.
- **Servers hold no sign-in state** (sign-in is a token), so more copies of
  the API can run behind a load balancer.
- **Background work goes through a queue in the database**, so more workers
  on more machines simply take more jobs.

Not ready yet (details in [section 3](#3-what-has-to-change-before-selling)):
new agencies are created by hand (`hp tenant`); no plans or billing; files
are on one server's disk; two rate limits are kept in a server's memory; one
mail server for everyone; one AI allowance for every agency; a few defaults
are Hyppies'.

## 2. Changes for one agency, not all

Agencies will ask for changes. The rule that keeps a SaaS alive: **one app
for every customer, never a copy per agency.** Copies mean every fix is made
many times and they drift apart. Instead:

1. **Make the request a setting**, off (or as today) by default, that each
   agency fills in its own way. This is how screening call questions,
   categories and templates already work.
2. **Feature switches per agency** (to build): a list of which features each
   agency has on. Use it to try a new feature with one agency first, to sell
   add-ons and plan tiers, and to keep something only one client wants away
   from the rest.
3. **Custom fields per agency** (to build): each agency adds its own fields to
   candidates, jobs and companies. This is the most common agency request;
   built once, it saves dozens of one-off changes.
4. **One-off work through integrations**, not app changes: the public API and
   webhooks ("tell me when a candidate is placed") for links to an agency's
   payroll or website. Charge for it as custom work.
5. **Say no, or "on the roadmap"**, when a request is right for one agency only.

Freelancers and agencies use the same app; the feature switches give a
freelancer a simpler one (no teams, roles or approval steps).

## 3. What has to change before selling

Sizes: **S** a few days, **M** one to two weeks, **L** several weeks.

### To sell it

| Change | Why | Size |
|---|---|---|
| Neutral defaults; Hyppies becomes an ordinary customer | Benefits template, invoice example and a few texts are Hyppies' | S |
| Self-serve sign-up with a free trial | Today `hp tenant` on the server creates an agency | M |
| Plans, seats and billing (e.g. Stripe): card payments, invoices, limits per plan (users, storage, AI) | No plans or billing exist | L |
| Feature switches per agency | Changes for one agency only ([section 2](#2-changes-for-one-agency-not-all)) | M |
| Custom fields per agency | The most common request | L |
| AI allowance per agency or plan | `HP_AI_MONTHLY_TOKENS` is one number for every agency | S |
| Email from each agency's own domain, or their own Gmail / Outlook | Today all mail leaves through one server address; agencies' mail must not land in spam | M–L |
| Your own control panel: all agencies, plan, usage, suspend, support access (with their permission) | Running the business | M |
| Two-step sign-in; single sign-on for large agencies later | Business customers ask for it | M |
| Export everything, and delete everything when an agency leaves | PDPA and trust | M |
| Terms of service, privacy policy, data processing agreement | Each agency owns its candidates' data; you process it for them | (legal) |

### To run on more than one server

| Change | Why | Size |
|---|---|---|
| Files in cloud storage (S3 or compatible) | Uploads are on one server's disk (`HP_FILES_DIR`); a second server cannot see them. `platform/blob` is built to switch | M |
| Sign-in and booking-page rate limits in the database (or Redis) | They are kept in each server's memory (`identity/auth.go`, `scheduling`), so each server counts separately | S |
| Database connections: size each server's pool; PgBouncer in front of the database | Each API server opens at most 4 connections by default (pgx: `pool_max_conns` in `HP_DATABASE_URL` raises it); many servers × many connections needs a pooler | S |
| Two queue lanes: AI calls, and CV reading / formatting | AI calls mostly wait 10–30 s on the AI company; in one queue a burst of them delays CV reading | M |
| Monitoring and alerts: errors, slow requests, queue depth, database load | You cannot run 1,000 agencies blind | M |
| Managed backups with point-in-time recovery, and a tested restore | `hp backup` is nightly; a SaaS needs "back to 10:42 this morning" | S (managed database) |

### At 5,000+ users online

| Change | Why | Size |
|---|---|---|
| Read-only copies of the database for lists and reports | The database is the first real limit | M |
| A search engine for CVs (OpenSearch, Typesense or Meilisearch) fed by jobs | Searching tens of millions of CVs in PostgreSQL gets slow ([ADR 0003](adr/0003-postgresql.md)) | L |
| Split the biggest tables by month (audit log, activity, finished jobs) | They grow fastest | M |
| Python workers added and removed with the queue length | Busy mornings, quiet nights | M |
| Web pages from a CDN | Faster for users, nothing for the servers to do | S |

## 4. Set-up at each size, with costs

Two price levels per stage: **budget** (rented servers from a provider such
as Hetzner or DigitalOcean, running PostgreSQL yourself or on their managed
service) and **AWS-style** (managed database, load balancer and storage from
a big cloud; more expensive, less to run yourself). Monthly, in US dollars,
**without AI** (see [section 5](#5-the-biggest-cost-ai)).

For data kept in Malaysia, choose a Malaysian or Singapore region.

### Stage 0: today, pilot (up to ~20 agencies, ~100 users)

One machine with Docker Compose (`deploy/docker-compose.yml`): PostgreSQL,
API, Go worker, Python worker, web. Backups copied off the machine every night.

| | Budget | AWS-style |
|---|---|---|
| One server, 8 cores, 16–32 GB | RM 160–330 | RM 620–1,000 |
| Off-site backups, email sending service | RM 40–120 | RM 80–200 |
| **Total** | **RM 200–450** | **RM 700–1,200** |

### Stage 1: launch (up to ~200 agencies, ~1,000 users online)

Needs the "more than one server" changes above.

```
            load balancer
           /             \
      API + Go worker   API + Go worker        (2 servers, 4 cores, 8 GB)
                  \       /
     managed PostgreSQL (8 cores, 32 GB) ── nightly + point-in-time backups
                    |
   Python workers (1–2 servers, 8 cores, 16 GB)      S3 file storage
```

| | Budget | AWS-style |
|---|---|---|
| 2 API servers | RM 120–250 | RM 820–1,200 |
| Database (8 cores, 32 GB, backups) | RM 620–1,200 | RM 2,900–4,100 |
| 2 Python worker servers | RM 250–490 | RM 1,200–1,800 |
| Files (1–3 TB), load balancer, email, monitoring | RM 250–620 | RM 820–1,600 |
| **Total** | **RM 1,240–2,560** | **RM 5,740–8,700** |

### Stage 2: 1,000 agencies, 5,000 users online

| | Budget | AWS-style |
|---|---|---|
| 3 API servers (4 cores) | RM 180–370 | RM 1,200–1,800 |
| 2 Go worker servers | RM 120–250 | RM 820–1,200 |
| Python workers, 4–6 servers (8 cores), more in busy hours | RM 620–1,200 | RM 2,900–4,900 |
| Database primary (16 cores, 64 GB) + 1 read copy + PgBouncer | RM 2,000–3,700 | RM 10,200–14,300 |
| Files (grows ~1 TB a month at heavy use; older CVs to cheaper storage) | RM 200–1,200 | RM 410–2,500 |
| Load balancer, email, monitoring, logs | RM 410–1,000 | RM 1,200–2,900 |
| **Total** | **RM 3,530–7,720** | **RM 16,730–27,600** |

About **RM 0.70–5.50 per user per month** without AI.

### Stage 3: 20,000 users online

| | Budget | AWS-style |
|---|---|---|
| 8–10 API servers | RM 620–1,200 | RM 3,700–6,100 |
| Go workers, 4 servers | RM 250–490 | RM 1,600–2,500 |
| Python workers, 15–20 servers at busy hours (scaled down at night) | RM 2,000–4,100 | RM 10,200–16,400 |
| Database primary (32 cores, 128 GB) + 2 read copies + PgBouncer | RM 6,100–10,200 | RM 32,800–49,200 |
| CV search engine, 3 servers | RM 820–1,600 | RM 4,900–8,200 |
| Files (10+ TB, older ones on cheaper storage) | RM 820–3,300 | RM 2,000–6,100 |
| Load balancer, CDN, email, monitoring, logs | RM 1,200–2,500 | RM 4,100–8,200 |
| **Total** | **RM 11,810–23,390** | **RM 59,300–96,700** |

About **RM 0.60–4.80 per user per month** without AI. At this size, hire (or
contract) someone to look after the servers and the database.

### How the sizes were worked out

**Users online → requests.** A recruiter at work opens a page every 10–30
seconds; a page makes 3–5 requests: **0.2–0.5 requests a second per user**.
5,000 online: 1,000–2,500 a second; 20,000 online: 4,000–10,000.

**API servers.** One 4-core machine running the API **and** the database
answered **about 1,300 requests a second** with no errors. A 4-core server
running only the API should do 1,000–2,000. Hence 2–3 servers for 5,000
users online and 6–10 for 20,000, with room to spare.

**Background work.** Assumed per recruiter per day: 30 CVs read, 5
formatted CVs, 20 AI questions or fit scores; a fifth of the day's work in
the busiest hour (Malaysian agencies keep the same hours).

| Busiest hour, 5,000 recruiters | Rate | Cost |
|---|---|---|
| CVs read (80% text, 20% scans) | ~8 a second | ~10 cores |
| Formatted CVs (Word → PDF with LibreOffice) | ~1.4 a second | ~4–5 cores, ~1 GB |
| AI questions (waiting on the AI company) | ~6 a second, ~85 waiting at once | almost no processing; ~6 Python processes, ~0.6 GB |

About **15 cores and 4–6 GB** of memory for the workers at 5,000 recruiters;
multiply by four for 20,000. If your agencies read fewer CVs a day, everything
here shrinks with it.

**Storage.** 150,000 CVs a day at ~300 KB is about **1 TB of files a month**,
plus ~30 GB a month of CV text in the database. Most agencies will read far
fewer CVs; measure it in the pilot.

## 5. The biggest cost: AI

At scale, AI costs more than all the servers together. Rough sum: AI calls ×
words sent and received × the provider's price.

Example: 5,000 recruiters × 20 AI calls a day × 22 working days = **2.2
million calls a month**, each ~3,000 tokens in and ~700 out:

| Model (price per million tokens in / out, check current prices) | Per month | Per user |
|---|---|---|
| DeepSeek (~RM 1.23 / ~RM 1.84) | ~RM 11,000 | ~RM 2.19 |
| Claude Haiku (~RM 4.10 / ~RM 20.50) | ~RM 58,600 | ~RM 11.73 |
| Claude Sonnet (~RM 12.30 / ~RM 61.50) | ~RM 175,900 | ~RM 35.18 |

What to do about it:

- **Agencies bring their own AI key** (already built: Settings > AI provider):
  they pay the AI company directly; you charge for the app.
- **Or include AI in the plan with a monthly allowance per plan** (to build:
  the allowance is one number for everyone today), and sell top-ups.
- Default to a low-cost model; offer stronger models on higher plans.
- Cache answers that are asked again (the same CV and job), and score fit
  only when asked.

## 6. Python or Go?

HiringPlug uses Go for the website's server (API) and most background work,
and Python for reading CVs, CV Studio's formatting and talking to the AI.
**Keep Python.** Measured on this project:

| Part | At rest | Busiest moment measured |
|---|---|---|
| Go API | 39 MB | 89 MB at 500 requests at once |
| Go worker | 35 MB | |
| **Python worker** | 75 MB | 133 MB, reading a scanned CV |
| LibreOffice (started by the Python worker; Word → PDF, old .doc) | 0 | **208 MB** |
| Tesseract (started by the Python worker; reads scans) | 0 | 62 MB |

- **People clicking around never touch Python**: pages are served by the Go
  API and the database. Python only does background work.
- The heavy parts (LibreOffice, Tesseract, the PDF library) are separate C
  programs; **Go would need the same ones**. Python itself costs about 40–90
  MB more per worker than Go: a few GB at 20,000 users, about **RM 200–620 a
  month**.
- **Reading CVs stays in Python**: its PDF and Word libraries were tuned on
  real CVs in CV Studio; Go's PDF libraries are weaker, so names, phones and
  dates would come out worse.
- **CV Studio's formatting stays in Python**: about 10,000 lines kept exactly
  as in CV Studio, so CV Studio's fixes can be copied across.
- **Where Go would help, at scale**: the jobs that only send text to the AI
  and wait (assistant, fit scores, AI search ranking). Go waits on thousands
  of answers in one small program; Python needs more processes. Move them
  only if their lane becomes a real cost (Stage 3).

## What we measured

10 November 2026, one 4-core, 16 GB machine running PostgreSQL 16, the API
and the load generator together; 3,009 candidates in the agency.

**API load test.** A Go program kept loading the pages a recruiter opens
(the candidate list, a candidate, their notes and emails, a job's pipeline,
a search, the signed-in user) as fast as answers came back:

| Requests at once | Answered per second | Typical wait (95% under) | API memory | Errors |
|---|---|---|---|---|
| 50 | 1,272 | 0.04 s (0.05 s) | 59 MB | none |
| 200 | 1,332 | 0.15 s (0.17 s) | 69 MB | none |
| 500 | 1,337 | 0.38 s (0.41 s) | 89 MB | none |

The ceiling was the shared 4 cores (the API at one core, the database's four
connections at ~40% each), not an error or memory.

**Background jobs** (from the worker's own timings): a text PDF read in under
0.1 s; a scanned page 1.5–2 s; reading details out of a CV 15–60 ms; a
formatted CV's PDF 1.4–6.5 s (LibreOffice; the first is slowest).

**Before launch**, repeat this on the real servers with realistic data
(hundreds of agencies, millions of candidates) and a busy-hour mix of reads,
uploads and AI calls.

## 8. Keeping costs down

- **Start at Stage 0 and grow**: the changes in [section 3](#3-what-has-to-change-before-selling)
  let you add servers as customers arrive; do not buy Stage 3 on day one.
- **Budget providers first**: the same set-up costs a third to a quarter of a
  big cloud; move parts (e.g. the database) to managed services when the time
  you spend running them costs more.
- **AI**: agencies' own keys, or allowances per plan ([section 5](#5-the-biggest-cost-ai)).
- **Workers by the hour**: add Python workers on busy mornings, remove them
  at night.
- **Older files to cheaper storage** (e.g. CVs untouched for a year).
- **Fewer simultaneous jobs on small servers**: `HP_AI_CONCURRENCY` (3 by
  default) limits how many CVs are read or formatted at once, and so the
  memory peaks; jobs then queue a few seconds longer.
- **Price per user** with the cost per user in mind: servers cost roughly
  RM 0.60–5.50 per user a month; AI, if included, much more.

## 9. Order of work

1. Neutral defaults; Hyppies as an ordinary customer.
2. Feature switches and custom fields per agency.
3. Files in cloud storage; rate limits in the database; queue lanes;
   monitoring; managed database with point-in-time backups (Stage 1 ready).
4. Self-serve sign-up, plans, billing, AI allowance per plan, your control
   panel.
5. Email from each agency's own domain or mailbox; two-step sign-in; export
   and delete on leaving; legal documents.
6. Load test on the Stage 1 servers; launch with a few friendly agencies.
7. At ~2,000 users online: read copies of the database, PgBouncer, workers
   that scale with the queue; then a CV search engine and split tables as
   you approach Stage 3.

## 10. The market: agencies you can sell to

How many recruitment agencies there are in each country, and how many are
the kind HiringPlug is for: agencies placing professionals and executives
with companies, and freelance recruiters. Searched November 2026.

**Read the counts with care.** No country publishes a count of that kind of
agency. Licence lists include agencies that bring in foreign workers or
domestic helpers (a different business with different software needs), and
business registers include staffing and outsourcing firms. The "likely
targets" column is **our rough estimate** of the professional-recruitment
share, not a statistic: check it against the official lists before planning
on it.

| Country | What is counted | Count | Likely targets (estimate) | Source |
|---|---|---|---|---|
| **Malaysia** | Registered private employment agencies, Peninsular Malaysia only (Labour Department list, March 2026, as reported by a business-services firm; not verified against the list) | **1,273**, of which 833 in the Klang Valley | 300–500 | [MISHU](https://mishu.my/blog/company-incorporation-and-formation/open-private-employment-agency/); for comparison 453 licensed agencies in 2018 after the 2017 amendments ([The Star](https://thestar.com.my/news/nation/2018/12/20/only-453-employment-agencies-left)) |
| **Singapore** | Licensed employment agencies (Ministry of Manpower, 2020; the live list is MOM's EA Directory). Many place foreign domestic workers | **about 3,900** | 1,300–2,000 | [People Matters](https://sea.peoplemattersglobal.com/news/recruiting-and-onboarding/employment-agencies-in-singapore-to-follow-new-licensing-conditions-from-october-26098); [MOM: check a licensed EA](https://www.mom.gov.sg/faq/employment-agencies/why-should-i-work-with-a-licensed-ea-and-how-can-i-check-if-they-are-licensed) |
| **Indonesia** | Recruitment and staffing companies in the business register (NACE 78, a commercial directory); includes outsourcing. No count of headhunters found | **1,322** | 300–600 | [InfobelPRO Indonesia](https://www.infobelpro.com/companies/indonesia/recruitment-and-staffing) |
| **Thailand** | Recruitment and staffing companies in the business register (NACE 78, a commercial directory); Department of Employment licence counts found were small and undated (130; 218 in an older study) | **3,736** registered | 200–500 | [InfobelPRO Thailand](https://www.infobelpro.com/companies/thailand/recruitment-and-staffing); [Bangkok Post](https://www.bangkokpost.com/thailand/general/2938190/job-seekers-told-to-verify-recruitment-agencies) |
| **Vietnam** | Headhunting firms, as estimated by a Vietnamese HR firm; 11,076 recruitment and staffing companies in the business register (much broader) | **300–500** headhunters | 300–500 | [Talentnet](https://talentnetgroup.com/vn/featured-insights/trends-developing-headhunters-in-vietnam); [InfobelPRO Vietnam](https://www.infobelpro.com/companies/vietnam/recruitment-and-staffing) |
| **Philippines** | Private employment agencies are licensed by DOLE's regional offices; no total count found | — | not estimated | [DOLE rules for private employment agencies](https://www.digest.ph/labor/rules-and-regulations-governing-private-recruitment-and-placement-agency-for-local-employment) |

Freelance recruiters are not in these counts, and in every country they add
to the market (on the Solo plan).

**What it means for the targets.** Across the five countries with figures,
the likely targets come to roughly **2,400–4,100 agencies**, plus the
Philippines and freelancers.

| Goal | Share of the likely targets |
|---|---|
| 10 agencies (covers a lean set-up) | Malaysia alone, about 2–3% of its likely targets |
| 60 agencies | Malaysia alone, about 12–20%: possible, most of it in the Klang Valley |
| 200–400 agencies | 5–15% of the region: a realistic regional business in a few years |
| 1,000 agencies | 25–40% of the region: a market leader's share; plan for it only after the first two countries work |

**Order of countries.**

1. **Malaysia**: home, Hyppies as the reference customer, the app already
   fits (ringgit, SST, PDPA, Malaysian work permits).
2. **Singapore**: the most agencies, English, the same time zone, and higher
   prices than Malaysia (charge in Singapore dollars).
3. **Philippines**: English, a large recruitment and outsourcing industry.
4. **Indonesia, Vietnam, Thailand**: large markets, but each needs the app in
   its own language and a local partner or sales person.

**What the app needs for each country** (check the rates before launch):

| Country | Currency and tax on the subscription | Language | Data protection law | Work passes to track |
|---|---|---|---|---|
| Malaysia | RM; SST (8%) | English, Malay | PDPA 2010 (built) | Built (local, RP-T, PR, spouse visa, needs a visa) |
| Singapore | SGD; GST 9% | English | PDPA 2012 | Employment Pass, S Pass, Work Permit, Dependant's Pass |
| Indonesia | IDR; VAT (PPN) 11–12% | Bahasa Indonesia | PDP Law 2022 | KITAS and work permits for foreigners |
| Thailand | THB; VAT 7% | Thai | PDPA 2019 | Work permit and visa for foreigners |
| Vietnam | VND; VAT 10% (8% while a temporary cut lasts) | Vietnamese | Decree 13/2023 and the personal data law | Work permit for foreigners |
| Philippines | PHP; VAT 12% | English | Data Privacy Act 2012 | Alien Employment Permit, 9(g) visa |

The app already keeps settings per agency; these would become per-country
settings (currency, tax, work pass types, language), chosen when an agency
signs up.

# Roadmap

Each milestone ships working, tested features on the same foundation. The
order puts the core first (candidates, CVs, pipeline), then the things that
depend on it.

| Milestone | Delivers |
|---|---|
| **M1 Foundation** ✅ | Contract and code generation for Go, Python and TypeScript; company isolation in the database; roles and salary permissions; candidates; CV upload with text extraction and OCR; audit log; job queue; web app shell; API docs; CI; local deployment; module generator |
| **M2 Candidate depth** | CV parsing into a profile (source-grounded, with a sample-CV quality test); duplicate detection (email, phone, similar names); industry and specialization lists; tags and skills; notes (screening, CV, interview) with templates; activity timeline; custom roles, invites, data scope (own/team/all); CSV data import; images and old .doc CVs |
| **M3 Clients and pipeline** | Companies (auto-fill), contacts (email verifier); jobs and job ads; pipeline stages with a board per job and a dashboard; interview scheduling basics |
| **M4 AI and formatting** | Formatted CVs from Word templates; blind CVs with a leak check; AI assistant sidebar (DeepSeek or another provider, permission-aware); CV-to-job matching with quoted reasons; AI cost limits |
| **M5 Communication** | Email (send, templates, two-way sync); calendar sync; careers page and apply flow; placements and fees (agency); offers (in-house) |
| **M6 Reach** | Job ads puller (business development leads); job board posting; public API keys and webhooks; reports |
| **M7 Cloud** | S3 file storage, managed PostgreSQL, single sign-on (Google, Microsoft), backups and restore drills, tracing and alerts, company sign-up for SaaS |

Some items can move earlier if you need them sooner. Each module is
independent, so changing the order is cheap.

## Waiting on you

- **Anonymised sample CVs** (10–30, including two-column and scanned ones)
  for the parsing quality test and two-column reading order (M2).
- **Your branded CV template** in Word (M4).
- **The AI provider and hosting region** for DeepSeek (M4).
- **Job ads puller sources** you want first (M6).

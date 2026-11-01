# Integrations

Every integration follows the same pattern, so each one is easy to add and
cannot take the core down:

- **Behind an interface**, one implementation per provider. Swapping a provider
  doesn't touch business code.
- **Through the job queue**, never while a user waits. Calls to other systems
  are retried with growing waits and end up `dead` (visible) instead of lost.
- **Inbound data is stored before it is processed** (webhooks, emails, job ads),
  so it can be replayed after a fix.
- **Secrets per company** are stored encrypted, never logged, and never
  returned by the API.

| Integration | Purpose | Notes | Milestone |
|---|---|---|---|
| Email verifier | Check contact emails | Syntax, MX, disposable and role checks built in; mailbox (SMTP) checks need port 25, often blocked, so a provider can be plugged in. Results keep their evidence and date. | M3 |
| Company auto-fill | Fill company details from a name or website | Website structured data, then AI extraction; optional Apollo / Google Places / business registry. Suggestions with source; a person accepts. | M3 |
| Calendars | Interview scheduling | Google and Microsoft 365 (OAuth per user); free/busy and invites | M3–M5 |
| Email | Send, templates, two-way sync | Outbound via SMTP or a provider; inbox sync with Gmail / Microsoft Graph; messages linked to candidates and contacts | M5 |
| Careers page | Public job list and apply form | A small public site per company, served from the API under `public` permissions; applications enter the pipeline | M5 |
| Job ads puller | Find companies that are hiring | Recruiter-saved ads, careers pages and public ATS feeds, licensed data. No automated scraping of LinkedIn or JobStreet (their terms forbid it). | M6 |
| Job boards | Post ads, receive applicants | Official partner APIs (e.g. SEEK/JobStreet, LinkedIn Jobs) or a multiposting service | M6 |
| Data import | Move from your current ATS | CSV import with a preview and mapping; API importers where the old system allows | M2 |
| Public API and webhooks | Let other systems integrate | API keys per company with permissions; webhooks from the same events that write the audit log; documented at `/docs` | M6 |
| AI providers | Parsing, matching, assistant | One provider interface in Python; provider and region per company; usage and cost tracked | M2–M4 |

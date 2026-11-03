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
| Job ads puller ✅ | Find companies that are hiring | Public ATS feeds (Greenhouse, Lever, Ashby, SmartRecruiters), careers pages with JobPosting markup (robots.txt respected), ads saved by hand; licensed data later. No automated scraping of LinkedIn or JobStreet (their terms forbid it). | 6.4 |
| Job boards | Post ads, receive applicants | Official partner APIs (e.g. SEEK/JobStreet, LinkedIn Jobs) or a multiposting service | M6 |
| Data import | Move from your current ATS | CSV import with a preview and mapping; API importers where the old system allows | M2 |
| Public API and webhooks ✅ | Let other systems integrate | API keys per company acting as a role; webhooks from the same events that write the audit log; documented at `/docs` (see below) | 6.5 |
| AI providers | Parsing, matching, assistant | One provider interface in Python; provider and region per company; usage and cost tracked | M2–M4 |

## Connecting your own systems: API keys and webhooks ✅

**API keys** (Integrations page, owners and admins). A key acts as the role
chosen for it (admin, recruiter, sourcer, hiring manager or viewer; never
owner), in your company only. Send it as `Authorization: Bearer hpk_...` to any
endpoint in `/docs`. The key is shown once; only its SHA-256 is stored. A
revoked key stops working at once. Changes made with a key are recorded with
the key as the actor. Keys cannot create keys or webhooks, even as admin.

**Webhooks.** Choose the events (candidate created or updated, application
added or moved, careers applications, interviews scheduled or changed,
feedback, offers approved, sent, accepted, declined or withdrawn, placements,
jobs, companies, contacts). Every change is already written to the audit log;
a database trigger turns each one a webhook listens for into a delivery,
queued in the same transaction as the change, so nothing is announced that
did not happen, and nothing that happened is missed. Salary changes are never
events, and salary values never appear in them.

Each delivery is a JSON `POST`:

```json
{"id": "whd_...", "event": "application.moved", "occurredAt": "2026-11-03T02:00:00Z", "actorId": "usr_...",
 "entity": {"type": "application", "id": "appl_..."}, "changes": {"stage": {"from": "interview", "to": "offer"}}}
```

with headers `X-HiringPlug-Event`, `X-HiringPlug-Delivery` (the same `id`, to
ignore repeats) and `X-HiringPlug-Signature: t=<unix time>,v1=<hex>`, where
`<hex>` is the HMAC-SHA256 of `<unix time>.<raw body>` with the webhook's
secret (shown once when the webhook is added). Check it, and that the time is
within a few minutes, before trusting a delivery. In Python:

```python
import hashlib, hmac, time

def verify(secret: str, header: str, body: bytes) -> bool:
    parts = dict(p.split("=", 1) for p in header.split(","))
    expected = hmac.new(secret.encode(), f"{parts['t']}.".encode() + body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, parts["v1"]) and abs(time.time() - int(parts["t"])) < 300
```

Answer with any 2xx within 10 seconds. Anything else (or no answer) is tried
again after 1 minute, 5 minutes, 30 minutes, 2 hours and 12 hours, then the
delivery fails; each webhook's latest deliveries, with the answer or error,
are on the Integrations page, and "Send test" sends a `webhook.ping`.
Addresses must be HTTPS. Deliveries never go to the server's own or private
networks (checked on the address actually connected to) and never follow
redirects (the same protection covers careers pages read for leads);
`HP_ALLOW_PRIVATE_NETWORKS=true` lifts both for trying webhooks on
your own machine (refused in production).


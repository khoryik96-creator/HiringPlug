# Architecture

HiringPlug is **one codebase split into strict modules** (a *modular
monolith*), written in three languages, each where it is strongest:

- **Go** runs the API, the business rules, the database and core background
  jobs. It is fast, strict and stable: the part that must never break.
- **Python** reads documents and runs AI: text extraction, OCR, CV parsing,
  PDFs and the AI assistant; matching next. It has the best
  libraries, and keeping this fast-changing, less predictable work separate
  protects the core.
- **TypeScript** (React) is the web app recruiters use.

## The big picture

```
 Recruiters (browser)
        │  HTTPS, typed calls (types generated from the contract)
        ▼
 ┌──────────────────────────────────────────────┐
 │ Go API  (cmd/api)                            │
 │  contract checks: route, sign-in,            │
 │  permission, input  →  module handlers       │
 │  identity · candidates · documents · ...     │
 └─────────────┬──────────────────────▲─────────┘
               │ one transaction:     │ reads
               │ data + queued job    │
               ▼                      │
 ┌──────────────────────────────────────────────┐
 │ PostgreSQL                                   │
 │  data (row-level security per company)       │
 │  job queue (shared by Go and Python)         │
 │  audit log                                   │
 └───────┬──────────────────────────▲───────────┘
   "ai"  │ jobs                     │ "core" jobs (results)
         ▼                          │
 ┌───────────────────┐       ┌──────┴─────────────┐
 │ Python AI worker  │       │ Go worker          │
 │ (ai/)             │       │ (cmd/worker)       │
 │ CV text, OCR, ... │       │ saves results,     │
 └─────────┬─────────┘       │ runs core jobs     │
           │ signed link     └────────────────────┘
           ▼
     CV files (local disk now; S3 in the cloud)
```

Rules of the road:

- The **web app talks only to the Go API**. The Go API is the only way in:
  sign-in, permissions, company isolation and validation live in one place.
- **Go and Python talk only through the job queue**, never by calling each
  other while a user waits. Uploads return immediately; if Python is slow or
  down, work waits and the app keeps working.
- **Go owns the database and every business rule.** Python never writes
  business tables; it returns results as jobs, and Go saves them. A rule is
  never written twice in two languages.
- **The contract is the single source of truth** (`contracts/`). Go types, the
  TypeScript client and the Python models are generated from it.

## Modules

A module is one area of the business (candidates, documents, jobs, pipeline,
companies, notes, ...). Each one:

- has its **own contract file** (`contracts/modules/<module>.yaml`) declaring
  its endpoints, data shapes, permissions, error codes and jobs;
- **owns its tables** (only its own SQL file touches them);
- has the **same layout**, so every module reads the same way:

```
backend/internal/modules/candidates/
  candidates.go    public API: what the app and other modules may use
  domain.go        the rules, as pure functions (no database, no HTTP)
  service.go       use cases: one transaction each, rules + storage + audit
  http.go          HTTP handlers: contract types <-> service, no business rules
  internal/store/  generated from db/queries/candidates.sql (private to the module)
```

Dependencies point one way: `http → service → domain`, with storage used by the
service. `domain.go` imports no database or HTTP code, so the rules are tested
with plain unit tests.

**Modules never import each other.** When `documents` needs to check that a
candidate exists, it declares the small interface it needs
(`Exists(ctx, tx, id)`), and `internal/app/modules.go`, the one place where
modules meet, passes the candidates module in. Adding or changing a module
therefore cannot reach into another module's internals; Go's `internal`
directories and an architecture test enforce it.

Some use cases span modules. Applying through the careers page adds a
candidate, their CV and their application **in one transaction**: the
`careers` module declares what it needs (`FindOrCreateApplicant`, `AddFile`,
`AddApplied`, `PublishedAd`), each module offers that function taking the
caller's transaction, and `internal/app` adapts them (`adapters.go`).
Nothing is half-done if one step fails, and no module imports another.

**Adding a module** is one command (`task new:module`): it creates the contract
file, a migration with row-level security, the SQL, the Go code in this layout,
unit and integration tests, and the wiring. See
[playbooks/add-a-module.md](playbooks/add-a-module.md).

## What happens on a request

1. **Request ID.** Every request gets one (or keeps a valid incoming
   `X-Request-Id`). It is returned in the response header and in every error,
   is on every log line, is stored on audit entries, and is copied onto every
   job the request queues, in Go and in Python.
2. **Contract checks** (`platform/httpserver/contract.go`): the route must exist
   in the contract; the caller must be signed in and hold the operation's
   `x-permission`; parameters and body must match the schemas (unknown fields
   are rejected). Handlers can trust their input.
3. **The handler** converts contract types to domain values and calls the
   service.
4. **The service** opens one transaction with `database.InTx`, which sets the
   tenant, the user and their permissions for row-level security. It applies
   the domain rules, writes data, queues jobs and writes an audit entry, all
   committed together or not at all.
5. **Errors** are `apperr` errors with a stable code from the contract (for
   example `CONFLICT_STALE_VERSION`). Anything unexpected becomes a 500 with the
   request ID, and is logged with its cause.

## Data safety

- **Company isolation.** Every row carries `tenant_id`, and PostgreSQL
  row-level security only shows the current company's rows. If code forgets a
  filter, the database still refuses. A test fails if any new table lacks it.
- **Sensitive fields.** Salary details live in their own table with their own
  policy: rows are invisible unless the transaction carries
  `candidates.compensation:read`. The API also says "restricted" rather than
  "none recorded". See [access-control.md](access-control.md).
- **No lost updates.** Editable records have a `version`. A save based on an
  old version gets `409 CONFLICT_STALE_VERSION` instead of silently overwriting
  someone else's change.
- **Least privilege.** The API connects as a restricted role and refuses to
  start as a role that could bypass isolation. The Python worker's role can
  only call the job-queue functions; it cannot read candidate tables. Workers
  download files through short-lived signed links, never with storage keys.
- **Audit trail.** Every change records who, what, when and the request ID.
  Salary changes record only that they changed, never the amounts.

## Background jobs

The job queue is a PostgreSQL table with four functions, written once and
called identically from Go and Python: `job_enqueue`, `job_claim`,
`job_complete`, `job_fail`.

- **Queued in the same transaction** as the data change, so a job can never be
  lost or point at data that was rolled back.
- **Claimed with a lease.** If a worker dies, the lease expires and another
  worker takes over; the first worker's late result is discarded.
- **Retried** with growing waits (10s, 20s, 40s, up to 1 hour), then marked
  `dead` with its last error, so failures are visible, never silent.
- **Declared in the contract** (`x-jobs`: queue and payload schema). Payloads
  are checked against the schema when queued (Go) and when read (Python).
  The Go worker refuses to start if a job on its queue has no handler, and
  tests fail for either worker if one is missing.

## Observability

- **Logs** are JSON with the same field names in Go and Python: `request_id`,
  `tenant_id`, `user_id`, `job_id`, `job_kind`. One search follows a click
  through the API, the Python worker and the Go worker.
- **Health:** `/health/live` (process up) and `/health/ready` (database reachable).
- **Startup checks:** the API refuses to start with invalid settings (all
  problems listed at once), a privileged database role, or a database behind
  the code's migrations.
- **Next:** OpenTelemetry traces and error alerts (Sentry), when it runs in the
  cloud.

## Scaling

All state is in PostgreSQL and file storage, so the API and workers are
stateless and scale by running more copies:

1. **One machine (today):** Docker Compose runs PostgreSQL, the API, both
   workers and the web app.
2. **Growing:** more API and worker copies behind a load balancer, connection
   pooling (PgBouncer), a read replica for reports, files in S3.
3. **Large:** a dedicated search engine fed by jobs, partitioned activity and
   audit tables, and, only if ever needed, a busy module moved into its own
   service. Its contract file and job interface are already its boundary.

## Repository map

| Path | What |
|---|---|
| `contracts/` | API and job contract (one file per module) |
| `backend/cmd/` | `api`, `worker`, `hp` (admin), `contracts` (merge), `scaffold` (new module) |
| `backend/internal/platform/` | config, errors, auth, database, job queue, files, HTTP, logs, audit |
| `backend/internal/modules/` | the business modules |
| `backend/internal/app/` | wiring and the integration tests |
| `backend/db/` | migrations and SQL queries |
| `ai/` | Python AI worker |
| `web/` | React web app |
| `deploy/` | Dockerfiles and Docker Compose |
| `docs/` | this documentation |

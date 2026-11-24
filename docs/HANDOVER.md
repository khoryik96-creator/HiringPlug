# Handover (22 Nov 2026, version 1.37.0)

For the next developer or AI helper picking up HiringPlug, a recruitment
agency system for recruiters in Malaysia. Read in this order:

1. [AGENTS.md](../AGENTS.md): the rules (most enforced by `task verify`).
2. [README.md](README.md): the docs' reading order (architecture first).
3. [WORKING-ON.md](WORKING-ON.md): who is changing what right now.
4. [features.md](features.md): what every screen does (long; search it).
   The app's own **Help** (the ? in the top bar, `web/src/help/`) is the
   short, plain-words version for users.

## What is built

A modular monolith: a Go API and Go worker (`backend/`), a Python AI worker
(`ai/`), a React web app (`web/`), PostgreSQL for data, search and the job
queue. Every module's API is a contract in `contracts/modules/*.yaml`.

- **Recruiting core**: candidates (CVs read with OCR and the AI, formatted
  and blind CVs in CV Studio's way, compare, duplicates and merge), companies
  and contacts (website auto-fill, email verifier, LinkedIn checks, Find HR),
  jobs from the client's JD, the pipeline (Steps and Board), interviews,
  booking links, offers with approval, placements with fees, retainers and
  invoices emailed on the start date, careers page and job board links.
- **Search**: quick search (Kano's /find rules), boolean and advanced search
  over CVs and profiles, AI search against a JD, possible candidates per job.
- **Communication**: email from a connected Microsoft, Google or SMTP mailbox,
  templates with fields, one email to many, client emails about several
  candidates, Outlook Mail and Calendar inside the app, WhatsApp links with
  templates (1.30).
- **Running an agency**: roles and access per company (enforced by the
  database, salary its own permission), teams, two-step sign-in (1.29),
  automations, expense claims with receipt OCR and cloud storage, leads from
  job ads, reports, market map, interview insights, PDPA tools, Bin, backups.
- **Recent versions**: phones (1.31), merging duplicate companies and
  contacts (1.32), notifications: the bell and the morning email (1.33),
  exports to Excel/CSV from every list and report and Excel import (1.34),
  in-app help and a getting-started checklist on the dashboard (1.37).
  1.35 (client shortlist link) and 1.36 (activity targets and revenue
  forecast) were being built alongside 1.37; see WORKING-ON.md and the
  roadmap for whether they are merged.
- The JobAdder migration tool (`tools/jobadder-migrator`) is separate and
  discovery-only so far: see [jobadder-migration.md](jobadder-migration.md).
- Deeper notes per area: [calendar.md](calendar.md) (Outlook calendar,
  private holds), [mailbox-connections.md](mailbox-connections.md) (Microsoft,
  Google, SMTP), [industries.md](industries.md), [claims-storage.md](claims-storage.md),
  [apollo-hr.md](apollo-hr.md), [access-control.md](access-control.md).

## How to run it

- On your own machine: [run-locally.md](playbooks/run-locally.md); on a server:
  [deploy-to-a-server.md](playbooks/deploy-to-a-server.md).
- Developing: `task setup` once (only for missing dependencies), `task db:up`,
  then `task dev` (or `go run ./cmd/api` in `backend/` plus `pnpm dev` in
  `web/`). `hp migrate` then `hp dev-setup` (in `backend/`, `go run ./cmd/hp`)
  prints demo users and tokens (owner, recruiter, sourcer, viewer; password
  `demo-password-2026`). `hp tenant -name … -owner-email … -owner-name …`
  makes a fresh, empty company (to see the getting-started checklist) and
  `hp token -user usr_…` a sign-in token for it.
- Several helpers on one machine: give each its own database
  (`/hiringplug_<name>` in the three `HP_DATABASE*`/`HP_AI_DATABASE_URL`
  settings of its own `.env`), its own API port (`HP_HTTP_ADDR`) and point
  the web dev server at it with `HP_DEV_API=http://localhost:<port>`.
- Screenshots (AGENTS rule 12): Playwright (pre-installed), set
  `sessionStorage['hiringplug.token']` to a token and reload; check desktop
  and 390px phone width.
- Checks: targeted tests while working (`go test ./internal/app/ -run …`,
  `npx vitest run <files>` in `web/`), then `task verify` (about 10 minutes;
  run it in the background with the dev stack stopped). `task gen` after
  editing `contracts/` or `backend/db/`.

## Where things are

| What | Where |
|---|---|
| A module's API, permissions, errors, jobs | `contracts/modules/<module>.yaml` |
| Database changes (append-only) | `backend/db/migrations/`; numbers are taken in ranges when helpers work in parallel (00170–00199 were given out on 22 Nov) |
| A module's SQL / Go | `backend/db/queries/<module>.sql`, `backend/internal/modules/<module>/` |
| Wiring (the only place modules meet) | `backend/internal/app/` (integration tests live here too) |
| AI worker | `ai/src/hiringplug_ai/` (CV Studio's engine vendored in `ai/cvstudio`) |
| Web pages, routes, menus | `web/src/pages/`, `web/src/routes.tsx` (one table drives the router, top bar and Settings menu) |
| In-app help topics, getting-started steps | `web/src/help/` (typed data; update a topic when its screen changes) |
| Each person's settings | `GET/PUT /v1/me/preferences` (identity module) |

## How the owner works with us

- The owner is a recruiter, not a developer: write to them in plain words.
- The agency: 20 to 30 consultants, each with their own vertical (software,
  data, DevOps, security, sales, marketing, finance, operations,
  compliance...). Search by meaning must read each vertical's titles, tools
  and qualifications in the right field (`ai/tests/test_meaning_verticals.py`
  lists the typical ones; add to it when a vertical's titles are misread).
- Build on a branch and push it; **main moves only when the owner says
  "merge"** (fast-forward, no pull requests unless asked). Raise `VERSION`
  with every update (minor for features, patch for fixes).
- Several helpers work at once and see each other only through GitHub: keep
  your WORKING-ON.md entry current and push often.
- Every screen change: a screenshot in the real app, shown when reporting.
  Every bug fix: a test that fails without it.
- They like to **see options before a screen is redesigned**: a small HTML
  mock-up with two or three choices, then they pick.
- They name tools to copy: **CV Studio** (their CV formatter, vendored) and
  **Kano** (their browser tool; only its /find code was shared). If asked to
  copy something you cannot see, say so and ask for it.
- No model names in commits.

## Must never happen

- **Real CVs are never committed.** Tests use invented people and CV
  Studio's synthetic example (`ai/cvstudio/examples`).
- **Salary** never goes into audit entries, logs, AI prompts or help text
  examples; salary in emails only per the rules in features.md.
- No secrets in code; settings are `HP_*` variables listed in `.env.example`.

## What is next (roadmap "Product pipeline")

1. Client shortlist link (7) and activity targets and revenue forecast (8),
   if not merged yet.
2. Search by meaning (9): done in 1.40.0 (docs/ai-worker.md, "Search by meaning").
3. Housekeeping (10): help, checklist and this handover are done; **tidying
   old branches (the Leads workspace merged or closed) is left to the owner**.
4. Then: batch CV formatting, Find HR for unlinked leads, Gmail replies and
   bounces, AI usage and cost per month, an audit-log page and "who viewed
   salary", single sign-on, cloud readiness (files in object storage, sign-in
   limits in PostgreSQL, key rotation, alerts). Waiting on the owner: Google
   calendar sync and job board posting (partner accounts), live Microsoft
   mailbox and calendar validation.

Known open ends: the top bar still scrolls its page links at 1280px wide
(it fits from 1366px); in one-email-to-many every copy's attachment points at
the same stored file, so anything that deletes email files must keep a file
another email still uses.

## Things that cost time before

- Stop the dev stack before `task verify`: on a busy machine the AI-queue
  tests time out. `hp migrate` runs the migrations built into the binary, so
  rebuild `hp` (or `go run`) after adding one.
- `task gen` rewrites every module's `internal/store/models.go` when a table
  or column changes: expected, commit them.
- In contract YAML, a `description:` inside `{ ... }` with a comma or colon
  must be quoted.
- Two files whose names differ only in case are refused by a test; files
  stay under 500 lines.
- A new permission needs a migration adding it to the roles of companies
  that chose their own permissions (see `00079_job_documents_delete.sql`).
- The test server takes uploads of at most 256 KB: test big-file limits as
  unit tests. The dev stack has no mail server ("failed: no mail server is
  set up" is expected).
- AI features without a real key: a small local OpenAI-compatible stand-in,
  `HP_AI_PROVIDER=openai_compatible`, `HP_AI_PROVIDER_URL=http://127.0.0.1:<port>/v1`;
  never present its output as real AI quality.
- Headless Chromium does not draw PDFs in an iframe: render the page with
  `pdftoppm` for screenshots, and say so. Docker builds cannot reach package
  registries from the sandbox; rely on CI's Docker job.

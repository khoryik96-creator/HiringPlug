# Handover (10 Nov 2026)

For the next person or AI agent picking up HiringPlug. Read `AGENTS.md`
first (the rules, enforced by `task verify`), then `docs/README.md`;
`docs/features.md` says what every screen does.

## Where things stand

- **main = `455f021`**. The salary feature and bug fixes are merged; all jobs
  passed in main CI run `42802569118`, including service and browser tests.
- Nothing uncommitted, nothing half-built. Every `claude/*` branch is
  already in main; start new work on a fresh branch from main.
- Latest migration: `00081_salary_calculation.sql`. Migrations are append-only.

## How the owner works with us

- Build on the branch, push it; **fast-forward main only when the owner says
  "merge"**. No pull requests unless asked.
- Before merging: `task verify`; the new-module check (in a throwaway
  worktree: `task new:module -- name=ci_samples entity=ci_sample`,
  `task gen:contract gen:go`, `go test ./internal/app/ ./internal/modules/... ./internal/devtools/...`,
  `task lint:go`); the end-to-end run (API + both workers + `ai/tests/e2e`)
  and the browser smoke tests (`web/e2e`). CI runs all of these too, so a
  green CI run on the exact commit counts.
- After merging, check GitHub CI on main a few minutes later. Re-run jobs
  that infrastructure cancelled (an apt timeout happened once); reproduce and
  fix real failures on the branch.
- Every screen change: screenshot it in the real app (AGENTS rule 12) and
  show it. Every bug fix: a test that fails without it (rule 11).
- No model names in commits. Write to the owner in plain words; they are a
  recruiter, not a developer.
- The owner likes to **see options before a screen is redesigned**: a small
  HTML mock-up with two or three choices, then they pick ("1.a and 2.C").
- They name other tools to copy: **CV Studio** (their CV formatter; its
  engine is vendored in `ai/cvstudio`) and **Kano** (their browser tool; only
  its /find code was ever shared, as an upload, not in the repo). If asked to
  copy something from them that you cannot see, say so and ask for it.
- **Deleting an uploaded document is admin-only**, as a permission of its own
  per kind of document (`companies.documents:delete`,
  `jobs.documents:delete`): the button is always shown, disabled for others.
  A new permission needs a migration adding it to admins who chose their own
  permissions (see `00079_job_documents_delete.sql`).

## Must never happen

- **Real CVs are never committed.** The owner uploaded real people's CVs for
  testing (in earlier sessions' uploads); use them only locally. Tests use
  invented people and CV Studio's synthetic example (`ai/cvstudio/examples`).
- **Salary** never goes into audit entries, logs or AI prompts. Salary in
  client emails only per the rules in `docs/features.md` (only senders who may
  see salary fill those fields; such emails hidden from roles without salary
  access unless Settings says otherwise). CV Studio formatting removes lines
  stating the candidate's own pay before the AI sees the CV.
- No secrets in code; settings are `HP_*` variables listed in `.env.example`.

## What was built 7–10 Nov (newest first; details in `docs/features.md`)

- **Screening salary**: descriptive current and expected pay, fixed monthly
  totals, percentage expectations and configurable component/bonus rules in
  Settings. Variable pay stays excluded by default. Ambiguous amounts require
  review; only the fixed monthly figure appears on candidate cards.
- **Email**: one email to many people, each their own copy (`POST
  /v1/email-batches`, tick boxes on the candidate and contact lists and the
  pipeline); one email to the client about several candidates, each with
  their own details as a block or a table row and their own profile attached
  (`candidates` on `/v1/emails`, table `email_candidate`); attachments up to
  20 MB in all; contacts emailed from their page; templates with a formatting
  toolbar and field picker (only admins change them; anyone saves a new one).
- **AI assistant** can be docked to the right of the screen (remembered in
  the browser, `web/src/components/useDock.ts`); answers are cards.
- **Job page**: pipeline first with possible candidates folded under it,
  notes next; uploaded JDs kept with the job (view, download, admin delete);
  job ads as foldable cards.
- **Candidates**: duplicate CV uploads caught (same file skipped, same name
  asks); screening calls with who and when (salary follows the latest);
  notes timeline with the latest call pinned, filters and search; list with
  your own columns and work authorization as a coloured initials disc;
  advanced search (dropdowns and a boolean on CVs) and AI search (CV Studio's
  AI Crawler); home search by Kano's /find rules.
- **Companies**: categories with colours (Settings) and a card; LinkedIn
  page; benefits and pre-offer process; documents; terms of business as
  rates by salary band.
- **Placements**: invoicing (terms, recipient, due date, invoice emailed on
  the start date), invoice preview in the app.
- **CV Studio's way**: blind CVs, blind job ads rewritten by the AI,
  formatted CVs (Word and PDF, "About him/her", agency file names, delete).

## Open ends and ideas (none started)

- The assistant's dock is a standard side panel: the owner compared it with
  Kano's; if they share how Kano docks, match it.
- In one-email-to-many, every copy's attachment row points at the **same
  stored file** (`storage_key`). Anything that later deletes email files
  must keep a file while another email still uses it.
- Proposed before and still open: batch CV formatting; possible candidates
  by meaning (embeddings); cloud readiness (files in S3, sign-in limits in
  PostgreSQL, key rotation, alerts). Waiting on the owner: Google/Microsoft
  calendar sync (OAuth apps), posting ads to job boards (partner accounts),
  a test of CV Studio formatting with their real AI key.

## Things that cost time before

- Reuse the existing environment and run targeted checks during edits; finish
  with `task verify` before pushing. See
  [development-workflow.md](playbooks/development-workflow.md) for the measured
  CI mirror delay, bounded installer and verification sequence.
- Check database health if tests cannot connect; start the existing service
  only if stopped (a retained cloud session may already have Docker PostgreSQL).
- Stop the dev stack (api, workers, `pnpm dev`) before `task verify`: on a
  busy machine the AI-queue tests time out and workers are killed for memory.
  Run `task verify` in the background; it takes about 10 minutes.
- `hp migrate` runs the migrations built into the binary: rebuild `hp`
  after adding one.
- `task gen` regenerates every module's `internal/store/models.go` when a
  table is added: expected, commit them.
- In contract YAML, a `description:` inside `{ ... }` with a comma or colon
  must be quoted, or the contract stops being valid OpenAPI.
- Two files whose names differ only in case (`shortlist.ts`,
  `Shortlist.tsx`) are refused by a test.
- The test server takes uploads of at most 256 KB (`testMaxUpload`): test
  big-file limits as unit tests.
- In the dev stack there is no mail server: sent emails show "failed: no
  mail server is set up". That is expected; the integration tests have one.

## Working in a cloud session

- Inspect installed tools and database health first. Reuse the workspace's
  existing environment activation script and caches when available.
- Run `task setup` only for missing dependencies or changed lockfiles (Go,
  Python with `uv`, web with `pnpm`, and `npm ci` for CV Studio's renderer).
  Run targeted checks during edits, then `task verify` before pushing.
- To run the real app: build `backend/cmd/{api,worker,hp}`, set the `HP_*`
  variables from `.env.example` against a local database, `hp dev-setup`
  prints demo users and tokens (owner, recruiter, sourcer, viewer; password
  `demo-password-2026`), start `api`, `worker`, `uv run hiringplug-ai` (in
  `ai/`) and `pnpm dev` (in `web/`, port 5173). For screenshots, Playwright:
  set `sessionStorage['hiringplug.token']` and reload.
- To see AI features without a real key: run a small local OpenAI-compatible
  stand-in and set `HP_AI_PROVIDER=openai_compatible`,
  `HP_AI_PROVIDER_URL=http://127.0.0.1:<port>/v1`, `HP_AI_MODEL=stand-in`,
  then choose "Server default" in Settings > AI provider. Never present
  stand-in output as real AI quality.
- Headless Chromium does not draw PDFs inside an iframe: for screenshots of
  CV previews, render the PDF page with `pdftoppm` and say so.
- Docker builds cannot reach package registries from the sandbox's proxy;
  test images' pieces directly (e.g. nginx with the config mounted), and rely
  on CI's Docker job for full builds.

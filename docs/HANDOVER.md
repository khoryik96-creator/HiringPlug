# Handover (7 Nov 2026)

For the next person or AI agent picking up HiringPlug. Read `AGENTS.md`
first (the rules, enforced by `task verify`), then `docs/README.md`.

## Where things stand

- **main = `a7a1e7c`**, CI green up to `1b0106e`; the check on `a7a1e7c`
  (CV Studio formatting) was still running at handover: look at the latest
  CI run on main first.
- Working branch: `claude/brave-thompson-k08hlg` (equal to main). Nothing
  uncommitted, nothing half-built.
- Latest migration: `00056_studio_cv.sql`. Migrations are append-only.

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

## Must never happen

- **Real CVs are never committed.** The owner uploaded real people's CVs for
  testing (in this session's uploads); use them only locally. Tests use
  invented people and CV Studio's synthetic example (`ai/cvstudio/examples`).
- **Salary** never goes into audit entries, logs or AI prompts. Salary in
  client emails only per the rules in `docs/features.md` (only senders who may
  see salary fill those fields; such emails hidden from roles without salary
  access unless Settings says otherwise). CV Studio formatting removes lines
  stating the candidate's own pay before the AI sees the CV.
- No secrets in code; settings are `HP_*` variables listed in `.env.example`.

## What was built most recently (newest first)

1. **Formatted CVs CV Studio's way** (`a7a1e7c`): "Make formatted CV" → "From
   their CV". CV Studio's engine is vendored **unchanged** in `ai/cvstudio`
   (rules, prompts, Node renderer `generate.js`, Hyppies template); only
   `ai/src/hiringplug_ai/cvstudio.py` touches it. Job `cv.format_studio`
   (AI worker) → `cv.studio_formatted` (Go, cvformats module, `studio.go`).
   Design in `docs/ai-worker.md` ("CV Studio's way"). **Only tested with a
   stand-in AI** (CV Studio's synthetic parse); the owner was asked to try it
   with a real DeepSeek/Claude key on a real CV.
   LibreOffice cannot open a .docx ending on a framed paragraph (CV Studio's
   closing terms), so `render_pdf.py` retries with an empty paragraph after it.
2. **Architecture fixes** (`a3dce91`…`1b0106e`): finished jobs keep no payload
   and are pruned after 30 days; job priorities in the contract (`priority:
   high|normal|low`) and several jobs at once per worker; `hp backup` /
   `hp restore` with a nightly Docker `backup` service (tools must match the
   database's major version, see `platform/backup/tools.go`; CI runs
   PostgreSQL 16's tools from the postgres:16 image because the runner's
   Ubuntu 26.04 has no postgresql-client-16); Settings > System (background
   jobs, Try again, last backup); one route table for the web app
   (`web/src/routes.tsx`, pages lazy-loaded); security headers
   (`deploy/security-headers.conf`, shared by nginx and the browser tests);
   sign-out ends the session on the server; Playwright smoke tests in CI.
3. One **Settings** button for everything; CV preview on the candidate page
   (Word CVs get a PDF copy); blind job ads catch the JD's own spelling of the
   client's name.
4. Earlier this week: AI provider settings (DeepSeek / Claude / ChatGPT keys
   per company), contacts' LinkedIn employment check, booking links (a small
   Calendly for Gmail/Outlook), review fixes.

## What is next (proposed to the owner, not started)

Can be built now:

1. **CV Studio's summary bullets**: generate the "About him/her" summary
   from the CV (`ai/cvstudio/prompts/SUMMARY_NORMAL.txt`, `SUMMARY_BLIND.txt`,
   `candidate-summary.js`; pay filtered), and insert a chosen summary into an
   existing Word CV without regenerating it (`ADAPTERS_AND_MIGRATION.md`,
   "generate-docx multipart"). Proposed as the next step.
2. **Batch formatting**: several CVs at once (the queue already runs jobs in
   parallel; mostly a screen).
3. **Possible candidates by meaning** (embeddings) instead of word search.
4. **Cloud readiness** (only needed with more than one server): files in S3
   (`platform/blob` is ready for it), sign-in rate limits in PostgreSQL
   instead of memory, rotating `HP_AI_KEYS_SECRET` (sealed keys carry a `v1.`
   prefix for this), error alerts and tracing.

Waiting on the owner:

5. Their real-key test of CV Studio formatting.
6. Google / Microsoft calendar sync for interviews and replies back into the
   app: needs registered OAuth apps and their keys.
7. Posting ads directly to JobStreet / LinkedIn / Indeed: needs partner
   accounts.
8. The "FIFA card" candidate intro: the owner will share more; do it last.

## Working in a cloud session

- `service postgresql start`; `export PATH=$PATH:/root/go/bin`.
- `task setup` (Go, Python with `uv`, web with `pnpm`, and `npm ci` for CV
  Studio's renderer); `task verify` takes about 8 minutes.
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

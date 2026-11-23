# Working on HiringPlug (for people and AI agents)

Read this before changing code. The rules exist so that adding or fixing one
thing never breaks another. Most are enforced by `task verify`; the rest are
checked in review.

## Commands

- `task verify` before every push. It must pass. Commit any files it regenerates.
- Raise `VERSION` (major.minor.patch, shown beside HiringPlug's name in the top
  bar) with every update pushed to main: the patch number for fixes, the minor
  for new features, so anyone can tell which update they are running.
- ShellCheck is required by `task lint:shell` (Debian/Ubuntu:
  `apt-get install shellcheck`; macOS: `brew install shellcheck`; Windows: use
  WSL). `task test:ci` runs CI-script tests with Python 3's standard library.
- Reuse installed dependencies and running databases. Run targeted tests while
  editing, then `task verify` once the change is ready to push. Repeat full
  verification only after a change or failure warrants it. See
  [the efficient development workflow](docs/playbooks/development-workflow.md).
- `task gen` after editing anything in `contracts/` or `backend/db/`.
- `task new:module -- name=<plural> entity=<singular>` to start a module.
- `task test` needs PostgreSQL (`task db:up`) and its client tools of the same version (pg_dump, for backups), rclone (off-site backups), Tesseract, antiword, LibreOffice (old .doc files) and Node (CV Studio's renderer; `task setup` installs its package).
  Tests never skip silently.

## Where things go

```
contracts/modules/<module>.yaml   the module's API, permissions, error codes and jobs (source of truth)
backend/db/migrations/            database changes (new files only)
backend/db/queries/<module>.sql   the module's SQL (sqlc turns it into Go)
backend/internal/modules/<module>/
    <module>.go    public API of the module (what internal/app and other modules may use)
    domain.go      pure rules: no database, no HTTP (unit tested)
    service.go     use cases: one transaction each; rules + storage + audit + visibility
    http.go        handlers: contract types <-> service; no business rules
    jobs.go        handlers for the module's jobs on the core queue (if any)
backend/internal/platform/   shared plumbing (config, errors, auth, database, jobs, logs)
backend/internal/app/        wiring: the only place modules meet
ai/src/hiringplug_ai/        the Python AI worker (jobs on the "ai" queue)
web/src/                     the React app
```

## Rules

1. **Contract first.** Change the API by editing `contracts/modules/*.yaml`, then
   `task gen`. Never edit generated files (`openapi.yaml`, `api.gen.go`,
   `internal/store`, `contracts.py`, `schema.d.ts`).
2. **Modules do not import each other.** If module A needs module B, define a
   small interface in A and pass B in from `internal/app/modules.go`.
   (`TestModulesOnlyMeetInTheAppPackage`)
3. **A module owns its tables.** Only its own `db/queries/<module>.sql` touches
   them. Other modules go through its public functions.
4. **Every operation declares `x-permission`.** Sensitive data gets its own
   permission (see `candidates.compensation:read`) and its own table with
   row-level security, so the database enforces it too.
5. **Every table with tenant data has row-level security.**
   (`TestEveryTenantTableHasRowLevelSecurity`)
6. **Migrations are append-only.** Never edit or delete a committed migration;
   add a new one. (`task check:migrations`) Number it above the highest
   migration on `main` when it merges, never in a block reserved earlier: a
   server that already has a higher number refuses to apply a lower one and
   does not start.
7. **Use the service's transaction.** All database work goes through
   `database.InTx`, which scopes it to one tenant. Write an audit entry for
   every change. Never put salary values in audit entries, logs or AI prompts.
8. **Go and Python talk only through the job queue.** Declare the job in the
   module contract (`x-jobs`); queue it inside the same transaction as the data
   change (`jobqueue.Enqueue`).
9. **Errors:** return `apperr` errors with a contract `ErrorCode`; anything else
   becomes a 500 and is logged. Add new codes to the module contract.
10. **Small files.** At most 500 lines (`TestFilesStaySmall`); split by concern.
11. **Tests:** every change has tests; every bug fix includes a test that fails
    without the fix. Prefer integration tests through the real HTTP API and real
    PostgreSQL (`internal/app/*_test.go`) over mocks. Tests live next to the
    feature, never in version-named files.
12. **Look at what you built.** Every change to a screen is checked in the real
    app before it is called done: run it (`task dev`, or the API plus `pnpm dev`)
    with realistic data, take a screenshot of each changed screen (Playwright;
    the browser is pre-installed), and compare it with the approved design or
    mockup and with what was asked. Fix what differs, and show the screenshot
    when reporting. Tests check behaviour; only a screenshot shows the result.
13. **No secrets in code or logs.** Settings come from `HP_*` variables, all
    listed in `.env.example` (checked by a test).

## Working alongside other helpers

Several AI helpers work on HiringPlug at once, on different accounts, and
cannot see each other except through GitHub. So:

- **Before starting**, `git fetch` and read
  [docs/WORKING-ON.md](docs/WORKING-ON.md) on `origin/main`, and glance at the
  newest branches (`git branch -r --sort=-committerdate | head`). If your task
  touches what an entry lists, tell the owner and wait, instead of editing the
  same module or screen.
- **Add your entry** to `docs/WORKING-ON.md` as a commit to `main` that changes
  only that file (rebase on the newest `main` first; never force-push). If you
  cannot push to `main`, put it in your branch's first commit and tell the
  owner.
- **Push your branch often**, not only at the end: work that is not pushed
  cannot be picked up by anyone else.
- **Keep the entry current.** When you stop for any reason (a usage limit, the
  owner's call, a blocker), push, then set *State* and write *Next* so another
  helper can finish the job.
- **Taking over:** read the entry, check out its branch, read its recent
  commits, finish it under these same rules, and change *Who* to yourself.
- **When it is merged**, remove the entry (a commit changing only that file).

## Definition of done

`task verify` passes; the change has tests; changed screens were checked in a
screenshot against their design (rule 12); docs in `docs/` are updated if the
architecture, a module's purpose or a workflow changed; the PR checklist is
filled in; your `docs/WORKING-ON.md` entry is removed.

More: [docs/README.md](docs/README.md) (reading order) and
[docs/playbooks/](docs/playbooks) (step-by-step guides).

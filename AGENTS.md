# Working on HiringPlug (for people and AI agents)

Read this before changing code. The rules exist so that adding or fixing one
thing never breaks another. Most are enforced by `task verify`; the rest are
checked in review.

## Commands

- `task verify` before every push. It must pass. Commit any files it regenerates.
- `task gen` after editing anything in `contracts/` or `backend/db/`.
- `task new:module -- name=<plural> entity=<singular>` to start a module.
- `task test` needs PostgreSQL (`task db:up`), Tesseract, antiword and LibreOffice (old .doc files).
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
   add a new one. (`task check:migrations`)
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
12. **No secrets in code or logs.** Settings come from `HP_*` variables, all
    listed in `.env.example` (checked by a test).

## Definition of done

`task verify` passes; the change has tests; docs in `docs/` are updated if the
architecture, a module's purpose or a workflow changed; the PR checklist is
filled in.

More: [docs/README.md](docs/README.md) (reading order) and
[docs/playbooks/](docs/playbooks) (step-by-step guides).

# Guardrails: why changes don't break things

The principle: **every rule is enforced by the strongest tool available.** In
order of preference: the type checker → the database → an automatic check on
every change → a test → a startup check → review → documentation. A rule that
only lives in documentation eventually gets broken, so very few do.

`task verify` runs every check locally; CI runs the same checks on every push
and pull request (`.github/workflows/ci.yml`).

## What stops each kind of regression

| Risk | What stops it | Where |
|---|---|---|
| Web app, Go and Python disagree on a field | All three are generated from one contract; a change breaks compilation where the field is used. CI fails if generated code is stale. | `task gen`, `check:gen` |
| Two modules define the same endpoint, data shape, permission, error code or job | The contract merge refuses, naming both modules | `contractbundle` + its tests |
| An endpoint without a permission | The merge refuses; the HTTP layer would also refuse to serve it | `x-permission` checks |
| An endpoint in the contract with no handler | Go does not compile (`api.StrictServerInterface`) | `internal/app/modules.go` |
| A job with no worker to run it | The Go worker refuses to start; tests fail for both workers | `TestEveryCoreJobHasAHandler`, `test_every_ai_queue_job_has_a_python_handler` |
| Bad input reaching the code | Every request is checked against the contract (types, formats, lengths, unknown fields) before handlers run | `httpserver/contract.go` |
| Bad data in the database | Constraints mirror the rules (checks, foreign keys including the tenant, unique email per company) | migrations |
| Company A sees company B's data | Row-level security in PostgreSQL; a test fails if any tenant table lacks it; another proves raw SQL cannot cross over | `TestEveryTenantTableHasRowLevelSecurity`, `TestTheDatabaseEnforcesIsolationEvenForRawSQL` |
| Salary shown to the wrong role | Permission check in the service **and** a database policy on the salary table | `TestSalaryIsOnlyVisibleWithPermission` |
| The app running as a database superuser (bypassing isolation) | The API and worker refuse to start | `database.CheckRestrictedRole` |
| Two people editing the same record | Version numbers: the second save gets `409 CONFLICT_STALE_VERSION` | `TestUpdateCandidate` |
| A lost or duplicated background job | Jobs are queued in the same transaction as the data; leases, retries, dead jobs | `jobqueue` tests |
| A job payload in the wrong shape | Checked against the contract when queued (Go) and read (Python) | `TestPayloadsAreCheckedAgainstTheContract` |
| Modules tangling together | Modules may not import each other; platform may not import modules; domain rules may not touch the database or HTTP | `architecture_test.go` |
| Giant files that nobody can safely change | At most 500 lines per Go file, 300 per web file | `TestFilesStaySmall`, ESLint `max-lines` |
| A new enum value not handled everywhere | Exhaustiveness checks (Go `exhaustive`, TS `switch-exhaustiveness-check`) | linters |
| Forgotten `await` / unhandled errors | `errcheck`, `errorlint` (Go); strict TypeScript ESLint rules; mypy strict (Python) | linters |
| An edited old migration | CI compares migrations with `main` | `task check:migrations` |
| Running against an out-of-date database | The API refuses to start if migrations are pending | `database.PendingMigrations` |
| A setting nobody knows about | Every `HP_*` variable must be in `.env.example` | `TestEveryVariableIsInEnvExample` |
| Wrong or missing settings in production | Startup lists every problem and stops; production refuses development secrets | `config.Load` |
| Secrets or salaries in logs | Salaries are never logged or audited; signed download links are not logged | audit + logging code |
| A fixed bug coming back | Every fix ships with a test that fails without the fix (PR checklist) | review |
| A new module done differently | The generator creates it in the standard layout, already passing every check; CI generates one and runs everything | `task new:module`, CI scaffold step |
| Cross-language bugs only visible end to end | A real API + Go worker + Python worker test runs in CI | `task e2e`, CI `e2e` job |

## Debuggability built in

- **One request ID** follows a click everywhere: response header, error body,
  every log line (Go and Python), audit entries, and jobs.
- **Stable error codes** (`VALIDATION_FAILED` with the exact fields,
  `CONFLICT_STALE_VERSION`, ...): the web app and integrations react to codes,
  and people can search for them.
- **Failed jobs are visible:** each keeps its attempts and last error; `dead`
  jobs are listed for retry.
- **Audit log:** who changed what and when, per record.

See [playbooks/debug-a-problem.md](playbooks/debug-a-problem.md).

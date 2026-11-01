# Add a module

Example: a `notes` module (screening and interview notes).

## 1. Generate it

```bash
task new:module -- name=notes entity=note
task gen
task test
```

`name` is the plural (used in the URL, permissions and contract file) and
`entity` the singular (table and type names). Add `prefix=...` to choose the
ID prefix (2–8 letters; defaults to the entity name).

This creates, already wired and passing every check:

| File | Contains |
|---|---|
| `contracts/modules/notes.yaml` | `GET/POST /v1/notes`, schemas, permissions `notes:read` / `notes:create` |
| `backend/db/migrations/000NN_notes.sql` | the `note` table with row-level security |
| `backend/db/queries/notes.sql` | insert and list queries |
| `backend/internal/modules/notes/` | `notes.go`, `domain.go` (+ test), `service.go`, `http.go` |
| `backend/internal/app/notes_test.go` | an integration test: create, list, permissions, company isolation |
| `backend/sqlc.yaml`, `backend/internal/app/modules.go` | the wiring |

## 2. Shape it to your needs, in this order

1. **Contract** (`contracts/modules/notes.yaml`): fields, endpoints,
   permissions (with default roles), error codes, jobs. Run `task gen`. The Go
   compiler now lists the handlers you must write or change.
2. **Migration**: edit the generated file *until it is merged to main*; after
   that, only add new migration files. Keep the row-level security lines.
3. **Queries** (`backend/db/queries/notes.sql`), then `task gen`.
4. **Domain** (`domain.go`): the rules as pure functions, with unit tests.
5. **Service** (`service.go`): one transaction per use case
   (`database.InTx`), rules + queries + `audit.Record`.
6. **Handlers** (`http.go`): convert contract ↔ domain types; no rules here.
7. **Tests**: extend `internal/app/notes_test.go` through the HTTP API: success,
   validation errors, permissions per role, company isolation.

## 3. Needs another module?

Don't import it. Declare the small interface you need in your module (see
`documents.Candidates`) and pass the other module in, in
`internal/app/modules.go`.

## 4. Finish

`task verify`, then open a pull request (the template has a checklist).

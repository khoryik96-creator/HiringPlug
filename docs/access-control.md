# Access control

## Who is calling

Every API call carries `Authorization: Bearer <token>`. The token names the
company (tenant) and the user. The user is looked up on every request, so
deactivating a user signs them out at once. Development uses tokens signed with
a shared secret (`hp dev-setup`, `hp token`). Production will use single
sign-on (Google, Microsoft) through an OpenID Connect provider; only the token
check changes.

## What they may do

**Roles** hold **permissions**. Each module declares its permissions, and which
roles get them by default, in its contract file. That file is the only list:
Go reads it at startup, and the web app gets the user's permissions from
`GET /v1/me` to show or hide buttons.

Your "view / modify / write" maps to `:read` / `:update` / `:create`. Sensitive
details get their own permissions.

| Permission | owner | admin | recruiter | sourcer | hiring manager | viewer |
|---|---|---|---|---|---|---|
| `candidates:read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `candidates:create` | ✓ | ✓ | ✓ | ✓ | | |
| `candidates:update` | ✓ | ✓ | ✓ | ✓ | | |
| `candidates:merge` (duplicates) | ✓ | ✓ | ✓ | | | |
| `candidates.compensation:read` (salary) | ✓ | ✓ | ✓ | | | |
| `candidates.compensation:update` (salary) | ✓ | ✓ | ✓ | | | |
| `documents:read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `documents:create` | ✓ | ✓ | ✓ | ✓ | | |
| `companies:read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `companies:create`, `companies:update` | ✓ | ✓ | ✓ | ✓ | | |
| `contacts:read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `contacts:create`, `contacts:update` | ✓ | ✓ | ✓ | ✓ | | |
| `jobs:read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `jobs:create`, `jobs:update` | ✓ | ✓ | ✓ | | | |
| `jobs.salary:read` (budget) | ✓ | ✓ | ✓ | | ✓ | |
| `jobs.salary:update` (budget) | ✓ | ✓ | ✓ | | | |
| `pipeline:read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `pipeline:update` (add to jobs, move) | ✓ | ✓ | ✓ | ✓ | | |
| `notes:read`, `notes.activity:read` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `notes:create` (and delete own) | ✓ | ✓ | ✓ | ✓ | ✓ | |
| `notes.sensitive:read` (sensitive notes) | ✓ | ✓ | ✓ | | | |
| `notes:manage` (delete anyone's) | ✓ | ✓ | | | | |

Coming in M2: **custom roles** per company (an admin screen to pick
permissions), **data scope** (own / team / all candidates, important for agency
ownership), user invites, and a "who viewed salary" log.

## Three layers of enforcement

Salary is the example; every sensitive field follows the same pattern.

1. **The endpoint.** Every operation declares `x-permission`. The HTTP layer
   checks it before the handler runs. Changing salary needs
   `candidates.compensation:update`.
2. **The service.** When reading a candidate, salary is only loaded for users
   with `candidates.compensation:read`. Otherwise the response leaves it out
   and says so: `"restrictedFields": ["compensation"]`. The web app then shows
   "🔒 restricted", and nobody mistakes "hidden" for "none recorded".
3. **The database.** Salary is in its own table, `candidate_compensation`, with
   row-level security that requires the permission in the transaction. Even a
   buggy query, or a new endpoint someone forgets to protect, gets no rows.
   `TestTheDatabaseEnforcesIsolationEvenForRawSQL` proves it with raw SQL.

![A sourcer sees that salary is restricted](images/candidate-sourcer.png)

The same permissions also bound what leaves the system. Audit entries record
that a salary changed but never the amounts, and the AI assistant (M4) only
receives the fields the user may see.

## Company isolation

Every table holding a company's data has `tenant_id` and a row-level security
policy, and the app connects as a role that cannot bypass it. Details and
guarantees: [architecture.md](architecture.md#data-safety).

## Adding a sensitive field

1. Store it in its own table (or a table only for sensitive fields) with a
   policy like `candidate_compensation`'s.
2. Declare `<module>.<field>:read` / `:update` permissions with
   `sensitive: true` in the module contract.
3. Load it in the service only when the user has the permission. Otherwise add
   it to `restrictedFields`.
4. Never log or audit its values; keep it out of AI prompts unless permitted.
5. Add tests for each role that may and may not see it, through the HTTP API.

# Debug a problem

## 1. Get the request ID

Every error the API returns includes `error.requestId`, and every response has
an `X-Request-Id` header. The web app shows it as "Reference: req_...". Ask for
it in every bug report.

## 2. Follow it through the logs

All services log JSON with the same field names. Search for the ID:

```bash
grep req_4ym3c5vw0mg1k3jdyq25zc15xy api.log worker.log ai.log
```

You will see the HTTP request (method, path, status, duration, company, user),
any rejection with its error code, and every job the request queued, with the
same `request_id` in the Python worker and the Go worker that handled them.

## 3. Check the record's history

```sql
SELECT created_at, actor_id, action, changes, request_id
FROM audit_log WHERE entity_id = 'cand_...' ORDER BY created_at;
```

## 4. Check background work

```sql
SELECT id, kind, status, attempts, max_attempts, last_error, run_after, request_id
FROM job WHERE status IN ('pending', 'running', 'dead') ORDER BY created_at DESC LIMIT 50;
```

`dead` jobs failed every attempt; `last_error` says why. After fixing the cause,
retry one with `UPDATE job SET status = 'pending', attempts = 0, run_after = now() WHERE id = '...'`.

## 5. A CV looks wrong or incomplete

Look at the document: `status`, `extractor` (which tools read it),
`extractionWarnings` (for example pages OCR could not read) and
`errorMessage`. Reproduce it with a generated document in `ai/tests/`.

## 6. The app will not start

It prints every configuration problem at once. Common ones: a missing
`HP_DATABASE_URL`; connecting as a superuser (use the restricted app role); the
database behind the code (run `hp migrate` or `hp bootstrap`).

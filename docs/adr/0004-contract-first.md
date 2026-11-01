# 0004 Contract first, one file per module

**Decision.** The API, permissions, error codes and job messages are written
first in OpenAPI, one file per module (`contracts/modules/`), merged and
checked by `task gen`, which then generates Go server types, the TypeScript
client and Python models.

**Why.** With three languages, a hand-maintained copy of each type would drift.
Generated types make a contract change show up as compile errors wherever it
matters. One file per module lets a new module be added without editing a
shared file, and the merge refuses collisions (duplicate paths, schemas,
permissions, error codes, jobs), so one module cannot silently override
another. The same contract drives request validation, the response checks in
tests, and the API documentation at `/docs`.

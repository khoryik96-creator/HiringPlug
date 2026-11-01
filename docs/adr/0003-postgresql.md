# 0003 PostgreSQL for data, jobs and (at first) search

**Decision.** PostgreSQL is the single stateful system: the data, the job queue
shared by Go and Python, the audit log, and search to start with (accent- and
case-insensitive search with trigram indexes).

**Why.** Recruitment data is relational (candidates, jobs, applications,
companies, contacts), and constraints and transactions protect it. One system
to run, back up and debug is right for a local-first product. Keeping the job
queue in the same database means a job is queued in the same transaction as
the data change, so it can never be lost or point at data that was rolled
back.

**When to add more.** A dedicated search engine (OpenSearch, Typesense or
Meilisearch) when search needs exceed PostgreSQL. It is fed by jobs, so
modules don't change. Redis or a message broker only if the PostgreSQL queue
becomes a measured bottleneck, which is far beyond ATS volumes.

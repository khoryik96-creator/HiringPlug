# 0005 Company and field security inside the database

**Decision.** Company (tenant) isolation and sensitive-field access (salary)
are enforced by PostgreSQL row-level security, not only by application code.
Each transaction carries the company, the user and their permissions.

**Why.** "Company A sees company B's candidates" and "a sourcer sees salaries"
are the most damaging bugs an ATS can have, and a single forgotten filter
causes them. With row-level security the database refuses even when the code is
wrong. The app connects as a restricted role and refuses to start as a role
that could bypass the policies, and tests prove the policies hold for raw SQL.

**Rules that follow.** Every table with company data gets a policy (a test
checks). Sensitive fields live in their own table with their own policy. The
Python worker's role can only use the job functions.

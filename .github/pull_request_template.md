## What and why

<!-- What does this change do for recruiters or for the system, and why? -->

## Checklist

- [ ] `task verify` passes locally (and any regenerated files are committed)
- [ ] Contract changes are additive, or breaking changes are intentional and called out above
- [ ] New or changed behaviour has tests; every bug fix includes a test that fails without the fix
- [ ] New database tables with tenant data have row-level security (a test checks this)
- [ ] New migrations only (committed migrations are never edited)
- [ ] Sensitive data (salary, identity documents) stays behind its permission, including in logs, audit entries and AI prompts
- [ ] Docs updated if the architecture, a module's purpose or a workflow changed

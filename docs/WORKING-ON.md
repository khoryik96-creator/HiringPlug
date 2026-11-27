# Working on now

Several AI helpers (Claude on two accounts, ChatGPT) change HiringPlug at the
same time. This page is how they keep out of each other's way and pick up each
other's work. It lists only work in progress; finished work is in `main` and
[HANDOVER.md](HANDOVER.md). The rules are in [AGENTS.md](../AGENTS.md)
("Working alongside other helpers").

How to write an entry (newest at the top, one per task):

```
### <short task name>
- Who: <Claude (account A) | Claude (account B) | ChatGPT | a person>, session or chat title
- Branch: <branch name>
- Touches: <modules, screens or files, e.g. contacts module, web/src/pages/Contacts*>
- Started: <date>    Last update: <date>
- State: <in progress | waiting for the owner | stopped (limit reached) | ready to merge>
- Next: <what is left, precise enough that another helper can finish it>
```

## In progress

### Post-migration fixes and the HyPlug rename (taken over)
- Who: Claude (account B), taken over from Claude (account A) on 26 Nov at the owner's request
- Branch: claude/brave-thompson-k08hlg
- Touches: nothing now; later every user-visible "HiringPlug" string (the rename)
- Started: 25 Nov 2026    Last update: 27 Nov 2026
- State: merged: 1.50.1 (held repeating jobs), 1.51.0 (P1), 1.52.0 (server checks), 1.53.0 (mail dock, header wrap),
  1.54.0 (Download all, view attachments, stay after Send), 1.55.0 (stale records: tag, filter, monthly email line).
  Waiting on the owner: the rename's go-ahead; the CV whose PDF reads without spaces ("DatabaseAdministrator").
- Next:
  1. Rename the product to HyPlug in everything people see (not formatted CVs or the agency's own branding; keep hidden
     technical names: Go module, database, HP_* settings, Python package, storage keys). Waits for the owner's go-ahead.
  2. With the owner's CV: PDF text read without word gaps (pdfplumber x_tolerance in ai/.../extraction/columns.py),
     re-checked against the 31 sample CVs.

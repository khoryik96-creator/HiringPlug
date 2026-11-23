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

### Client shortlist link (roadmap 7)
- Who: Claude (account A), session "Hiring Plug" (a helper in a separate worktree)
- Branch: merged into claude/relaxed-hopper-ex7y3v when done
- Touches: a new shortlist module (migrations 00170-00179), the job pipeline screen, a public client page; pipeline stage moves
- Started: 22 Nov 2026    Last update: 22 Nov 2026
- State: in progress
- Next: build, tests, screenshots; version 1.35.0

### Activity targets and revenue forecast (roadmap 8)
- Who: Claude (account A), session "Hiring Plug" (a helper in a separate worktree)
- Branch: merged into claude/relaxed-hopper-ex7y3v when done
- Touches: targets in Settings, the dashboard and Reports page, reports export (migrations 00180-00189)
- Started: 22 Nov 2026    Last update: 22 Nov 2026
- State: in progress
- Next: build, tests, screenshots; version 1.36.0

### In-app help and first-run guide (part of roadmap 10)
- Who: Claude (account A), session "Hiring Plug" (a helper in a separate worktree)
- Branch: merged into claude/relaxed-hopper-ex7y3v when done
- Touches: a help page, the top bar, a dashboard checklist, docs/HANDOVER.md (migrations 00190-00199 if any)
- Started: 22 Nov 2026    Last update: 22 Nov 2026
- State: in progress
- Next: build, tests, screenshots; version 1.37.0

### Exports and Excel import: CSV/Excel from every list and report; .xlsx imports
- Who: Claude (account A), session "Hiring Plug"
- Branch: claude/relaxed-hopper-ex7y3v
- Touches: list pages (candidates, companies, contacts, jobs, placements), reports, the import page; export endpoints in those modules
- Started: 22 Nov 2026    Last update: 22 Nov 2026
- State: waiting for the owner (1.34.0 built; review fixes in progress)
- Next: review fixes, verify; merge when the owner says


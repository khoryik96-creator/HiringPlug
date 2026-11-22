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

### Exports and Excel import: CSV/Excel from every list and report; .xlsx imports
- Who: Claude (account A), session "Hiring Plug"
- Branch: claude/relaxed-hopper-ex7y3v
- Touches: list pages (candidates, companies, contacts, jobs, placements), reports, the import page; export endpoints in those modules
- Started: 22 Nov 2026    Last update: 22 Nov 2026
- State: ready to merge (1.34.0: exports, Excel import, tests, screenshots)
- Next: the owner says merge; then remove this entry


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
- Touches: contacts and companies (the stale-record flag and list); later every user-visible "HiringPlug" string
- Started: 25 Nov 2026    Last update: 27 Nov 2026
- State: merged so far: 1.50.1 (held recurring jobs carry on), 1.51.0 (P1: links re-signed on retry, backups verified,
  daily and per-feature AI limits, failed AI attempts counted), 1.52.0 (P2: Settings › System server checks; the
  Codespaces part skipped at the owner's word). Now: the stale-record sweep, as the owner chose "flag and list".
- Next:
  1. Stale-record sweep: contacts and companies untouched for a year get a "Not touched in a year" filter and tag on
     their lists, and a monthly line in the morning email. Nothing is changed or hidden.
  2. Rename the product to HyPlug in everything people see (not formatted CVs or the agency's own branding; keep hidden
     technical names: Go module, database, HP_* settings, Python package, storage keys). Waits for the owner's go-ahead.

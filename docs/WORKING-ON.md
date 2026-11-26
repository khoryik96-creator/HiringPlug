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
- Touches: jobqueue (held jobs), the dead-job handlers of recurring kinds; later every user-visible "HiringPlug" string
- Started: 25 Nov 2026    Last update: 26 Nov 2026
- State: in progress, in the owner's order: 1, then 3, then the stale-record sweep; 2 (the rename) waits for the owner's go-ahead.
  Operator settles of held jobs are already audited in each company's own log (cmd/hp/jobs.go settleCompany).
- Next:
  1. Known gap: cancelling a HELD recurring job after a restore (email.outlook_sync, email.bounce_scan, calendar.refresh,
     scheduling.refresh_calendar, lead_source.check, any self-rescheduling kind) ends its chain for good; only
     notifications.digest is re-armed. Make cancel (and final death) schedule the next normal run, one per chain, with tests.
     Also audit operator settles of another company's held jobs in that company's log; tests through cancel for
     contact.check_mailbox and contact.apollo_lookup.
  2. Rename the product to HyPlug in everything people see (not formatted CVs or the agency's own branding; keep hidden
     technical names: Go module, database, HP_* settings, Python package, storage keys).
  3. P1 of the report: download links re-signed when jobs are retried; backup verify/preflight; AI daily and per-feature
     limits and retry billing. P2: Codespaces startup, admin diagnostics. Then the stale-record sweep.

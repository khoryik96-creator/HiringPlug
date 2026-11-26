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

### Owner's list of 26 Nov: CV reading, email, client link, small fixes
- Who: Claude (account B), session on branch claude/brave-thompson-k08hlg
- Branch: claude/brave-thompson-k08hlg
- Touches: CV reading (ai/ parser: names, current job title and employer), Outlook mail screens (forward with
  attachments and an editable body, remembered recipients, last place visited, docking right, Send always in view),
  client shortlist link (notify the recruiter on an answer, fields in our comment, the CV viewable on a phone),
  booking links (Teams link / phone, or ask for theirs), company country tag after the name, Team page
  (Cancel invitation button), dropping CVs anywhere on the candidate list, removing a candidate from a job,
  "Your sign-in is invalid or has expired" while working
- Started: 26 Nov 2026    Last update: 26 Nov 2026
- State: in progress
- Next: the list above, CV reading first; each part merged on its own

### Post-migration fixes and the HyPlug rename (paused)
- Who: Claude (account A), session "Hiring Plug"
- Branch: claude/relaxed-hopper-ex7y3v (in step with main; nothing unmerged)
- Touches: jobqueue (held jobs), the dead-job handlers of recurring kinds; later every user-visible "HiringPlug" string
- Started: 25 Nov 2026    Last update: 25 Nov 2026
- State: stopped (owner's call, usage limit). 1.47.0 (P0 of the owner's post-migration report) is merged.
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

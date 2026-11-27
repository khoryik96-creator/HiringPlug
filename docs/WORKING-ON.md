# Working on now

Several AI helpers (Claude on two accounts, ChatGPT) change HyPlug at the
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

### Targets by stage category, people picker, deeper reports
- Who: Claude (account B), session "brave-thompson"
- Branch: claude/brave-thompson-k08hlg
- Touches: reports module (targets, activity, summary), web/src/pages/Dashboard*, Reports*, TargetSettings*
- Started: 27 Nov 2026    Last update: 27 Nov 2026
- State: in progress
- Next: 1) targets for any stage category (set by admins); 2) choose one or more people, or the team, on the
  dashboard's targets and jobs; 3) reports: billings by month/quarter/YTD/year, CVs sent and each category,
  conversions between them, placements, average days to fill and to the first CV, by week/month/quarter/year.

### Post-migration fixes and the HyPlug rename (taken over)
- Who: Claude (account B), taken over from Claude (account A) on 26 Nov at the owner's request
- Branch: claude/brave-thompson-k08hlg
- Touches: the product's name everywhere people see it (1.56.0, the rename), then the PDF text reader
- Started: 25 Nov 2026    Last update: 27 Nov 2026
- State: merged: 1.50.1 to 1.55.0, the rename 1.56.0, 1.56.1 (the committed migrations 1.56.0 changed put back),
  1.56.2 (PDFs saved without spaces between words read right; the owner's CV), 1.57.0 (stage categories:
  1st, 2nd and Final Interview on the board, tags and report; owners edit them in Settings › Pipeline stages).
- Next: the LinkedIn companion's listed name (manifest "name") becomes "HyPlug LinkedIn Companion" once companions
  run 1.7.0 (its updater takes either name from then on).

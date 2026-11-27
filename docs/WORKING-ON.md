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

### Stage categories (1st, 2nd, Final Interview)
- Who: Claude (account B)
- Branch: claude/brave-thompson-k08hlg (worked locally, merged when verified)
- Touches: pipeline module (stage_category table, migration 00207, stages, moves), reports funnel, web Settings › Pipeline stages, StepsEditor, PipelineBoard, CandidateApplications, Reports
- Started: 27 Nov 2026    Last update: 27 Nov 2026
- State: verifying 1.57.0
- Next: merge when `task verify` passes; then remove this entry.

### Post-migration fixes and the HyPlug rename (taken over)
- Who: Claude (account B), taken over from Claude (account A) on 26 Nov at the owner's request
- Branch: claude/brave-thompson-k08hlg
- Touches: the product's name everywhere people see it (1.56.0, the rename), then the PDF text reader
- Started: 25 Nov 2026    Last update: 27 Nov 2026
- State: merged: 1.50.1 to 1.55.0, the rename 1.56.0, 1.56.1 (the committed migrations 1.56.0 changed put back),
  1.56.2 (PDFs saved without spaces between words read right; the owner's CV).
- Next: the LinkedIn companion's listed name (manifest "name") becomes "HyPlug LinkedIn Companion" once companions
  run 1.7.0 (its updater takes either name from then on).

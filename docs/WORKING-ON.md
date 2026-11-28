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

### Post-migration fixes and the HyPlug rename (taken over)
- Who: Claude (account B), taken over from Claude (account A) on 26 Nov at the owner's request
- Branch: claude/brave-thompson-k08hlg
- Touches: since 1.57: stage categories, targets, the performance report, Settings lists (renames reach candidates)
- Started: 25 Nov 2026    Last update: 28 Nov 2026
- State: merged: 1.50.1 to 1.55.0, the rename 1.56.0, 1.56.1 (the committed migrations 1.56.0 changed put back),
  1.56.2 (PDFs saved without spaces between words read right; the owner's CV), 1.57.0 (stage categories:
  1st, 2nd and Final Interview on the board, tags and report; owners edit them in Settings › Pipeline stages),
  1.58.0 (Apollo email saved to the contact), 1.59.0 to 1.59.2 (targets per stage category, people picker,
  performance report), 1.60.0 and 1.60.1 (a word renamed in Settings is renamed on every candidate and in
  everyone's favourites; the dropdowns only offer the list). In progress: 1.60.2, the review fixes for 1.60.1.
- Next: the LinkedIn companion's listed name (manifest "name") becomes "HyPlug LinkedIn Companion" once companions
  run 1.7.0 (its updater takes either name from then on).

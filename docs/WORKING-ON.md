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

### Code review of 1.38.1; meeting editor toolbar
- Who: Claude (account A), session "Hiring Plug"
- Branch: claude/relaxed-hopper-ex7y3v
- Touches: review only; then web/src/components/clientEmail/RichEditor.tsx and web/src/components/calendar/ComposeBody.tsx (the toolbar's buttons in the new-meeting window)
- Started: 23 Nov 2026    Last update: 23 Nov 2026
- State: in progress
- Next: report the review of bddbe0d..1.38.1; fix the toolbar's squashed labels in the meeting window; fix what the owner asks

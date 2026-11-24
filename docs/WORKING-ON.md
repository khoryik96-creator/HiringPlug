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

### Code review of 1.40.0 (search by meaning)
- Who: Claude (account A), session "Hiring Plug"
- Branch: claude/relaxed-hopper-ex7y3v
- Touches: the meaning module, matching (possible candidates), the AI worker's meaning/ and the candidates page; on the branch already, not merged: 1.40.1 (why in a Match column, the owner's report of text running across columns) and 1.40.2 (the agency's verticals: cyber security and operations fields, tools and titles)
- Started: 23 Nov 2026    Last update: 24 Nov 2026
- State: in progress (the review's checks run; 6 of the first 10 findings confirmed so far)
- Next: finish the review, report it to the owner, fix what the owner asks, merge 1.40.x when the owner says


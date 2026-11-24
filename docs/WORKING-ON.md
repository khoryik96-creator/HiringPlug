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

### AI usage and cost per month, with a limit
- Who: Claude (account A), session "Hiring Plug"
- Branch: claude/relaxed-hopper-ex7y3v
- Touches: a new AI usage page in Settings; the AI allowance check (assistant, matching, app ai_adapters); each AI-using module gains a per-month usage query (assistant, matching, jobs, companies, cvformats, interviewprep, documents, talent search); config HP_AI_MONTHLY_TOKENS becomes the default limit
- Started: 24 Nov 2026    Last update: 24 Nov 2026
- State: in progress
- Next: usage per month and per feature with its cost from the owner's prices; the owner sets the monthly limit in Settings; warning near the limit

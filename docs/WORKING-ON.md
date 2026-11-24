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

### Find HR for a lead not linked to a company (owner chose layout A)
- Who: Claude (account A), session "Hiring Plug"
- Branch: claude/relaxed-hopper-ex7y3v
- Touches: Leads page (web/src/pages/Leads.tsx, new LeadFindHR panel), FindHR component (a compact mode), companies module (a "named like" lookup), the LinkedIn companion (a company search action)
- Started: 24 Nov 2026    Last update: 24 Nov 2026
- State: in progress
- Next: a Find HR side panel on Leads: pick the company (one in HiringPlug with a like name, or add it, its LinkedIn page found by the companion), link the lead, then the company's Find HR scan in the panel

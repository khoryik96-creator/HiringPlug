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

### Notifications: a bell for new applicants, tasks, offers to approve and tomorrow's interviews; an optional morning email
- Who: Claude (account A), session "Hiring Plug"
- Branch: claude/relaxed-hopper-ex7y3v
- Touches: a new notifications module (migration 00165), small read functions in jobs, pipeline, automations, offers, interviews; the top bar (web/src/App*), Settings › Your account
- Started: 22 Nov 2026    Last update: 22 Nov 2026
- State: in progress (backend done and tested; web next)
- Next: web: bell with unseen count in the top bar (GET /v1/notifications, POST /v1/notifications/seen); Settings › Your account "Notifications" section with a switch per kind and the morning email (PUT /v1/notifications/settings); web tests, screenshots, docs, VERSION 1.33.0, task verify

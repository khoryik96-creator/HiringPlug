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

### Post-migration fixes (owner's report): P0
- Who: Claude (account A), session "Hiring Plug"
- Branch: claude/relaxed-hopper-ex7y3v (one pull request per group)
- Touches: backup/restore (platform/backup, cmd/hp), deploy Dockerfiles and compose, documents module (state on failed jobs), jobqueue (dead AI jobs, held jobs after a restore), aiproviders (paid-AI kill switch) and the AI worker, Settings › System, docs/playbooks
- Started: 25 Nov 2026    Last update: 25 Nov 2026
- State: in progress
- Next: restored files owned by the runtime user; terminal AI-job failure updates the document; outbound jobs held after a restore from another server; global paid-AI switch; hp restore flags in any order. Then the HyPlug rename, then P1 and P2 of the report

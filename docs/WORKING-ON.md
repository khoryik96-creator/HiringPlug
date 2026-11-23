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

### Search by meaning (roadmap item 9)
- Who: Claude (account A), session "Hiring Plug"
- Branch: claude/relaxed-hopper-ex7y3v
- Touches: a new meaning module (backend/internal/modules/meaning), candidates search, matching (possible candidates), documents (CV read hook), the CV reader (a headhunter at an executive search firm), AI worker (meaning/, a local model), migration 00192 (pgvector), deploy (pgvector database image, the model in the AI image), web: the candidates page (≈ Meaning) and the job page's Possible candidates
- Started: 23 Nov 2026    Last update: 23 Nov 2026
- State: waiting for the owner (built and verified as 1.40.0 on the branch; merged when the owner says)
- Next: the owner tries it and says merge; then merge to main and remove this entry. Later, if wanted: AI search (talent) ranking its pool by meaning; the exact-words bonus for a tool's own name only

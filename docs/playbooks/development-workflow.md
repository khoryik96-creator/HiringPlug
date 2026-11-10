# Efficient development and CI diagnosis

Read `AGENTS.md` and `docs/HANDOVER.md` once when picking up work. Keep a short
record of the branch, changes, environment, checks and remaining work so a
resumed session does not repeat discovery.

## Reuse the environment

Check the current checkout, installed tools and database health before setup.
Keep Go, uv, pnpm and browser caches. Reuse `ai/.venv` and `web/node_modules`;
install dependencies only when missing or their lockfile changes. In a retained
cloud workspace, reuse its existing environment activation script when present.
Start PostgreSQL only if it is stopped. Rebuild binaries only after their
sources or embedded migrations change; do not run an older migration binary.

## Verify at the right scope

During development, reproduce a bug at the lowest relevant layer and run that
test while fixing it. For example:

```bash
(cd backend && go test ./internal/platform/salarycalc)
(cd web && pnpm exec vitest run src/path/to/changed.test.tsx)
(cd ai && uv run pytest tests/test_changed.py)
```

Run each command from the repository root (the paths above are examples).
Check changed screens in the real app and inspect their screenshots. Finish
editing and reviewing before the required `task verify` run; stop the dev
API, workers and web server first to avoid queue and memory contention. Keep
its log and exit status. Do not run another full suite on an unchanged tree
that already passed. Source changes after verification need another pass
before pushing; targeted checks do not replace that requirement.

CI on the exact commit also runs the generated-module check, service end-to-end
tests, browser smoke tests and Docker builds. A green run counts for those
pre-merge checks, as described in `HANDOVER.md`; do not duplicate them locally
without a failure or another reason. Push the branch, inspect CI, and merge only
when requested. Check main CI after merging.

The end-to-end CI job builds its own services and runs alongside the language
checks. It does not need their outputs; removing that dependency avoids waiting
for those jobs before it starts. All jobs must still pass before merging.

## Investigate slow CI

Inspect job and step timestamps, then the slow step's log. Compare with a recent
successful run. A long setup step is not evidence that application tests hang.
If `gh` cannot retrieve redirected logs, use the connected GitHub job-log tool;
do not repeatedly retry the same blocked download.

The OCR/document installer in `.github/scripts/install-document-tools.sh`
limits index updates to 30 seconds and package downloads to 90 seconds per
attempt, then retries once using Ubuntu's archive mirror instead of the Azure
mirror. It downloads before configuring packages, fails if downloads still
fail, and never skips the document tests. Both callers have a six-minute step
limit. Inspect the error before rerunning only the failed infrastructure job;
preserve successful jobs. Do not restart a run that is progressing normally.

Report what finished and the cause of a significant delay. During longer work,
give brief updates about once a minute with useful new information instead of
repeated unchanged status dumps. Batch independent reads, avoid rereading large
files, and keep exploration proportional to the change.

## Measured delay on 10 November 2026

- Main run `42802569118`, commit `455f021`: the document-tool installation took
  764 seconds in end-to-end CI, versus 17 seconds in the Python job. Its log
  shows a 651-second gap during downloads from `azure.archive.ubuntu.com`,
  after the 40 MB LibreOffice core package was requested. The entire 101 MB
  download took 12 minutes 32 seconds. CI ultimately passed all jobs.
- The preceding run on the same commit, `42801396203`, installed the same
  tools in 16 seconds in end-to-end CI. The variability was in the package
  mirror/download path, rather than HiringPlug's tests.
- Four retained local verification logs span about 29 minutes in total
  (12m28s, 6m07s, 5m48s and 5m04s). Later runs reused the installed environment;
  they do not support blaming repeated dependency installation for the whole
  delay. Late follow-up fixes caused repeated full runs. Finishing targeted
  checks before the final suite avoids this avoidable work.

These measurements do not establish a speed comparison with another coding
tool; they identify concrete delays in this session.

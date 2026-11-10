# Efficient development and CI diagnosis

Read `AGENTS.md` and `docs/HANDOVER.md` when picking up work. Check the current
branch, existing changes, installed tools and database health before setup.
Reuse Go, uv, pnpm and browser caches, `ai/.venv` and `web/node_modules`. Install
dependencies only when missing or their lockfile changes. Start PostgreSQL
only if stopped; the cloud setup commands are in `HANDOVER.md`.

## Verification

Reproduce bugs at the lowest relevant layer and run focused tests while editing.
Check changed screens in the real app and inspect their screenshots. Rebuild
binaries after source or embedded migration changes before using them.

Finish editing and review, stop the dev API, workers and web server to avoid
queue and memory contention, then run `task verify` before pushing. Keep its log
and exit status. Repeat full verification after changes or failures that warrant
it; do not repeat a successful suite on an unchanged tree.

`task lint:shell` checks the CI scripts with ShellCheck. `task test:ci` runs their
tests from `.github/scripts/tests` using Python 3's standard library, without
the AI worker or external services. Both are included in `task verify` and CI.

CI on the exact commit also covers the generated-module check, service
end-to-end tests, browser smoke tests and Docker builds. A green run counts for
those pre-merge checks under `HANDOVER.md`. Push the branch, inspect CI, merge
when requested, then check main CI.

End-to-end tests run alongside the language checks because they build their own
services. This reduces waiting but can spend CI minutes on end-to-end tests
even when another job fails. Every job must still pass before merging.

## Slow CI and document tools

Inspect job and step timestamps, then the slow step's log. Compare with a recent
successful run to separate setup delays from test failures. If CLI log downloads
are blocked, use another available GitHub log client rather than repeating the
same failed request. Retry only the failed infrastructure job after diagnosing
the error; preserve successful jobs.

The AI and end-to-end jobs share the document-tool action in
`.github/actions/document-tools`. It caches downloaded `.deb` files by package
list, runner image and architecture. APT still refreshes signed package indexes
and validates versions and checksums; new package versions may need downloading.
Changing the runner image or package list creates a fresh cache. A cold cache
can require downloads in both jobs; later runs reuse the saved packages.

The installer keeps index updates bounded to 30 seconds and package downloads
to 90 seconds per attempt, then retries once. It recognises regional/runner
Ubuntu archive names in direct sources and `apt-mirrors.txt`, switches them to
the main archive, and warns explicitly if no URL changed. Package downloads
finish before configuration starts. Installation failures stop CI and never
skip document tests. Each caller has a six-minute limit.

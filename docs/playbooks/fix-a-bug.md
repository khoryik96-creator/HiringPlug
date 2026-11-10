# Fix a bug

1. **Find it.** Use the request ID from the error message or the
   `X-Request-Id` header to find every log line for that request in the API,
   the Go worker and the Python worker. See [debug-a-problem.md](debug-a-problem.md).
2. **Reproduce it with a failing test first**, at the lowest layer that shows
   it:
   - a wrong rule → `domain_test.go` (fast, pure);
   - wrong behaviour through the API → `internal/app/<module>_test.go`;
   - a document-reading problem → `ai/tests/` with a generated document that
     reproduces it (never commit a real CV).
3. **Fix it** in the right layer (rules in `domain.go`, data in queries,
   orchestration in `service.go`).
4. **Prove the test guards the fix:** undo the fix once and watch the test fail.
5. **Keep the test** next to the feature it protects, never in a
   version-named file.
6. Database problem? **Never edit a committed migration**; add a new one that
   corrects the data or schema.
7. Finish targeted checks and review, then `task verify` before pushing the
   branch. Follow `HANDOVER.md` for merging; create a pull request only when
   asked. See [development-workflow.md](development-workflow.md) for environment
   reuse and CI diagnosis.

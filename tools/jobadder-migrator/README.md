# HiringPlug Migration: JobAdder discovery

A separate Go application with a local browser interface. This first release
inventories accessible JobAdder resources and saves the original discovery
responses. **It does not yet export 80,000 candidate records, download CVs or
import anything into HiringPlug.** No source records are modified.

## Windows: run the packaged tool

1. Extract the ZIP to a local folder. Keep the executable, launchers and this
   README together. No Docker, Go, Python or HiringPlug installation is needed.
2. Double-click **Demo.bat** to try synthetic sample data without credentials.
   Close its console before opening the real tool on the same port.
3. Double-click **Start.bat** for your real account. Keep the console open.
4. Choose a private workspace passphrase of at least 16 characters. Keep it in
   your password manager; there is no recovery if it is lost.
5. Connect JobAdder as described below, then click **Start new inventory**.
6. Review denied/unavailable resources and coverage gaps. **Unknown** is not
   zero. Download the count report and, if needed, original responses.

The default workspace is `%APPDATA%\HiringPlugMigration`. Demo data lives in its
separate `demo` subfolder. The browser opens automatically; if it does not, open
`HiringPlug Migration.url` inside the workspace while the console is running.
That shortcut contains a private local session key: do not share it.

Press Ctrl+C to stop. Reopen with the same workspace and passphrase, then use
**Resume / retry unresolved** to keep completed checkpoints. A new inventory
keeps earlier runs in the workspace and displays the latest one.

The executable is not code-signed. Windows SmartScreen may ask you to inspect
the publisher/source. Repository CI runs tests on Windows and Linux; a local
cross-build alone does not establish compatibility with your particular PC.

## Connect your JobAdder account

Use your existing JobAdder developer application. Register the exact callback
shown in the interface; the default is:

```
http://localhost:8769/oauth/callback
```

Enter the app's client ID and secret **locally in this tool**, then choose
**Sign in to JobAdder**. The application requests `read offline_access`. It uses
the regional API address returned by JobAdder and refreshes expired access
tokens when refresh credentials are available. The JobAdder app/account must
permit this redirect and these scopes; provider-side approval is outside this
tool.

Alternatively, expand **Already have an access token?** and enter the access
token and its returned API address, ending in `/v2`. Token-only connections
cannot refresh: reconnect when they expire. Do not paste secrets or tokens into
chat, issues, reports or source code.

The workspace is bound conservatively to the API address, current API user and
account name. A different user/account/address requires a separate workspace.
If the account name is missing, the connection is refused rather than risking
mixing data. The tool needs access to `/users/current` to verify this identity.

For another port or a separate account, run in PowerShell from the tool folder:

```powershell
.\HiringPlugMigration.exe --workspace "$env:LOCALAPPDATA\HiringPlugMigration-Agency2" --port 8770
```

Register the corresponding port's callback. Flags also include `--demo` and
`--no-browser`. The listener accepts connections only on `127.0.0.1`.

## What this release checks

- Counts for candidates, companies, contacts, jobs, applications, placements,
  notes, submissions, floats, interviews, events, tasks, folders, users, groups,
  offices, ads, opportunities, requisitions and candidate attachment metadata.
- Activity collections, candidate payroll/billing access and source definitions
  such as custom fields, statuses, attachment categories and lookup lists.
- Individual access gaps, failed requests, missing totals and resumable progress.

Counts use JobAdder's `Limit=0` where supported. Permissions, default visibility
and resources not supported by your account can limit the result. Definition
responses can be paginated: saving one response does not mean every definition
or custom-field value has been exported. Counts are observed at different times,
not a transactionally consistent account snapshot.

CV binaries/versions, nested attachments, folder membership, invoices,
timesheets, rate cards, requisition forms/approvals, complete stage histories,
email/SMS bodies and deleted/purged records still need deeper extraction or a
separate JobAdder export. The coverage report lists these gaps explicitly.

## Preservation and privacy

- Successful response bodies are saved byte for byte, encrypted using AES-GCM
  with a scrypt-derived workspace key, and hashed with SHA-256. Originals are
  checked before download. Numeric precision, unknown fields, nulls, wording
  and typos in those responses are not rewritten.
- Saved credentials and response bodies are encrypted. Counts, labels, run
  timestamps, status codes and hashes are SQLite metadata and are not encrypted.
  Use a private local folder and an encrypted disk; avoid shared/synced folders.
- Downloaded reports and original-response ZIP files are **unencrypted**. They
  may contain personal data; handle them as private account exports.
- Stop the application before backing up the **whole workspace folder**.
  Preserve the passphrase separately. Do not copy only an open SQLite database
  and omit its WAL files. Replacing the executable does not reset the workspace.
- API resource calls use GET only. OAuth token exchange/refresh uses POST to
  JobAdder's authorization server; it does not write business records. There
  are no HiringPlug writes, AI calls, analytics or background CV parsing.

## Develop and verify

From this folder:

```sh
go mod download
go vet ./...
go test -race ./...
go build -trimpath -o dist/HiringPlugMigration .
```

From the repository root, `task verify` also checks this module and cross-builds
`tools/jobadder-migrator/dist/HiringPlugMigration.exe` for Windows x64.
`task build:migrator` runs just that build. Race tests require a supported Go
host/toolchain; `go test ./...` also runs on Windows CI.

The browser check needs the repository's existing web Playwright installation:

```sh
node tools/jobadder-migrator/test/browser-smoke.cjs \
  tools/jobadder-migrator/dist/HiringPlugMigration /tmp/jobadder-ui-check
```

It uses synthetic data only and checks unlock/connect screens, inventory,
pause/resume, denied-access reporting, downloads and mobile layout. It does not
test real JobAdder consent or promise that every account exposes every endpoint.

The full migration architecture and remaining stages are documented in
`docs/jobadder-migration.md` in the HiringPlug repository.

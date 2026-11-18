# HiringPlug Migration: JobAdder extraction

A separate Go application with a local browser interface. It extracts available
JobAdder records, related data and native files into an encrypted local workspace.
It also preserves provider exports and indexes supported logical formats.
**All 30 scope areas are tracked; complete account coverage still requires source
reconciliation and provider exports for API gaps. Nothing is imported into
HiringPlug or changed in JobAdder by this tool.**

## Windows: run the packaged tool

1. Extract the ZIP to a local folder. Keep the executable, launchers and this
   README together. No Docker, Go, Python or HiringPlug installation is needed.
2. Double-click **Demo.bat** to try synthetic sample data without credentials.
   Close its console before opening the real tool on the same port.
3. Double-click **Start.bat** for your real account. Keep the console open.
4. Choose a private workspace passphrase of at least 16 characters. Keep it in
   your password manager; there is no recovery if it is lost.
5. Connect JobAdder as described below, then click **Start new inventory** for counts or **Start full API extraction**.
6. Use **Pause** and **Resume extraction / retry gaps** to continue saved work.
   Keep the console open during extraction. Reopening the same workspace does
   not refetch completed checkpoints. A new extraction deliberately reads again.
7. Review **30-area coverage**, **API route coverage** and unresolved requests.
   Add any JobAdder/provider exports under **Add provider exports**.
8. Pause work. In Chrome/Edge, **Save complete export to folder** streams all
   parts into a new private folder you select. Alternatively, choose
   **Prepare downloads**, then download the manifest and
   **every object part**. Keep them together. The manifest has IDs, associations,
   original filenames, headers, source metadata, statuses and checksums.

The default workspace is `%APPDATA%\HiringPlugMigration`. Demo data lives in its
separate `demo` subfolder. The browser opens automatically; if it does not, open
`HiringPlug Migration.url` inside the workspace while the console is running.
That shortcut contains a private local session key: do not share it.

Press Ctrl+C to stop. Reopen with the same workspace and passphrase, then use
the appropriate **Resume** button to keep completed checkpoints. A new inventory
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

## Extraction and coverage

The tool bundles all 240 documented v2 GET operations. It queues fixed, typed
routes from actual source identifiers; it never fetches arbitrary URLs found in
notes, exports or API bodies. Bulk collections provide notes, applications,
placements and activities; the route report names each equivalent bulk source.
Full detail routes and non-equivalent child routes are separately checkpointed.
It preserves every returned field, including unknown fields and original custom,
salary, note, status and tag values. Related references remain separate observed
representations; a short reference cannot replace a full profile.

- Explicit partitions include open/closed jobs, active/inactive applications,
  approved/unapproved placements and folders owned by active/inactive users.
  Placement queries include permanent, contract, temporary and credit types.
- Old/completed tasks, inactive/deleted users, activity details and non-latest
  attachments are requested where the documented endpoint supports them.
- Attachment metadata discovers original retained files across candidates,
  companies, contacts, jobs, applications, placements, requisitions and notes.
  Native downloads omit conversion `Accept` headers. Approved HTTPS object-storage
  redirects strip bearer credentials; unknown destinations become review gaps.
- JSON pagination advances by actual page size, checks totals and detects
  repeated pages independently of changing pagination links. Source changes
  during a run can still cause offset drift; this is not a database snapshot.
- The queue grows as records are found. Known-task counts are not a final
  percentage. Default requests are paced conservatively (about two/second,
  excluding response/processing time). An 80,000-candidate account with files and
  linked records can take many hours or days; do a representative pilot first.
- `Retry-After` deadlines persist across restart. Long waits pause immediately.
  Expired token-only connections require reconnection; OAuth can refresh.
- Errors and denied/missing resources remain unresolved. A missing optional
  photo may return the same 404 as an inaccessible photo; absence is not proof
  of permission. A zero-request child route is unverified, not confirmed empty.

**API pass finished** means the current known queue was processed, not that all
30 areas were completely recovered. The public API lacks a complete mailbox,
personal settings/templates and full audit/privacy history. Deleted endpoints
return limited recent tombstones, not restorable deleted record bodies.
Video URLs and rate-card strings remain references; external contents need an
appropriate separate source. Ask JobAdder for a full account export, attachments
and a schema/manifest, then reconcile IDs, versions, links and boundary samples.

### Provider exports: format may be unknown

The tool first encrypts and preserves the original file byte for byte. ZIP
members are additionally retained without extracting source paths onto disk.
CSV (including BOM-marked UTF-16), JSON arrays/`items` envelopes and JSONL/NDJSON
are streamed into a row-pointer index. Choose the declared record type and an
optional original ID field; values and column names are not renamed or cleaned.
An unmapped type remains unmapped. Generic indexing does not interpret foreign
keys, grant accounts, resolve conflicts or verify source/account identity.

Opaque `.bak`, `.pst`, `.eml`, `.mbox`, SQL, Excel and other files remain intact
with **needs adapter** status. A SQL Server backup needs a restore/read adapter
or logical relational export after its actual schema is known. No SQL or
archive executable is run. Uploading an archive never marks an area complete.
A cancelled/malformed intake retains its saved original and reports review.

Safety bounds are explicit: 1 TiB per original, 16 MiB per logical indexing
record, 32 MiB per API JSON indexing response, 100,000 ZIP members per archive,
three ZIP levels, and expansion-ratio checks. Exceeding a bound flags review;
it does not pretend truncated contents are complete. Large original objects
are streamed using 1 MiB authenticated encrypted chunks. Export parts target
256 MiB/1,000 objects; a single larger object occupies its own larger part.
A part is staged on disk before download, so allow space for the largest part,
ZIP intake staging and the encrypted workspace. Browser downloads stream to disk. Folder export
requires Chrome/Edge's directory picker; other browsers use individual parts.
If the saved object set changes between parts, a plan check stops the export
rather than combining inconsistent manifest/parts. Incomplete folders must be
kept separate and retried. Unlinked completed objects from interrupted indexing
are included for association review, rather than silently omitted.

Inventory remains available separately: `Limit=0` counts, original definition
responses, category paging and its original discovery evidence downloads.

## Preservation and privacy

- Successful response bodies are saved byte for byte, encrypted using AES-GCM
  with a scrypt-derived workspace key, and hashed with SHA-256. Originals are
  checked before download. Numeric precision, unknown fields, nulls, wording
  and typos in those responses are not rewritten.
- Response/attempt history is append-only. A failed retry does not erase an
  earlier saved response, and changed responses retain both versions. ZIPs
  include `observations/` with saved versions, `responses/` with the latest saved
  response per resource, and a manifest identifying attempt/evidence timestamps.
  For paginated categories, `responses/` is the last saved page; use all the
  pages in the completed scan's observation history. Upgrades preserve evidence
  still present in older workspaces; already overwritten data cannot be recovered.
- Saved credentials and response bodies are encrypted. Source IDs, routing paths, relationships, counts, labels, run
  timestamps, status codes and hashes are SQLite metadata and are not encrypted.
  Use a private local folder and an encrypted disk; avoid shared/synced folders.
- Downloaded reports and original-response ZIP files are **unencrypted**. They
  may contain personal data; handle them as private account exports.
- ZIPs are prepared and verified in a private temporary file before download,
  while retaining status access; new extraction/intake waits until staging/download ends. The OS removes staging on close/process exit
  (unlinked on Unix, delete-on-close on Windows); downloads have a write deadline.
- Stop the application before backing up the **whole workspace folder**.
  Preserve the passphrase separately. Do not copy only an open SQLite database
  and omit its WAL files. Replacing the executable does not reset the workspace.
- API resource calls use GET only. OAuth token exchange/refresh uses POST to
  JobAdder's authorization server; it does not write business records. There
  are no HiringPlug writes, AI calls, analytics or background CV parsing.

## Develop and verify

From this folder:

```sh
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
pause/resume, full extraction, exact native IDs/files, 30-area/240-route coverage,
provider intake, manifest/object downloads and mobile layout. It does not
test real JobAdder consent or promise that every account exposes every endpoint.

The full migration architecture and remaining stages are documented in
`docs/jobadder-migration.md` in the HiringPlug repository.
